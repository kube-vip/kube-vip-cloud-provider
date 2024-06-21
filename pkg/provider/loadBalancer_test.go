package provider

import (
	"context"
	"net/netip"
	"testing"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/config"
	"github.com/stretchr/testify/assert"
	"go4.org/netipx"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_DiscoveryPoolCIDR(t *testing.T) {
	type args struct {
		data v1.ConfigMap
		cidr string
	}

	dummy := new(v1.ConfigMap)
	dummy.Data = map[string]string{}
	dummy.Data["cidr-dummystart"] = "172.16.0.1/24"
	dummy.Data["cidr-global"] = "192.168.1.1/24"
	dummy.Data["cidr-system"] = "10.10.10.8/29"
	dummy.Data["allow-share-system"] = "true"
	dummy.Data["cidr-dummyend"] = "172.16.0.2/24"
	dummy.Data["cidr-ipv6"] = "2001::10/127"

	tests := []struct {
		name       string
		args       args
		want       string
		allowShare bool
		wantBool   bool
		wantErr    bool
	}{
		{
			name: "cidr lookup for known namespace",
			args: args{
				*dummy,
				"system",
			},
			want:       "10.10.10.8/29",
			allowShare: true,
			wantBool:   false,
			wantErr:    false,
		},
		{
			name: "ipv6, cidr lookup for known namespace",
			args: args{
				*dummy,
				"ipv6",
			},
			want:     "2001::10/127",
			wantBool: false,
			wantErr:  false,
		},
		{
			name: "cidr lookup for unknown namespace",
			args: args{
				*dummy,
				"basic",
			},
			want:     "192.168.1.1/24",
			wantBool: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotString, gotBool, allowShare, err := discoverPool(&tt.args.data, tt.args.cidr, "") // #nosec G601
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverPool() error: %v, expected: %v", err, tt.wantErr)
				return
			}
			if !assert.EqualValues(t, gotString, tt.want) && !assert.EqualValues(t, gotBool, tt.wantBool) {
				t.Errorf("discoverPool() returned: %s : %v, expected: %s : %v", gotString, gotBool, tt.want, tt.wantBool)
			}
			if allowShare != tt.allowShare {
				t.Errorf("discoverPool() has invalid allowShare. expected: %v, got %v", tt.allowShare, allowShare)
			}
		})
	}
}

func Test_DiscoveryPoolRange(t *testing.T) {
	type args struct {
		data    v1.ConfigMap
		ipRange string
	}

	dummy := new(v1.ConfigMap)
	dummy.Data = map[string]string{}
	dummy.Data["range-dummystart"] = "172.16.0.1-172.16.0.254"
	dummy.Data["range-global"] = "192.168.1.1-192.168.1.254"
	dummy.Data["range-system"] = "10.10.10.8-10.10.10.15"
	dummy.Data["range-dummyend"] = "172.16.1.1-172.16.1.254"
	dummy.Data["cidr-ipv6"] = "2001::10-2001::20"

	tests := []struct {
		name     string
		args     args
		want     string
		wantBool bool
		wantErr  bool
	}{
		{
			name: "range lookup for known namespace",
			args: args{
				*dummy,
				"system",
			},
			want:     "10.10.10.8-10.10.10.15",
			wantBool: false,
			wantErr:  false,
		},
		{
			name: "ipv6, range lookup for known namespace",
			args: args{
				*dummy,
				"ipv6",
			},
			want:     "2001::10-2001::20",
			wantBool: false,
			wantErr:  false,
		},
		{
			name: "range lookup for unknown namespace",
			args: args{
				*dummy,
				"basic",
			},
			want:     "192.168.1.1-192.168.1.254",
			wantBool: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotString, gotBool, _, err := discoverPool(&tt.args.data, tt.args.ipRange, "") // #nosec G601
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverPool() error: %v, expected: %v", err, tt.wantErr)
				return
			}
			if !assert.EqualValues(t, gotString, tt.want) && !assert.EqualValues(t, gotBool, tt.wantBool) {
				t.Errorf("discoverPool() returned: %s : %v, expected: %s : %v", gotString, gotBool, tt.want, tt.wantBool)
			}
		})
	}
}

