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

const (
	// this annotation is for specifying IPs for a loadbalancer
	// use plural for dual stack support in the future
	// Example: kube-vip.io/loadbalancerIPs: 10.1.2.3
	loadbalancerIPsAnnotations = "kube-vip.io/loadbalancerIPs"
	implementationLabelKey     = "implementation"
	implementationLabelValue   = "kube-vip"
	legacyIpamAddressLabelKey  = "ipam-address"
)

// kubevipLoadBalancerManager -
type kubevipLoadBalancerManager struct {
	kubeClient     kubernetes.Interface
	nameSpace      string
	cloudConfigMap string
}

func newLoadBalancer(kubeClient kubernetes.Interface, ns, cm string) cloudprovider.LoadBalancer {
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
	if service.Labels[implementationLabelKey] == implementationLabelValue {
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
		if v, ok := service.Annotations[loadbalancerIPsAnnotations]; !ok || len(v) == 0 {
			klog.Warningf("service.Spec.LoadBalancerIP is defined but annotations '%s' is not, assume it's a legacy service, updates its annotations", loadbalancerIPsAnnotations)
			// assume it's legacy service, need to update the annotation.
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				recentService, getErr := k.kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
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
				_, updateErr := k.kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
				return updateErr
			})
			if err != nil {
				return nil, fmt.Errorf("error updating Service Spec [%s] : %v", service.Name, err)
			}
		}
		return &service.Status.LoadBalancer, nil
	}

	// Get the clound controller configuration map
	controllerCM, err := k.GetConfigMap(ctx, KubeVipClientConfig, KubeVipClientConfigNamespace)
	if err != nil {
		klog.Errorf("Unable to retrieve kube-vip ipam config from configMap [%s] in %s", KubeVipClientConfig, KubeVipClientConfigNamespace)
		// TODO - determine best course of action, create one if it doesn't exist
		controllerCM, err = k.CreateConfigMap(ctx, KubeVipClientConfig, KubeVipClientConfigNamespace)
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
		svcs, err = k.kubeClient.CoreV1().Services("").List(ctx, metav1.ListOptions{LabelSelector: getKubevipImplementationLabel()})
		if err != nil {
			return &service.Status.LoadBalancer, err
		}
	} else {
		svcs, err = k.kubeClient.CoreV1().Services(service.Namespace).List(ctx, metav1.ListOptions{LabelSelector: getKubevipImplementationLabel()})
		if err != nil {
			return &service.Status.LoadBalancer, err
		}
	}

	var existingServiceIPS []string
	for x := range svcs.Items {
		existingServiceIPS = append(existingServiceIPS, svcs.Items[x].Annotations[loadbalancerIPsAnnotations])
	}

	// If the LoadBalancer address is empty, then do a local IPAM lookup
	loadBalancerIP, err := discoverAddress(service.Namespace, pool, existingServiceIPS)

	if err != nil {
		return nil, err
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
		recentService.Labels[implementationLabelKey] = implementationLabelValue

		if recentService.Annotations == nil {
			recentService.Annotations = make(map[string]string)
		}
		// use annotation instead of label to support ipv6
		recentService.Annotations[loadbalancerIPsAnnotations] = loadBalancerIP

		// this line will be removed once kube-vip can recognize annotations
		// Set IPAM address to Load Balancer Service
		recentService.Spec.LoadBalancerIP = loadBalancerIP

		// Update the actual service with the address and the labels
		_, updateErr := k.kubeClient.CoreV1().Services(recentService.Namespace).Update(ctx, recentService, metav1.UpdateOptions{})
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

func getKubevipImplementationLabel() string {
	return fmt.Sprintf("%s=%s", implementationLabelKey, implementationLabelValue)
}
