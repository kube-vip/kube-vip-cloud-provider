package config

import v1 "k8s.io/api/core/v1"

const (
	// ConfigMapSearchOrderKey is the key in the ConfigMap that defines whether IPs are allocated from the beginning or from the end.
	ConfigMapSearchOrderKey = "search-order"

	// ConfigMapSkipStartIPsKey is the key in the ConfigMap that has the IPs to skip at the start and end of the CIDR
	ConfigMapSkipEndIPsKey = "skip-end-ips-in-cidr"

	// ConfigMapServiceInterfacePrefix is prefix of the key in the ConfigMap for specifying the service interface for that namespace
	ConfigMapServiceInterfacePrefix = "interface"
)

// KubevipLBConfig defines the configuration for the kube-vip load balancer in the kubevip configMap
// TODO: move all config into here so that it can be easily accessed and processed
type KubevipLBConfig struct {
	ReturnIPInDescOrder bool
	SkipEndIPsInCIDR    bool
}

// GetKubevipLBConfig returns the KubevipLBConfig from the ConfigMap
func GetKubevipLBConfig(cm *v1.ConfigMap) *KubevipLBConfig {
	c := &KubevipLBConfig{}
	if searchOrder, ok := cm.Data[ConfigMapSearchOrderKey]; ok {
		if searchOrder == "desc" {
			c.ReturnIPInDescOrder = true
		}
	}
	if skip, ok := cm.Data[ConfigMapSkipEndIPsKey]; ok {
		if skip == "true" {
			c.SkipEndIPsInCIDR = true
		}
	}
	return c
}