func Test_DiscoveryAddressCIDR(t *testing.T) {
	type args struct {
		namespace          string
		pool               string
		existingServiceIPS []string
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "available ip search for known namespace",
			args: args{
				"system",
				"10.10.10.8/29",
				[]string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.11",
			wantErr: false,
		},
		{
			name: "available ip search for unknown namespace",
			args: args{
				"unknown",
				"192.168.1.1/24",
				[]string{"10.10.10.8", "172.16.0.3", "192.168.1.1", "10.10.10.9", "10.10.10.10", "172.16.0.4", "192.168.1.2", "10.10.10.12"},
			},
			want:    "192.168.1.3",
			wantErr: false,
		},
		{
			name: "ipv6, available ip search for known namespace",
			args: args{
				"system",
				"fe80::10/126",
				[]string{"fe80::10", "fe80::11", "fe80::12"},
			},
			want:    "fe80::13",
			wantErr: false,
		},
		{
			name: "ipv6, available ip search for unknown namespace",
			args: args{
				"unknown",
				"fe80::10/126",
				[]string{"fe80::10", "fe80::11", "fe80::12"},
			},
			want:    "fe80::13",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &netipx.IPSetBuilder{}
			for i := range tt.args.existingServiceIPS {
				addr, err := netip.ParseAddr(tt.args.existingServiceIPS[i])
				if err != nil {
					t.Errorf("discoverAddress() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("discoverAddress() error = %v", err)
				return
			}

			gotString, err := discoverAddress(tt.args.namespace, tt.args.pool, s, &config.KubevipLBConfig{})
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverAddress() error: %v, expected: %v", err, tt.wantErr)
				return
			}
			if !assert.EqualValues(t, gotString, tt.want) {
				t.Errorf("discoverAddress() returned: %s, expected: %s", gotString, tt.want)
			}
		})
	}
}

func Test_DiscoveryAddressRange(t *testing.T) {
	type args struct {
		namespace          string
		pool               string
		existingServiceIPS []string
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "available ip search for known namespace",
			args: args{
				"system",
				"10.10.10.8-10.10.10.15",
				[]string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.11",
			wantErr: false,
		},
		{
			name: "available ip search for unknown namespace",
			args: args{
				"unknown",
				"192.168.1.1-192.168.1.254",
				[]string{"10.10.10.8", "172.16.0.3", "192.168.1.1", "10.10.10.9", "10.10.10.10", "172.16.0.4", "192.168.1.2", "10.10.10.12"},
			},
			want:    "192.168.1.3",
			wantErr: false,
		},
		{
			name: "available ip search for known namespace",
			args: args{
				"system",
				"fe80::ffff-fe80::1:3",
				[]string{"fe80::ffff", "fe80::1:0", "fe80::1:1"},
			},
			want:    "fe80::1:2",
			wantErr: false,
		},
		{
			name: "available ip search for unknown namespace",
			args: args{
				"unknown",
				"fe80::ffff-fe80::1:3",
				[]string{"fe80::ffff", "fe80::1:0", "fe80::1:1"},
			},
			want:    "fe80::1:2",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &netipx.IPSetBuilder{}
			for i := range tt.args.existingServiceIPS {
				addr, err := netip.ParseAddr(tt.args.existingServiceIPS[i])
				if err != nil {
					t.Errorf("discoverAddress() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("discoverAddress() error = %v", err)
				return
			}

			gotString, err := discoverAddress(tt.args.namespace, tt.args.pool, s, &config.KubevipLBConfig{})
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverAddress() error: %v, expected: %v", err, tt.wantErr)
				return
			}
			if !assert.EqualValues(t, gotString, tt.want) {
				t.Errorf("discoverAddress() returned: %s, expected: %s", gotString, tt.want)
			}
		})
	}
}

func ipFamilyPolicyPtr(p v1.IPFamilyPolicy) *v1.IPFamilyPolicy {
	return &p
}

