package provider

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"go4.org/netipx"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
	"k8s.io/utils/set"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/config"
	"github.com/kube-vip/kube-vip-cloud-provider/pkg/ipam"
)

const (
	// LoadbalancerIPsAnnotation is for specifying IPs for a loadbalancer
	// use plural for dual stack support in the future
	// Example: kube-vip.io/loadbalancerIPs: 10.1.2.3,fd00::100
	LoadbalancerIPsAnnotation = "kube-vip.io/loadbalancerIPs"

	// ImplementationLabelKey is the label key showing the service is implemented by kube-vip
	ImplementationLabelKey = "implementation"

	// ImplementationLabelValue is the label value showing the service is implemented by kube-vip
	ImplementationLabelValue = "kube-vip"

	// LegacyIpamAddressLabelKey is the legacy label key showing the service is implemented by kube-vip
	LegacyIpamAddressLabelKey = "ipam-address"

	// LoadbalancerServiceInterfaceAnnotationKey is the annotation key for specifying the service interface for a load balancer
	LoadbalancerServiceInterfaceAnnotationKey = "kube-vip.io/serviceInterface"
)

// kubevipLoadBalancerManager -
type kubevipLoadBalancerManager struct {
	kubeClient     kubernetes.Interface
	namespace      string
	cloudConfigMap string
}

func newLoadBalancer(kubeClient kubernetes.Interface, ns, cm string) cloudprovider.LoadBalancer {
	k := &kubevipLoadBalancerManager{
		kubeClient:     kubeClient,
		namespace:      ns,
		cloudConfigMap: cm,
	}
	return k
}

func (k *kubevipLoadBalancerManager) EnsureLoadBalancer(ctx context.Context, _ string, service *v1.Service, _ []*v1.Node) (lbs *v1.LoadBalancerStatus, err error) {
	return syncLoadBalancer(ctx, k.kubeClient, service, k.cloudConfigMap, k.namespace)
}

func (k *kubevipLoadBalancerManager) UpdateLoadBalancer(ctx context.Context, _ string, service *v1.Service, _ []*v1.Node) (err error) {
	_, err = syncLoadBalancer(ctx, k.kubeClient, service, k.cloudConfigMap, k.namespace)
	return err
}

func (k *kubevipLoadBalancerManager) EnsureLoadBalancerDeleted(ctx context.Context, _ string, service *v1.Service) error {
	return k.deleteLoadBalancer(ctx, service)
}

func (k *kubevipLoadBalancerManager) GetLoadBalancer(_ context.Context, _ string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	if service.Labels[ImplementationLabelKey] == ImplementationLabelValue {
		return &service.Status.LoadBalancer, true, nil
	}
	return nil, false, nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (k *kubevipLoadBalancerManager) GetLoadBalancerName(_ context.Context, _ string, service *v1.Service) string {
	return getDefaultLoadBalancerName(service)
}

func getDefaultLoadBalancerName(service *v1.Service) string {
	return cloudprovider.DefaultLoadBalancerName(service)
}

func (k *kubevipLoadBalancerManager) deleteLoadBalancer(_ context.Context, service *v1.Service) error {
	klog.Infof("deleting service '%s' (%s)", service.Name, service.UID)

	return nil
}

func checkLegacyLoadBalancerIPAnnotation(ctx context.Context, kubeClient kubernetes.Interface, service *v1.Service) (*v1.LoadBalancerStatus, error) {
	if service.Spec.LoadBalancerIP != "" {
		if v, ok := service.Annotations[LoadbalancerIPsAnnotation]; !ok || len(v) == 0 {
			klog.Warningf("service.Spec.LoadBalancerIP is defined but annotations '%s' is not, assume it's a legacy service, updates its annotations", LoadbalancerIPsAnnotation)
			// assume it's legacy service, need to update the annotation.
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				recentService, getErr := kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
				if getErr != nil {
					return getErr
				}
				if recentService.Annotations == nil {
					recentService.Annotations = make(map[string]string)
				}
				recentService.Annotations[LoadbalancerIPsAnnotation] = service.Spec.LoadBalancerIP
				// remove ipam-address label
				delete(recentService.Labels, LegacyIpamAddressLabelKey)

				// Update the actual service with the annotations
				_, updateErr := kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
				return updateErr
			})
			if err != nil {
				return nil, fmt.Errorf("error updating Service Spec [%s] : %v", service.Name, err)
			}
		}
		return &service.Status.LoadBalancer, nil
	}
	return nil, nil
}

