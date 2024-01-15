package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/ipam"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	cloudprovider "k8s.io/cloud-provider"

	"k8s.io/klog"
)

// kubevipLoadBalancerManager -
type kubevipLoadBalancerManager struct {
	kubeClient     *kubernetes.Clientset
	nameSpace      string
	cloudConfigMap string
}

type VipDetail struct {
	VsAddress    string `json:"vs_address,omitempty"`
	ResourceName string `json:"resource_name,omitempty"`
	ListenPort   string `json:"listen_port,omitempty"`
}

func newLoadBalancer(kubeClient *kubernetes.Clientset, ns, cm string) cloudprovider.LoadBalancer {
	k := &kubevipLoadBalancerManager{
		kubeClient:     kubeClient,
		nameSpace:      ns,
		cloudConfigMap: cm,
	}
	return k
}

func (k *kubevipLoadBalancerManager) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (lbs *v1.LoadBalancerStatus, err error) {
	return k.syncLoadBalancer(ctx, service)
}
func (k *kubevipLoadBalancerManager) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (err error) {
	_, err = k.syncLoadBalancer(ctx, service)
	return err
}

func (k *kubevipLoadBalancerManager) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	return k.deleteLoadBalancer(ctx, service)
}

func (k *kubevipLoadBalancerManager) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	if service.Labels["implementation"] == "kube-vip" {
		return &service.Status.LoadBalancer, true, nil
	}
	return nil, false, nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (k *kubevipLoadBalancerManager) GetLoadBalancerName(_ context.Context, clusterName string, service *v1.Service) string {
	return getDefaultLoadBalancerName(service)
}

func getDefaultLoadBalancerName(service *v1.Service) string {
	return cloudprovider.DefaultLoadBalancerName(service)
}

// nolint
func (k *kubevipLoadBalancerManager) deleteLoadBalancer(ctx context.Context, service *v1.Service) error {
	klog.Infof("deleting service '%s' (%s)", service.Name, service.UID)

	return nil
}

// syncLoadBalancer
// 1. Is this loadBalancer already created, and does it have an address? return status
// 2. Is this a new loadBalancer (with no IP address)
// 2a. Get all existing kube-vip services
// 2b. Get the network configuration for this service (namespace) / (CIDR/Range)
// 2c. Between the two find a free address

func (k *kubevipLoadBalancerManager) syncLoadBalancer(ctx context.Context, service *v1.Service) (*v1.LoadBalancerStatus, error) {
	// This function reconciles the load balancer state
	klog.Infof("syncing service '%s' (%s)", service.Name, service.UID)

	// The loadBalancer address has already been populated
	if service.Spec.LoadBalancerIP != "" {
		return &service.Status.LoadBalancer, nil
	}

	// Get the clound controller configuration map
	controllerCM, err := k.GetConfigMap(ctx, KubeVipClientConfig, "kube-system")
	if err != nil {
		klog.Errorf("Unable to retrieve kube-vip ipam config from configMap [%s] in kube-system", KubeVipClientConfig)
		// TODO - determine best course of action, create one if it doesn't exist
		controllerCM, err = k.CreateConfigMap(ctx, KubeVipClientConfig, "kube-system")
		if err != nil {
			return nil, err
		}
	}

	// Get ip pool from configmap and determine if it is namespace specific or global
	pool, global, err := discoverPool(controllerCM, service.Namespace, k.cloudConfigMap)

	if err != nil {
		return nil, err
	}

	// Get all services in this namespace or globally, that have the correct label
	var svcs *v1.ServiceList
	if global {
		svcs, err = k.kubeClient.CoreV1().Services("").List(ctx, metav1.ListOptions{LabelSelector: "implementation=kube-vip"})
		if err != nil {
			return &service.Status.LoadBalancer, err
		}
	} else {
		svcs, err = k.kubeClient.CoreV1().Services(service.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "implementation=kube-vip"})
		if err != nil {
			return &service.Status.LoadBalancer, err
		}
	}

	/*check ports isconflict*/
	var loadBalancerIP string = ""
	var isDupAddress bool = false

	usedVip := getAllVsUsedVip(svcs)
	usableVipList := getUsabletVip(usedVip, service.Spec.Ports)

	if len(usableVipList) > 0 {
		loadBalancerIP = usableVipList[0]
		isDupAddress = true
	} else {
		var existingServiceIPS []string
		for x := range svcs.Items {
			existingServiceIPS = append(existingServiceIPS, svcs.Items[x].Labels["ipam-address"])
		}

		// If the LoadBalancer address is empty, then do a local IPAM lookup
		loadBalancerIP, err = discoverAddress(service.Namespace, pool, existingServiceIPS)
		if err != nil {
			return nil, err
		}
	}

	// Update the services with this new address
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		recentService, getErr := k.kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}

		klog.Infof("Updating service [%s], with load balancer IPAM address [%s]", service.Name, loadBalancerIP)

		if recentService.Labels == nil {
			// Just because ..
			recentService.Labels = make(map[string]string)
		}
		// Set Label for service lookups
		recentService.Labels["implementation"] = "kube-vip"
		recentService.Labels["ipam-address"] = loadBalancerIP

		// Set IPAM address to Load Balancer Service
		recentService.Spec.LoadBalancerIP = loadBalancerIP

		if isDupAddress {
			if recentService.Annotations == nil {
				recentService.Annotations = make(map[string]string)
			}
			recentService.Annotations["kube-vip.io/ignore"] = "true"
			recentService.Spec.ExternalIPs = append(recentService.Spec.ExternalIPs, loadBalancerIP)
		}

		// Update the actual service with the address and the labels
		_, updateErr := k.kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return nil, fmt.Errorf("error updating Service Spec [%s] : %v", service.Name, err)
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

