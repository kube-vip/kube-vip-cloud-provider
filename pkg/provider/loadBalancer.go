package provider

import (
	"context"
	"fmt"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/ipam"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	cloudprovider "k8s.io/cloud-provider"

	"k8s.io/klog"
)

type kubevipServices struct {
	Services []services `json:"services"`
}

type services struct {
	Vip         string `json:"vip"`
	Port        int    `json:"port"`
	Type        string `json:"type"`
	UID         string `json:"uid"`
	ServiceName string `json:"serviceName"`
}

//PlndrLoadBalancer -
type kubevipLoadBalancerManager struct {
	kubeClient     *kubernetes.Clientset
	nameSpace      string
	cloudConfigMap string
}

func newLoadBalancer(kubeClient *kubernetes.Clientset, ns, cm string) cloudprovider.LoadBalancer {
	return &kubevipLoadBalancerManager{
		kubeClient:     kubeClient,
		nameSpace:      ns,
		cloudConfigMap: cm,
	}
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

	// Retrieve the kube-vip configuration from it's namespace
	cm, err := k.GetConfigMap(ctx, KubeVipClientConfig, service.Namespace)
	if err != nil {
		return nil, true, nil
	}

	// Find the services configuration in the configMap
	svc, err := k.GetServices(cm)
	if err != nil {
		return nil, false, err
	}

	for x := range svc.Services {
		if svc.Services[x].UID == string(service.UID) {
			return &service.Status.LoadBalancer, true, nil

			// return &v1.LoadBalancerStatus{
			// 	Ingress: []v1.LoadBalancerIngress{
			// 		{
			// 			IP: svc.Services[x].Vip,
			// 		},
			// 	},
			// }, true, nil
		}
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

func (k *kubevipLoadBalancerManager) deleteLoadBalancer(ctx context.Context, service *v1.Service) error {
	klog.Infof("deleting service '%s' (%s)", service.Name, service.UID)

	// Get the kube-vip (client) configuration from it's namespace
	cm, err := k.GetConfigMap(ctx, KubeVipClientConfig, service.Namespace)
	if err != nil {
		klog.Errorf("The configMap [%s] doensn't exist", KubeVipClientConfig)
		return nil
	}
	// Find the services configuration in the configMap
	svc, err := k.GetServices(cm)
	if err != nil {
		klog.Errorf("The service [%s] in configMap [%s] doensn't exist", service.Name, KubeVipClientConfig)
		return nil
	}

	// Update the services configuration, by removing the  service
	updatedSvc := svc.delServiceFromUID(string(service.UID))
	if len(service.Status.LoadBalancer.Ingress) != 0 {
		err = ipam.ReleaseAddress(service.Namespace, service.Spec.LoadBalancerIP)
		if err != nil {
			klog.Errorln(err)
		}
	}
	// Update the configMap
	_, err = k.UpdateConfigMap(ctx, cm, updatedSvc)
	return err
}

func (k *kubevipLoadBalancerManager) syncLoadBalancer(ctx context.Context, service *v1.Service) (*v1.LoadBalancerStatus, error) {

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

	// Retrieve the kube-vip configuration map
	namespaceCM, err := k.GetConfigMap(ctx, KubeVipClientConfig, service.Namespace)
	if err != nil {
		klog.Errorf("Unable to retrieve kube-vip service cache from configMap [%s] in [%s]", KubeVipClientConfig, service.Namespace)
		// TODO - determine best course of action
		namespaceCM, err = k.CreateConfigMap(ctx, KubeVipClientConfig, service.Namespace)
		if err != nil {
			return nil, err
		}
	}

	// This function reconciles the load balancer state
	klog.Infof("syncing service '%s' (%s)", service.Name, service.UID)

	// Find the services configuration in the configMap
	svc, err := k.GetServices(namespaceCM)
	if err != nil {
		klog.Errorf("Unable to retrieve services from configMap [%s], [%s]", KubeVipClientConfig, err.Error())

		// TODO best course of action, currently we create a new services config
		svc = &kubevipServices{}
	}

	// Check for existing configuration

	existing := svc.findService(string(service.UID))
	if existing != nil {
		klog.Infof("found existing service '%s' (%s) with vip %s", service.Name, service.UID, existing.Vip)
		return &service.Status.LoadBalancer, nil

		// If this is 0.0.0.0 then it's a DHCP lease and we need to return that not the 0.0.0.0
		// if existing.Vip == "0.0.0.0" {
		// 	return &service.Status.LoadBalancer, nil
		// }

		// //
		// return &v1.LoadBalancerStatus{
		// 	Ingress: []v1.LoadBalancerIngress{
		// 		{
		// 			IP: existing.Vip,
		// 		},
		// 	},
		// }, nil
	}

	if service.Spec.LoadBalancerIP == "" {
		// If the LoadBalancer address is empty, then do a local IPAM lookup
		service.Spec.LoadBalancerIP, err = discoverAddress(controllerCM, service.Namespace, k.cloudConfigMap)
		if err != nil {
			return nil, err
		}
		// Update the services with this new address
		klog.Infof("Updating service [%s], with load balancer IPAM address [%s]", service.Name, service.Spec.LoadBalancerIP)
		_, err = k.kubeClient.CoreV1().Services(service.Namespace).Update(ctx, service, metav1.UpdateOptions{})
		if err != nil {
			// release the address internally as we failed to update service
			ipamerr := ipam.ReleaseAddress(service.Namespace, service.Spec.LoadBalancerIP)
			if ipamerr != nil {
				klog.Errorln(ipamerr)
			}
			return nil, fmt.Errorf("Error updating Service Spec [%s] : %v", service.Name, err)
		}
	}

	// TODO - manage more than one set of ports
	newSvc := services{
		ServiceName: service.Name,
		UID:         string(service.UID),
		Type:        string(service.Spec.Ports[0].Protocol),
		Vip:         service.Spec.LoadBalancerIP,
		Port:        int(service.Spec.Ports[0].Port),
	}

	svc.addService(newSvc)

	_, err = k.UpdateConfigMap(ctx, namespaceCM, svc)
	if err != nil {
		return nil, err
	}
	return &service.Status.LoadBalancer, nil
}

func discoverAddress(cm *v1.ConfigMap, namespace, configMapName string) (vip string, err error) {
	var cidr, ipRange string
	var ok bool

	// Find Cidr
	cidrKey := fmt.Sprintf("cidr-%s", namespace)
	// Lookup current namespace
	if cidr, ok = cm.Data[cidrKey]; !ok {
		klog.Info(fmt.Errorf("No cidr config for namespace [%s] exists in key [%s] configmap [%s]", namespace, cidrKey, configMapName))
		// Lookup global cidr configmap data
		if cidr, ok = cm.Data["cidr-global"]; !ok {
			klog.Info(fmt.Errorf("No global cidr config exists [cidr-global]"))
		} else {
			klog.Infof("Taking address from [cidr-global] pool")
		}
	} else {
		klog.Infof("Taking address from [%s] pool", cidrKey)
	}
	if ok {
		vip, err = ipam.FindAvailableHostFromCidr(namespace, cidr)
		if err != nil {
			return "", err
		}
		return
	}

	// Find Range
	rangeKey := fmt.Sprintf("range-%s", namespace)
	// Lookup current namespace
	if ipRange, ok = cm.Data[rangeKey]; !ok {
		klog.Info(fmt.Errorf("No range config for namespace [%s] exists in key [%s] configmap [%s]", namespace, rangeKey, configMapName))
		// Lookup global range configmap data
		if ipRange, ok = cm.Data["range-global"]; !ok {
			klog.Info(fmt.Errorf("No global range config exists [range-global]"))
		} else {
			klog.Infof("Taking address from [range-global] pool")
		}
	} else {
		klog.Infof("Taking address from [%s] pool", rangeKey)
	}
	if ok {
		vip, err = ipam.FindAvailableHostFromRange(namespace, ipRange)
		if err != nil {
			return vip, err
		}
		return
	}
	return "", fmt.Errorf("No IP address ranges could be found either range-global or range-<namespace>")
}