func Test_discoverVIPs(t *testing.T) {
	type args struct {
		ipFamilyPolicy         *v1.IPFamilyPolicy
		ipFamilies             []v1.IPFamily
		pool                   string
		preferredIpv4ServiceIP string
		existingServiceIPS     []string
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "IPv4 pool",
			args: args{
				ipFamilyPolicy:     nil,
				ipFamilies:         nil,
				pool:               "10.10.10.8-10.10.10.15",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.11",
			wantErr: false,
		},
		{
			name: "IPv4 pool with IPv4 service",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicySingleStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol},
				pool:               "10.10.10.8-10.10.10.15",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.11",
			wantErr: false,
		},
		{
			name: "IPv4 pool with preferred IP",
			args: args{
				ipFamilyPolicy:         ipFamilyPolicyPtr(v1.IPFamilyPolicySingleStack),
				ipFamilies:             []v1.IPFamily{v1.IPv4Protocol},
				pool:                   "10.10.10.8-10.10.10.15",
				preferredIpv4ServiceIP: "10.10.10.9",
				existingServiceIPS:     []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.9",
			wantErr: false,
		},
		{
			name: "IPv6 pool",
			args: args{
				ipFamilyPolicy:     nil,
				ipFamilies:         nil,
				pool:               "fd00::1-fd00::10",
				existingServiceIPS: []string{"fd00::1", "fd00::2", "fd00::4"},
			},
			want:    "fd00::3",
			wantErr: false,
		},
		{
			name: "IPv6 pool with IPv6 service",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicySingleStack),
				ipFamilies:         []v1.IPFamily{v1.IPv6Protocol},
				pool:               "fd00::1-fd00::10",
				existingServiceIPS: []string{"fd00::1", "fd00::2", "fd00::4"},
			},
			want:    "fd00::3",
			wantErr: false,
		},
		{
			name: "IPv6 pool with IPv4 service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicySingleStack),
				ipFamilies:     []v1.IPFamily{v1.IPv4Protocol},
				pool:           "fd00::1-fd00::10",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "IPv4 pool with IPv6 service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicySingleStack),
				ipFamilies:     []v1.IPFamily{v1.IPv6Protocol},
				pool:           "10.10.10.8-10.10.10.15",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "IPv4 pool with PreferDualStack service",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.15",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.11",
			wantErr: false,
		},
		{
			name: "IPv4 pool with PreferDualStack service and preferred IPv4 service IP",
			args: args{
				ipFamilyPolicy:         ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:             []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:                   "10.10.10.8-10.10.10.15",
				preferredIpv4ServiceIP: "10.10.10.10",
				existingServiceIPS:     []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
			},
			want:    "10.10.10.10",
			wantErr: false,
		},
		{
			name: "IPv6 pool with PreferDualStack service",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "fd00::1-fd00::10",
				existingServiceIPS: []string{"fd00::1", "fd00::2", "fd00::4"},
			},
			want:    "fd00::3",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack service with no IP families explicitly specified",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				pool:           "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
			},
			want:    "10.10.10.8,fd00::1",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv4,IPv6 service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:           "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
			},
			want:    "10.10.10.8,fd00::1",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv6,IPv4 service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv6Protocol, v1.IPv4Protocol},
				pool:           "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
			},
			want:    "fd00::1,10.10.10.8",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv4,IPv6 service and preferred IPv4 service IP",
			args: args{
				ipFamilyPolicy:         ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:             []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:                   "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
				existingServiceIPS:     []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
				preferredIpv4ServiceIP: "10.10.10.8",
			},
			want:    "10.10.10.8,fd00::1",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv6,IPv4 service and preferred IPv4 service IP",
			args: args{
				ipFamilyPolicy:         ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:             []v1.IPFamily{v1.IPv6Protocol, v1.IPv4Protocol},
				pool:                   "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
				existingServiceIPS:     []string{"10.10.10.8", "10.10.10.9", "10.10.10.10", "10.10.10.12"},
				preferredIpv4ServiceIP: "10.10.10.8",
			},
			want:    "fd00::1,10.10.10.8",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv4,IPv6 service, but the IPv6 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"fd00::1", "fd00::2"},
			},
			want:    "10.10.10.8",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv4,IPv6 service, but the IPv4 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9"},
			},
			want:    "fd00::1",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv6,IPv4 service, but the IPv6 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"fd00::1", "fd00::2"},
			},
			want:    "10.10.10.8",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv6,IPv4 service, but the IPv4 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9"},
			},
			want:    "fd00::1",
			wantErr: false,
		},
		{
			name: "dualstack pool with PreferDualStack IPv4,IPv6 service, but no pools have available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9", "fd00::1", "fd00::2"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "dualstack pool with PreferDualStack IPv4,IPv6 service, but there is an invalid pool",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyPreferDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2,invalid-pool",
				existingServiceIPS: []string{},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "IPv4 pool with RequireDualStack service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:           "10.10.10.8-10.10.10.15",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "IPv6 pool with RequireDualStack service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:           "fd00::1-fd00::10",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "empty pool with RequireDualStack service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:           "",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "dualstack pool with RequireDualStack IPv4,IPv6 service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:           "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
			},
			want:    "10.10.10.8,fd00::1",
			wantErr: false,
		},
		{
			name: "dualstack pool with RequireDualStack IPv6,IPv4 service",
			args: args{
				ipFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:     []v1.IPFamily{v1.IPv6Protocol, v1.IPv4Protocol},
				pool:           "10.10.10.8-10.10.10.15,fd00::1-fd00::10",
			},
			want:    "fd00::1,10.10.10.8",
			wantErr: false,
		},
		{
			name: "dualstack pool with RequireDualStack IPv4,IPv6 service, but the IPv6 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"fd00::1", "fd00::2"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "dualstack pool with RequireDualStack IPv4,IPv6 service, but the IPv4 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "dualstack pool with RequireDualStack IPv6,IPv4 service, but the IPv6 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"fd00::1", "fd00::2"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "dualstack pool with RequireDualStack IPv6,IPv4 service, but the IPv4 pool has no available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "dualstack pool with RequireDualStack IPv4,IPv6 service, but no pools have available addresses",
			args: args{
				ipFamilyPolicy:     ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
				ipFamilies:         []v1.IPFamily{v1.IPv4Protocol, v1.IPv6Protocol},
				pool:               "10.10.10.8-10.10.10.9,fd00::1-fd00::2",
				existingServiceIPS: []string{"10.10.10.8", "10.10.10.9", "fd00::1", "fd00::2"},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &netipx.IPSetBuilder{}
			for i := range tt.args.existingServiceIPS {
				addr, err := netip.ParseAddr(tt.args.existingServiceIPS[i])
				if err != nil {
					t.Errorf("discoverVIP() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("discoverVIP() error = %v", err)
				return
			}

			gotString, err := discoverVIPs("discover-vips-test-ns", tt.args.pool, tt.args.preferredIpv4ServiceIP, s, &config.KubevipLBConfig{}, tt.args.ipFamilyPolicy, tt.args.ipFamilies)
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverVIP() error: %v, expected: %v", err, tt.wantErr)
				return
			}
			if !assert.EqualValues(t, tt.want, gotString) {
				t.Errorf("discoverVIP() returned: %s, expected: %s", gotString, tt.want)
			}
		})
	}
}