func parseAddrList(inputString string) (addrs []netip.Addr, err error) {
	addrStringList := strings.Split(inputString, ",")
	var addrList []netip.Addr

	for i := range addrStringList {
		addrString := addrStringList[i]
		addr, err := netip.ParseAddr(addrString)
		if err != nil {
			return nil, err
		}
		addrList = append(addrList, addr)
	}

	return addrList, nil
}

// Gather infos about implemented services
func mapImplementedServices(svcs *v1.ServiceList, allowShare bool) (inUseSet *netipx.IPSet, servicePortMap map[string]*set.Set[int32], err error) {

	builder := &netipx.IPSetBuilder{}
	servicePortMap = map[string]*set.Set[int32]{}

	for x := range svcs.Items {
		var svc = svcs.Items[x]

		if ips, ok := svc.Annotations[LoadbalancerIPsAnnotation]; ok {
			addrs, err := parseAddrList(ips)
			if err != nil {
				return nil, nil, err
			}

			for a := range addrs {
				addr := addrs[a]
				ip := addr.String()

				// Store service port mapping to help decide whether services could share the same IP.
				if allowShare && addr.Is4() {
					if len(svc.Spec.Ports) != 0 {
						for p := range svc.Spec.Ports {
							var port = svc.Spec.Ports[p].Port

							portSet, ok := servicePortMap[ip]
							if !ok {
								newSet := set.New[int32]()
								servicePortMap[ip] = &newSet
								portSet = servicePortMap[ip]
							}
							portSet.Insert(port)
						}
					} else {
						// special case, if the services does not define ports
						klog.Warningf("Service [%s] does not define ports, consider IP %s non-shareble", svc.Name, ip)

						newSet := set.New[int32](0)
						servicePortMap[ip] = &newSet
					}
				}

				// Add to IPSet in case we need to find a new free address
				builder.Add(addr)
			}
		}
	}
	inUseSet, err = builder.IPSet()
	if err != nil {
		return nil, nil, err
	}

	return inUseSet, servicePortMap, nil
}

// syncLoadBalancer
// 1. Is this loadBalancer already created, and does it have an address? return status
// 2. Is this a new loadBalancer (with no IP address)
// 2a. Get all existing kube-vip services
// 2b. Get the network configuration for this service (namespace) / (CIDR/Range)
// 2c. Between the two find a free address