func discoverAddress(namespace, pool string, existingServiceIPS []string) (vip string, err error) {
	// Check if DHCP is required
	if pool == "0.0.0.0/32" {
		vip = "0.0.0.0"
		// Check if ip pool contains a cidr, if not assume it is a range
	} else if strings.Contains(pool, "/") {
		vip, err = ipam.FindAvailableHostFromCidr(namespace, pool, existingServiceIPS)
		if err != nil {
			return "", err
		}
	} else {
		vip, err = ipam.FindAvailableHostFromRange(namespace, pool, existingServiceIPS)
		if err != nil {
			return "", err
		}
	}

	return vip, err
}

func getUsabletVip(usedVip []VipDetail, newPorts []v1.ServicePort) []string {

	usableVip := make([]string, 0)

	curPorts := getVsListenerPort(newPorts)

	var checkUsedIpCache = make(map[string]bool)
	for _, usedIpObj := range usedVip {

		if comparePorts(curPorts, usedIpObj.ListenPort) {
			newvip := usedIpObj.VsAddress
			if checkUsedIpCache[newvip] {
				usableVip = delUsableVip(usableVip, newvip)
			}
			checkUsedIpCache[newvip] = true
			continue
		} else {

			newvip := usedIpObj.VsAddress
			if !checkUsedIpCache[newvip] {
				usableVip = append(usableVip, newvip)
			}
			checkUsedIpCache[newvip] = true
		}
	}
	return usableVip
}

func delUsableVip(usableVip []string, raw string) []string {
	var newUsableVip []string
	for _, elem := range usableVip {
		if elem != raw {
			newUsableVip = append(newUsableVip, elem)
		}
	}
	return newUsableVip
}

func getAllVsUsedVip(svclist *v1.ServiceList) []VipDetail {

	var vipUsedList []VipDetail
	var vip VipDetail

	for _, vs := range svclist.Items {
		objectMeta := metav1.ObjectMeta(vs.ObjectMeta)

		if vs.Spec.Type == v1.ServiceTypeLoadBalancer && objectMeta.Labels["ipam-address"] != "" {
			lbIngressIp := getLbIPFromStatus(&vs)
			if len(lbIngressIp) >= 1 {
				vip.VsAddress = lbIngressIp[0]
			}
			vip.ResourceName = fmt.Sprintf("%s_%s", vs.Name, vs.Namespace)
			vip.ListenPort = getVsListenerPort(vs.Spec.Ports)
			vipUsedList = append(vipUsedList, vip)
		}
	}

	return vipUsedList
}

func getLbIPFromStatus(svc *v1.Service) []string {
	vsStatus := svc.Status
	ingressIps := make([]string, 0)

	for _, ingress := range vsStatus.LoadBalancer.Ingress {
		if ingress.IP != "" {
			ingressIps = append(ingressIps, ingress.IP)
		}
	}

	if len(svc.Spec.ExternalIPs) > 0 {
		ingressIps = append(ingressIps, svc.Spec.ExternalIPs...)
	}

	return ingressIps
}

func getVsListenerPort(apiPorts []v1.ServicePort) string {
	var portsListener string = ""

	for _, port := range apiPorts {
		switch port.Protocol {
		case v1.ProtocolTCP:
			if portsListener != "" {
				portsListener += ","
			}
			portsListener += fmt.Sprintf("%d/%s", port.Port, port.Protocol)

		case v1.ProtocolUDP:
			if portsListener != "" {
				portsListener += ","
			}
			portsListener += fmt.Sprintf("%d/%s", port.Port, port.Protocol)

		case v1.ProtocolSCTP:
			if portsListener != "" {
				portsListener += ","
			}
			portsListener += fmt.Sprintf("%d/%s", port.Port, port.Protocol)

		}

	}
	return portsListener
}

func comparePorts(srcPorts, dstPorts string) bool {
	var src_ports = strings.Split(srcPorts, ",")
	if len(src_ports) == 0 {
		return false
	}

	var dst_ports = strings.Split(dstPorts, ",")
	if len(dst_ports) == 0 {
		return false
	}

	for _, src_port := range src_ports {
		for _, dst_port := range dst_ports {
			if src_port == dst_port {
				return true
			}
		}
	}

	return false
}
