package provider

import cloudprovider "k8s.io/cloud-provider"

// Instances returns an instances interface. Also returns true if the interface is supported, false otherwise.
func (p *KubeVipCloudProvider) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 returns an instances interface. Also returns true if the interface is supported, false otherwise.
func (p *KubeVipCloudProvider) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return nil, false
}

// Zones returns a zones interface. Also returns true if the interface is supported, false otherwise.
func (p *KubeVipCloudProvider) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

// Clusters returns a clusters interface.  Also returns true if the interface is supported, false otherwise.
func (p *KubeVipCloudProvider) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Routes returns a routes interface along with whether the interface is supported.
func (p *KubeVipCloudProvider) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// HasClusterID provides an opportunity for cloud-provider-specific code to process DNS settings for pods.
func (p *KubeVipCloudProvider) HasClusterID() bool {
	return false
}