func syncLoadBalancer(ctx context.Context, kubeClient kubernetes.Interface, service *v1.Service, cmName, cmNamespace string) (*v1.LoadBalancerStatus, error) {
	// This function reconciles the load balancer state
	klog.Infof("syncing service '%s' (%s)", service.Name, service.UID)

	// The loadBalancer address has already been populated
	if status, err := checkLegacyLoadBalancerIPAnnotation(ctx, kubeClient, service); status != nil || err != nil {
		return status, err
	}

	// Check if the service already got a LoadbalancerIPsAnnotation,
	// if so, check if LoadbalancerIPsAnnotation was created by cloud-controller (ImplementationLabelKey == ImplementationLabelValue)
	if v, ok := service.Annotations[LoadbalancerIPsAnnotation]; ok && len(v) != 0 {
		klog.Infof("service '%s/%s' annotations '%s' is defined but service.Spec.LoadBalancerIP is not. Assume it's not legacy service", service.Namespace, service.Name, LoadbalancerIPsAnnotation)
		// Set label ImplementationLabelKey, otherwise cloud-provider will skip the service
		if service.Labels == nil || service.Labels[ImplementationLabelKey] != ImplementationLabelValue {
			klog.Infof("service '%s/%s' created with pre-defined ip '%s'", service.Namespace, service.Name, v)
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				recentService, getErr := kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
				if getErr != nil {
					return getErr
				}
				if recentService.Labels == nil {
					// Just because ..
					recentService.Labels = make(map[string]string)
				}
				recentService.Labels[ImplementationLabelKey] = ImplementationLabelValue
				// Update the actual service with the annotations
				_, updateErr := kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
				return updateErr
			})
			if err != nil {
				return nil, fmt.Errorf("error updating Service Spec [%s] : %v", service.Name, err)
			}
		}
		return &service.Status.LoadBalancer, nil
	}

	// Get the cloud controller configuration map
	controllerCM, err := getConfigMap(ctx, kubeClient, cmName, cmNamespace)
	if err != nil {
		klog.Errorf("Unable to retrieve kube-vip ipam config from configMap [%s] in %s", cmName, cmNamespace)
		// TODO - determine best course of action, create one if it doesn't exist
		controllerCM, err = createConfigMap(ctx, kubeClient, cmName, cmNamespace)
		if err != nil {
			return nil, err
		}
	}

	// Get ip pool from configmap and determine if it is namespace specific or global
	pool, global, allowShare, err := discoverPool(controllerCM, service.Namespace, cmName)
	if err != nil {
		return nil, err
	}

	var serviceNamespace = ""
	if !global {
		serviceNamespace = service.Namespace
	}

	svcs, err := kubeClient.CoreV1().Services(serviceNamespace).List(ctx, metav1.ListOptions{LabelSelector: getKubevipImplementationLabel()})
	if err != nil {
		return &service.Status.LoadBalancer, err
	}

	inUseSet, servicePortMap, err := mapImplementedServices(svcs, allowShare)
	if err != nil {
		return nil, err
	}

	kubevipLBConfig := config.GetKubevipLBConfig(controllerCM)

	preferredIpv4ServiceIP := ""

	if allowShare {
		preferredIpv4ServiceIP = discoverSharedVIPs(service, servicePortMap)
	}

	// If allowedShare is true but no IP could be shared, or allowedShare is false, switch to use IPAM lookup
	loadBalancerIPs, err := discoverVIPs(service.Namespace, pool, preferredIpv4ServiceIP, inUseSet, kubevipLBConfig, service.Spec.IPFamilyPolicy, service.Spec.IPFamilies)
	if err != nil {
		return nil, err
	}

	// Get the loadbalancer interface if it's defined for the namespace
	var loadbalancerInterface string
	if len(loadBalancerIPs) > 0 {
		loadbalancerInterface = discoverInterface(controllerCM, service.Namespace)
	}

	// Update the services with this new address
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		recentService, getErr := kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}

		klog.Infof("Updating service [%s], with load balancer IPAM address(es) [%s]", service.Name, loadBalancerIPs)

		if recentService.Labels == nil {
			// Just because ..
			recentService.Labels = make(map[string]string)
		}
		// Set Label for service lookups
		recentService.Labels[ImplementationLabelKey] = ImplementationLabelValue

		if recentService.Annotations == nil {
			recentService.Annotations = make(map[string]string)
		}
		// use annotation to specify static IP, instead of spec.LoadbalancerIP, to support IPv6 dualstack.
		recentService.Annotations[LoadbalancerIPsAnnotation] = loadBalancerIPs

		// this line will be removed once kube-vip can recognize annotations
		// Set IPAM address to Load Balancer Service
		recentService.Spec.LoadBalancerIP = strings.Split(loadBalancerIPs, ",")[0]

		if len(loadbalancerInterface) > 0 {
			klog.Infof("Updating service [%s], with load balancer interface [%s]", service.Name, loadbalancerInterface)
			recentService.Annotations[LoadbalancerServiceInterfaceAnnotationKey] = loadbalancerInterface
		}

		// Update the actual service with the address and the labels
		_, updateErr := kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return nil, fmt.Errorf("error updating Service Spec [%s] : %v", service.Name, retryErr)
	}

	return &service.Status.LoadBalancer, nil
}

func getConfigWithNamespace(cm *v1.ConfigMap, namespace, name string) (value, key string, err error) {
	var ok bool

	key = fmt.Sprintf("%s-%s", name, namespace)

	if value, ok = cm.Data[key]; !ok {
		return "", key, fmt.Errorf("no config for %s", name)
	}

	return value, key, nil
}

func getConfig(cm *v1.ConfigMap, namespace, configMapName, name, configType string) (value string, global bool, err error) {
	var key string

	value, key, err = getConfigWithNamespace(cm, namespace, name)
	if err != nil {
		klog.Info(fmt.Errorf("no %s config for namespace [%s] exists in key [%s] configmap [%s]", name, namespace, key, configMapName))
		value, key, err = getConfigWithNamespace(cm, "global", name)
		if err != nil {
			klog.Info(fmt.Errorf("no global %s config exists [%s]", name, key))
		} else {
			klog.Infof("Taking %s from [%s]", configType, key)
			return value, true, nil
		}
	} else {
		klog.Infof("Taking %s from [%s]", configType, key)
		return value, false, nil
	}

	return "", false, fmt.Errorf("no config for %s", name)
}

func discoverPool(cm *v1.ConfigMap, namespace, configMapName string) (pool string, global bool, allowShare bool, err error) {
	var cidr, ipRange, allowShareStr string

	// Check for VIP sharing
	allowShareStr, _, err = getConfig(cm, namespace, configMapName, "allow-share", "config")
	if err == nil {
		allowShare, _ = strconv.ParseBool(allowShareStr)
	}

	// Find Cidr
	cidr, global, err = getConfig(cm, namespace, configMapName, "cidr", "address")
	if err == nil {
		return cidr, global, allowShare, nil
	}

	// Find Range
	ipRange, global, err = getConfig(cm, namespace, configMapName, "range", "address")
	if err == nil {
		return ipRange, global, allowShare, nil
	}

	return "", false, allowShare, fmt.Errorf("no address pools could be found")
}