func Test_syncLoadBalancer(t *testing.T) {
	tests := []struct {
		name             string
		serviceNamespace string
		serviceName      string

		originalService v1.Service
		poolConfigMap   *v1.ConfigMap
		expectedService v1.Service
		wantErr         bool
	}{
		{
			name: "add new annotation to legacy service which already has spec.loadbalancerIP, remove legacy label",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
						"ipam-address":   "192.168.1.1",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "192.168.1.1",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
		},
		{
			name: "add new annotation and loadbalancerIP to new service",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{},
			},
			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeVipClientConfig,
					Namespace: KubeVipClientConfigNamespace,
				},
				Data: map[string]string{
					"cidr-global": "192.168.1.1/24",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "192.168.1.1",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
		},
		{
			name: "add new annotation to new service which doesn't have spec.loadbalancerIP, doesn't add spec.loadbalancerIP",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "192.168.1.1",
					},
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "192.168.1.1",
					},
				},
			},
		},
		{
			name: "ipv6, add new annotation and loadbalancerIP to new service",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{},
			},
			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeVipClientConfig,
					Namespace: KubeVipClientConfigNamespace,
				},
				Data: map[string]string{
					"cidr-global": "fe80::10/126",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "fe80::10",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "fe80::10",
				},
			},
		},
		{
			name: "create config map with different name and namespace",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{},
			},
			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "load-balancer",
					Namespace: "load-balancer-ns",
				},
				Data: map[string]string{
					"cidr-global": "192.168.1.1/24",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "192.168.1.1",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
		},
		{
			name: "dualstack loadbalancer",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{
					IPFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
					IPFamilies:     []v1.IPFamily{v1.IPv6Protocol, v1.IPv4Protocol},
				},
			},

			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeVipClientConfig,
					Namespace: KubeVipClientConfigNamespace,
				},
				Data: map[string]string{
					"cidr-global": "10.120.120.1/24,fe80::10/126",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "fe80::10,10.120.120.1",
					},
				},
				Spec: v1.ServiceSpec{
					IPFamilyPolicy: ipFamilyPolicyPtr(v1.IPFamilyPolicyRequireDualStack),
					IPFamilies:     []v1.IPFamily{v1.IPv6Protocol, v1.IPv4Protocol},
					LoadBalancerIP: "fe80::10",
				},
			},
		},
		{
			name: "service interface defined in global, service gets the interface config",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{},
			},
			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeVipClientConfig,
					Namespace: KubeVipClientConfigNamespace,
				},
				Data: map[string]string{
					"cidr-global":      "192.168.1.1/24",
					"interface-global": "eth0",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation:                 "192.168.1.1",
						LoadbalancerServiceInterfaceAnnotationKey: "eth0",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
		},
		{
			name: "service interface defined in service's namespace, service gets the interface config",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{},
			},
			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeVipClientConfig,
					Namespace: KubeVipClientConfigNamespace,
				},
				Data: map[string]string{
					"cidr-global":    "192.168.1.1/24",
					"interface-test": "eth0",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation:                 "192.168.1.1",
						LoadbalancerServiceInterfaceAnnotationKey: "eth0",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
		},
		{
			name: "service interface not defined in service's namespace, service doesn't get the interface config",
			originalService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
				},
				Spec: v1.ServiceSpec{},
			},
			poolConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeVipClientConfig,
					Namespace: KubeVipClientConfigNamespace,
				},
				Data: map[string]string{
					"cidr-global":    "192.168.1.1/24",
					"interface-what": "eth0",
				},
			},
			expectedService: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "name",
					Labels: map[string]string{
						"implementation": "kube-vip",
					},
					Annotations: map[string]string{
						LoadbalancerIPsAnnotation: "192.168.1.1",
					},
				},
				Spec: v1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := KubeVipClientConfigNamespace
			cm := KubeVipClientConfig
			if tt.poolConfigMap != nil {
				ns = tt.poolConfigMap.GetObjectMeta().GetNamespace()
				cm = tt.poolConfigMap.GetObjectMeta().GetName()
			}

			mgr := &kubevipLoadBalancerManager{
				kubeClient:     fake.NewSimpleClientset(),
				namespace:      ns,
				cloudConfigMap: cm,
			}

			// create dummy service
			_, err := mgr.kubeClient.CoreV1().Services("test").Create(context.Background(), &tt.originalService, metav1.CreateOptions{}) // #nosec G601
			if err != nil {
				t.Error(err)
			}

			// create pool if needed
			if tt.poolConfigMap != nil {
				_, err := mgr.kubeClient.CoreV1().ConfigMaps(ns).Create(context.Background(), tt.poolConfigMap, metav1.CreateOptions{})
				if err != nil {
					t.Error(err)
				}
			}

			_, err = syncLoadBalancer(context.Background(), mgr.kubeClient, &tt.originalService, cm, ns) // #nosec G601
			if err != nil {
				t.Error(err)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("syncLoadBalancer() error: %v, expected: %v", err, tt.wantErr)
				return
			}

			resService, err := mgr.kubeClient.CoreV1().Services("test").Get(context.Background(), "name", metav1.GetOptions{})
			if err != nil {
				t.Error(err)
			}

			assert.EqualValues(t, tt.expectedService, *resService)
		})
	}
}
