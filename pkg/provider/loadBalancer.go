package provider

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/ipam"
	"go4.org/netipx"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	cloudprovider "k8s.io/cloud-provider"

	"k8s.io/klog"
)

const (
	// this annotation is for specifying IPs for a loadbalancer
	// use plural for dual stack support in the future
	// Example: kube-vip.io/loadbalancerIPs: 10.1.2.3,fd00::100
	loadbalancerIPsAnnotations = "kube-vip.io/loadbalancerIPs"
	implementationLabelKey     = "implementation"
	implementationLabelValue   = "kube-vip"
	legacyIpamAddressLabelKey  = "ipam-address"
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
	if service.Labels[implementationLabelKey] == implementationLabelValue {
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
	if service.Spec.LoadBalancerIP != "" {
		if v, ok := service.Annotations[loadbalancerIPsAnnotations]; !ok || len(v) == 0 {
			klog.Warningf("service.Spec.LoadBalancerIP is defined but annotations '%s' is not, assume it's a legacy service, updates its annotations", loadbalancerIPsAnnotations)
			// assume it's legacy service, need to update the annotation.
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				recentService, getErr := kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
				if getErr != nil {
					return getErr
				}
				if recentService.Annotations == nil {
					recentService.Annotations = make(map[string]string)
				}
				recentService.Annotations[loadbalancerIPsAnnotations] = service.Spec.LoadBalancerIP
				// remove ipam-address label
				delete(recentService.Labels, legacyIpamAddressLabelKey)

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

	if v, ok := service.Annotations[loadbalancerIPsAnnotations]; ok && len(v) != 0 {
		klog.Infof("service '%s/%s' annotations '%s' is defined but service.Spec.LoadBalancerIP is not. Assume it's not legacy service", service.Namespace, service.Name, loadbalancerIPsAnnotations)
		// Set Label for service lookups
		if service.Labels == nil || service.Labels[implementationLabelKey] != implementationLabelValue {
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
				recentService.Labels[implementationLabelKey] = implementationLabelValue
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

	// Get the clound controller configuration map
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
	pool, global, err := discoverPool(controllerCM, service.Namespace, cmName)
	if err != nil {
		return nil, err
	}

	// Get all services in this namespace or globally, that have the correct label
	var svcs *v1.ServiceList
	if global {
		svcs, err = kubeClient.CoreV1().Services("").List(ctx, metav1.ListOptions{LabelSelector: getKubevipImplementationLabel()})
		if err != nil {
			return &service.Status.LoadBalancer, err
		}
	} else {
		svcs, err = kubeClient.CoreV1().Services(service.Namespace).List(ctx, metav1.ListOptions{LabelSelector: getKubevipImplementationLabel()})
		if err != nil {
			return &service.Status.LoadBalancer, err
		}
	}

	builder := &netipx.IPSetBuilder{}
	for x := range svcs.Items {
		if ip, ok := svcs.Items[x].Annotations[loadbalancerIPsAnnotations]; ok {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return nil, err
			}
			builder.Add(addr)
		}
	}
	inUseSet, err := builder.IPSet()
	if err != nil {
		return nil, err
	}

	descOrder := getSearchOrder(controllerCM)

	// If the LoadBalancer address is empty, then do a local IPAM lookup
	loadBalancerIPs, err := discoverVIPs(service.Namespace, pool, inUseSet, descOrder, service.Spec.IPFamilyPolicy, service.Spec.IPFamilies)
	if err != nil {
		return nil, err
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
		recentService.Labels[implementationLabelKey] = implementationLabelValue

		if recentService.Annotations == nil {
			recentService.Annotations = make(map[string]string)
		}
		// use annotation instead of label to support ipv6
		recentService.Annotations[loadbalancerIPsAnnotations] = loadBalancerIPs

		// this line will be removed once kube-vip can recognize annotations
		// Set IPAM address to Load Balancer Service
		recentService.Spec.LoadBalancerIP = strings.Split(loadBalancerIPs, ",")[0]

		// Update the actual service with the address and the labels
		_, updateErr := kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return nil, fmt.Errorf("error updating Service Spec [%s] : %v", service.Name, retryErr)
	}

	return &service.Status.LoadBalancer, nil
}

func discoverPool(cm *v1.ConfigMap, namespace, configMapName string) (pool string, global bool, err error) {
	var cidr, ipRange string
	var ok bool

	// Find Cidr
	cidrKey := fmt.Sprintf("cidr-%s", namespace)
	// Lookup current namespace
	if cidr, ok = cm.Data[cidrKey]; !ok {
		klog.Info(fmt.Errorf("no cidr config for namespace [%s] exists in key [%s] configmap [%s]", namespace, cidrKey, configMapName))
		// Lookup global cidr configmap data
		if cidr, ok = cm.Data["cidr-global"]; !ok {
			klog.Info(fmt.Errorf("no global cidr config exists [cidr-global]"))
		} else {
			klog.Infof("Taking address from [cidr-global] pool")
			return cidr, true, nil
		}
	} else {
		klog.Infof("Taking address from [%s] pool", cidrKey)
		return cidr, false, nil
	}

	// Find Range
	rangeKey := fmt.Sprintf("range-%s", namespace)
	// Lookup current namespace
	if ipRange, ok = cm.Data[rangeKey]; !ok {
		klog.Info(fmt.Errorf("no range config for namespace [%s] exists in key [%s] configmap [%s]", namespace, rangeKey, configMapName))
		// Lookup global range configmap data
		if ipRange, ok = cm.Data["range-global"]; !ok {
			klog.Info(fmt.Errorf("no global range config exists [range-global]"))
		} else {
			klog.Infof("Taking address from [range-global] pool")
			return ipRange, true, nil
		}
	} else {
		klog.Infof("Taking address from [%s] pool", rangeKey)
		return ipRange, false, nil
	}

	return "", false, fmt.Errorf("no address pools could be found")
}

func discoverVIPs(
	namespace, pool string, inUseIPSet *netipx.IPSet, descOrder bool,
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

	vipBuilder := strings.Builder{}

	// Handle single stack case
	if ipFamilyPolicy == nil || *ipFamilyPolicy == v1.IPFamilyPolicySingleStack {
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
		return discoverAddress(namespace, ipPool, inUseIPSet, descOrder)
	}

	// Handle dual stack case
	if *ipFamilyPolicy == v1.IPFamilyPolicyRequireDualStack {
		// With RequireDualStack, we want to make sure both pools with both IP
		// families exist
		if len(ipv4Pool) == 0 || len(ipv6Pool) == 0 {
			return "", fmt.Errorf("service requires dual-stack, but the configuration does not have both IPv4 and IPv6 pools listed for the namespace")
		}
	}

	primaryPool := ipv4Pool
	secondaryPool := ipv6Pool
	if len(ipFamilies) > 0 && ipFamilies[0] == v1.IPv6Protocol {
		primaryPool = ipv6Pool
		secondaryPool = ipv4Pool
	}
	// Provide VIPs from both IP families if possible (guaranteed if RequireDualStack)
	var primaryPoolErr, secondaryPoolErr error
	if len(primaryPool) > 0 {
		primaryVip, err := discoverAddress(namespace, primaryPool, inUseIPSet, descOrder)
		if err == nil {
			_, _ = vipBuilder.WriteString(primaryVip)
		} else if _, outOfIPs := err.(*ipam.OutOfIPsError); outOfIPs {
			primaryPoolErr = err
		} else {
			return "", err
		}
	}
	if len(secondaryPool) > 0 {
		secondaryVip, err := discoverAddress(namespace, secondaryPool, inUseIPSet, descOrder)
		if err == nil {
			if vipBuilder.Len() > 0 {
				vipBuilder.WriteByte(',')
			}
			_, _ = vipBuilder.WriteString(secondaryVip)
		} else if _, outOfIPs := err.(*ipam.OutOfIPsError); outOfIPs {
			secondaryPoolErr = err
		} else {
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

	return vipBuilder.String(), nil
}

func discoverAddress(namespace, pool string, inUseIPSet *netipx.IPSet, descOrder bool) (vip string, err error) {
	// Check if DHCP is required
	if pool == "0.0.0.0/32" {
		vip = "0.0.0.0"
		// Check if ip pool contains a cidr, if not assume it is a range
	} else if strings.Contains(pool, "/") {
		vip, err = ipam.FindAvailableHostFromCidr(namespace, pool, inUseIPSet, descOrder)
		if err != nil {
			return "", err
		}
	} else {
		vip, err = ipam.FindAvailableHostFromRange(namespace, pool, inUseIPSet, descOrder)
		if err != nil {
			return "", err
		}
	}

	return vip, err
}

func getKubevipImplementationLabel() string {
	return fmt.Sprintf("%s=%s", implementationLabelKey, implementationLabelValue)
}

func getSearchOrder(cm *v1.ConfigMap) (descOrder bool) {
	if searchOrder, ok := cm.Data["search-order"]; ok {
		if searchOrder == "desc" {
			return true
		}
	}
	return false
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