// Multiplex addresses:
// 1. get all used VipEndpoints (addr and port)
// 2. build usedIpset
// 3. find an IP in usedIps where the requested VipEndpoints are available
//		if found: assign this IP and return. Services without a Ports account for the whole IP
//		if not: find new free IP from Range and assign it

func discoverSharedVIPs(service *v1.Service, servicePortMap map[string]*set.Set[int32]) (vips string) {
	servicePorts := set.New[int32]()
	for p := range service.Spec.Ports {
		servicePorts.Insert(service.Spec.Ports[p].Port)
	}

	for ip := range servicePortMap {
		portSet := *servicePortMap[ip]
		if portSet.Has(0) {
			continue
		}

		intersect := servicePorts.Intersection(portSet)
		if intersect.Len() == 0 {
			klog.Infof("Share service [%s] ports %s, with address [%s] ports %s",
				service.Name,
				fmt.Sprint(servicePorts.SortedList()),
				ip,
				fmt.Sprint(portSet.SortedList()),
			)
			// All requested ports are free on this IP
			return ip
		}
	}

	return ""
}

func discoverVIPsSingleStack(namespace, ipv4Pool, ipv6Pool string, preferredIpv4ServiceIP string, inUseIPSet *netipx.IPSet, kubevipLBConfig *config.KubevipLBConfig,
	ipFamilies []v1.IPFamily) (vips string, err error) {

	ipPool := ipv4Pool
	if len(ipFamilies) == 0 {
		if len(ipv4Pool) == 0 {
			ipPool = ipv6Pool
		}
	} else if ipFamilies[0] == v1.IPv6Protocol {
		ipPool = ipv6Pool
	}
	if len(ipPool) == 0 {
		return "", fmt.Errorf("could not find suitable pool for the IP family of the service")
	}
	if ipPool == ipv4Pool && len(preferredIpv4ServiceIP) > 0 {
		return preferredIpv4ServiceIP, nil
	}
	return discoverAddress(namespace, ipPool, inUseIPSet, kubevipLBConfig)

}

func discoverFromPool(namespace, pool, preferredIpv4ServiceIP, ipv4Pool string, inUseIPSet *netipx.IPSet, kubevipLBConfig *config.KubevipLBConfig, vipList *[]string) (poolError, err error) {
	if len(pool) == 0 {
		return nil, nil
	}

	var vip string
	if pool == ipv4Pool && len(preferredIpv4ServiceIP) > 0 {
		vip = preferredIpv4ServiceIP
	} else {
		vip, err = discoverAddress(namespace, pool, inUseIPSet, kubevipLBConfig)
	}

	if err == nil {
		*vipList = append(*vipList, vip)
		return nil, nil
	} else if _, outOfIPs := err.(*ipam.OutOfIPsError); outOfIPs {
		poolError = err
		return poolError, nil
	}
	return nil, err
}

func discoverVIPsDualStack(namespace, ipv4Pool, ipv6Pool string, preferredIpv4ServiceIP string, inUseIPSet *netipx.IPSet, kubevipLBConfig *config.KubevipLBConfig,
	ipFamilyPolicy *v1.IPFamilyPolicy, ipFamilies []v1.IPFamily) (vips string, err error) {

	var vipList []string

	if *ipFamilyPolicy == v1.IPFamilyPolicyRequireDualStack {
		// With RequireDualStack, we want to make sure both pools with both IP
		// families exist
		if len(ipv4Pool) == 0 || len(ipv6Pool) == 0 {
			return "", fmt.Errorf("service requires dual-stack, but the configuration does not have both IPv4 and IPv6 pools listed for the namespace")
		}
	}

	// Choose pool order
	primaryPool := ipv4Pool
	secondaryPool := ipv6Pool
	if len(ipFamilies) > 0 && ipFamilies[0] == v1.IPv6Protocol {
		primaryPool = ipv6Pool
		secondaryPool = ipv4Pool
	}

	// Provide VIPs from both IP families if possible (guaranteed if RequireDualStack)
	var primaryPoolErr, secondaryPoolErr error

	if len(primaryPool) > 0 {
		primaryPoolErr, err = discoverFromPool(namespace, primaryPool, preferredIpv4ServiceIP, ipv4Pool, inUseIPSet, kubevipLBConfig, &vipList)
		if err != nil {
			return "", err
		}
	}

	if len(secondaryPool) > 0 {
		secondaryPoolErr, err = discoverFromPool(namespace, secondaryPool, preferredIpv4ServiceIP, ipv4Pool, inUseIPSet, kubevipLBConfig, &vipList)
		if err != nil {
			return "", err
		}
	}

	if *ipFamilyPolicy == v1.IPFamilyPolicyPreferDualStack {
		if primaryPoolErr != nil && secondaryPoolErr != nil {
			return "", fmt.Errorf("could not allocate any IP address for PreferDualStack service: %s", renderErrors(primaryPoolErr, secondaryPoolErr))
		}
		singleError := primaryPoolErr
		if secondaryPoolErr != nil {
			singleError = secondaryPoolErr
		}
		if singleError != nil {
			klog.Warningf("PreferDualStack service will be single-stack because of error: %s", singleError)
		}
	} else if *ipFamilyPolicy == v1.IPFamilyPolicyRequireDualStack {
		if primaryPoolErr != nil || secondaryPoolErr != nil {
			return "", fmt.Errorf("could not allocate required IP addresses for RequireDualStack service: %s", renderErrors(primaryPoolErr, secondaryPoolErr))
		}
	}

	return strings.Join(vipList, ","), nil
}

func discoverVIPs(
	namespace, pool, preferredIpv4ServiceIP string, inUseIPSet *netipx.IPSet, kubevipLBConfig *config.KubevipLBConfig,
	ipFamilyPolicy *v1.IPFamilyPolicy, ipFamilies []v1.IPFamily,
) (vips string, err error) {
	var ipv4Pool, ipv6Pool string

	// Check if DHCP is required
	if pool == "0.0.0.0/32" {
		return "0.0.0.0", nil
		// Check if ip pool contains a cidr, if not assume it is a range
	} else if len(pool) == 0 {
		return "", fmt.Errorf("could not discover address: pool is not specified")
	} else if strings.Contains(pool, "/") {
		ipv4Pool, ipv6Pool, err = ipam.SplitCIDRsByIPFamily(pool)
	} else {
		ipv4Pool, ipv6Pool, err = ipam.SplitRangesByIPFamily(pool)
	}
	if err != nil {
		return "", err
	}

	if ipFamilyPolicy == nil || *ipFamilyPolicy == v1.IPFamilyPolicySingleStack {
		return discoverVIPsSingleStack(namespace, ipv4Pool, ipv6Pool, preferredIpv4ServiceIP, inUseIPSet, kubevipLBConfig, ipFamilies)
	}
	return discoverVIPsDualStack(namespace, ipv4Pool, ipv6Pool, preferredIpv4ServiceIP, inUseIPSet, kubevipLBConfig, ipFamilyPolicy, ipFamilies)
}

func discoverAddress(namespace, pool string, inUseIPSet *netipx.IPSet, kubevipLBConfig *config.KubevipLBConfig) (vip string, err error) {
	// Check if DHCP is required
	if pool == "0.0.0.0/32" {
		vip = "0.0.0.0"
		// Check if ip pool contains a cidr, if not assume it is a range
	} else if strings.Contains(pool, "/") {
		vip, err = ipam.FindAvailableHostFromCidr(namespace, pool, inUseIPSet, kubevipLBConfig)
		if err != nil {
			return "", err
		}
	} else {
		vip, err = ipam.FindAvailableHostFromRange(namespace, pool, inUseIPSet, kubevipLBConfig)
		if err != nil {
			return "", err
		}
	}

	return vip, err
}

func getKubevipImplementationLabel() string {
	return fmt.Sprintf("%s=%s", ImplementationLabelKey, ImplementationLabelValue)
}

func renderErrors(errs ...error) string {
	s := strings.Builder{}
	for _, err := range errs {
		if err != nil {
			s.WriteString(fmt.Sprintf("\n\t- %s", err))
		}
	}
	return s.String()
}

// found interface of that service from configmap.
// if not found, return ""
func discoverInterface(cm *v1.ConfigMap, svcNS string) string {
	if interfaceName, ok := cm.Data[fmt.Sprintf("%s-%s", config.ConfigMapServiceInterfacePrefix, svcNS)]; ok {
		return interfaceName
	}
	// fall back to global interface
	if interfaceName, ok := cm.Data[fmt.Sprintf("%s-global", config.ConfigMapServiceInterfacePrefix)]; ok {
		return interfaceName
	}

	return ""
}
