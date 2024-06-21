package ipam

import (
	"net/netip"
	"testing"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/config"
	"go4.org/netipx"
)

func Test_buildHostsFromRange(t *testing.T) {
	type args struct {
		ipRangeString string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "single address",
			args: args{
				"192.168.0.10-192.168.0.10",
			},
			want:    []string{"192.168.0.10"},
			wantErr: false,
		},
		{
			name: "single range, three addresses",
			args: args{
				"192.168.0.10-192.168.0.12",
			},
			want:    []string{"192.168.0.10", "192.168.0.11", "192.168.0.12"},
			wantErr: false,
		},
		{
			name: "single range, across third octet",
			args: args{
				"192.168.0.253-192.168.1.2",
			},
			want:    []string{"192.168.0.253", "192.168.0.254", "192.168.0.255", "192.168.1.0", "192.168.1.1", "192.168.1.2"},
			wantErr: false,
		},
		{
			name: "two ranges, four addresses",
			args: args{
				"192.168.0.10-192.168.0.11,192.168.1.20-192.168.1.21",
			},
			want:    []string{"192.168.0.10", "192.168.0.11", "192.168.1.20", "192.168.1.21"},
			wantErr: false,
		},
		{
			name: "two ranges, four addresses w/overlap",
			args: args{
				"192.168.0.10-192.168.0.11,192.168.0.10-192.168.0.13",
			},
			want:    []string{"192.168.0.10", "192.168.0.11", "192.168.0.12", "192.168.0.13"},
			wantErr: false,
		},
		{
			name: "ipv6, two ips",
			args: args{
				"fe80::13-fe80::14",
			},
			want:    []string{"fe80::13", "fe80::14"},
			wantErr: false,
		},
		{
			name: "ipv6, single range, across third octet",
			args: args{
				"fe80::ffff-fe80::1:3",
			},
			want:    []string{"fe80::ffff", "fe80::1:0", "fe80::1:1", "fe80::1:2", "fe80::1:3"},
			wantErr: false,
		},
		{
			name: "ipv6, two ranges, 5 addresses",
			args: args{
				"fe80::10-fe80::12,fe81::13-fe81::14",
			},
			want:    []string{"fe80::10", "fe80::11", "fe80::12", "fe81::13", "fe81::14"},
			wantErr: false,
		},
		{
			name: "ipv6, two ranges, 5 addresses w/overlap",
			args: args{
				"fe80::10-fe80::12,fe80::10-fe80::14",
			},
			want:    []string{"fe80::10", "fe80::11", "fe80::12", "fe80::13", "fe80::14"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAddressesFromRange(tt.args.ipRangeString)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildHostsFromRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			builder := &netipx.IPSetBuilder{}
			for i := range tt.want {
				addr, err := netip.ParseAddr(tt.want[i])
				if err != nil {
					t.Errorf("buildAddressesFromRange() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("buildAddressesFromRange() error = %v", err)
				return
			}

			if !got.Equal(s) {
				t.Errorf("buildHostsFromRange() = %v, want %v", got.Prefixes(), tt.want)
			}
		})
	}
}

func Test_buildHostsFromCidr(t *testing.T) {
	type args struct {
		cidr  string
		kvlbc *config.KubevipLBConfig
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "single entry, /32, 1 address",
			args: args{
				cidr: "192.168.0.200/32",
			},
			want:    []string{"192.168.0.200"},
			wantErr: false,
		},
		{
			name: "single entry, /32, 1 address, if skipEndIPsInCIDR is set",
			args: args{
				cidr:  "192.168.0.200/32",
				kvlbc: &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want:    []string{"192.168.0.200"},
			wantErr: false,
		},
		{
			name: "single entry, 4 address",
			args: args{
				cidr: "192.168.0.200/30",
			},
			want:    []string{"192.168.0.200", "192.168.0.201", "192.168.0.202", "192.168.0.203"},
			wantErr: false,
		},
		{
			name: "single entry, 2 address, if skipEndIPsInCIDR is set",
			args: args{
				cidr:  "192.168.0.200/30",
				kvlbc: &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want:    []string{"192.168.0.201", "192.168.0.202"},
			wantErr: false,
		},
		{
			name: "single entry, /31, 2 address, if skipEndIPsInCIDR is set",
			args: args{
				cidr:  "192.168.0.200/31",
				kvlbc: &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want:    []string{"192.168.0.200", "192.168.0.201"},
			wantErr: false,
		},
		{
			name: "single entry, /31, 2 address, if skipEndIPsInCIDR is set",
			args: args{
				cidr:  "192.168.0.200/31",
				kvlbc: &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want:    []string{"192.168.0.200", "192.168.0.201"},
			wantErr: false,
		},
		{
			name: "dual entry, overlap address, return 8 addresses",
			args: args{
				cidr: "192.168.0.200/30,192.168.0.200/29",
			},
			want:    []string{"192.168.0.200", "192.168.0.201", "192.168.0.202", "192.168.0.203", "192.168.0.204", "192.168.0.205", "192.168.0.206", "192.168.0.207"},
			wantErr: false,
		},
		{
			name: "dual entry, overlap addressm return 6 addresses, if skipEndIPsInCIDR is set",
			args: args{
				cidr:  "192.168.0.200/30,192.168.0.200/29",
				kvlbc: &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want:    []string{"192.168.0.201", "192.168.0.202", "192.168.0.203", "192.168.0.204", "192.168.0.205", "192.168.0.206"},
			wantErr: false,
		},
		{
			name: "ipv6, two ips",
			args: args{
				cidr: "fe80::10/127",
			},
			want:    []string{"fe80::10", "fe80::11"},
			wantErr: false,
		},
		{
			name: "ipv6, two cidrs",
			args: args{
				cidr: "fe80::10/127,fe80::fe/127",
			},
			want:    []string{"fe80::10", "fe80::11", "fe80::fe", "fe80::ff"},
			wantErr: false,
		},
		{
			name: "ipv6, two cidrs with overlap",
			args: args{
				cidr: "fe80::10/126,fe80::12/127",
			},
			want:    []string{"fe80::10", "fe80::11", "fe80::12", "fe80::13"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHostsFromCidr(tt.args.cidr, tt.args.kvlbc)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildHostsFromCidr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			builder := &netipx.IPSetBuilder{}
			for i := range tt.want {
				addr, err := netip.ParseAddr(tt.want[i])
				if err != nil {
					t.Errorf("buildAddressesFromRange() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("buildHostsFromCidr() error = %v", err)
				return
			}

			if !got.Equal(s) {
				t.Errorf("buildHostsFromCidr() = %v, want %v", got.Ranges(), tt.want)
			}
		})
	}
}

func TestSplitCIDRsByIPFamily(t *testing.T) {
	type args struct {
		cidrs string
	}
	type output struct {
		ipv4Cidrs string
		ipv6Cidrs string
	}
	tests := []struct {
		name    string
		args    args
		want    output
		wantErr bool
	}{
		{
			name: "single ipv4 cidr",
			args: args{
				"192.168.0.200/30",
			},
			want: output{
				ipv4Cidrs: "192.168.0.200/30",
				ipv6Cidrs: "",
			},
			wantErr: false,
		},
		{
			name: "multiple ipv4 cidrs",
			args: args{
				"192.168.0.200/30,192.168.1.200/30",
			},
			want: output{
				ipv4Cidrs: "192.168.0.200/30,192.168.1.200/30",
				ipv6Cidrs: "",
			},
			wantErr: false,
		},
		{
			name: "single ipv6 cidr",
			args: args{
				"fe80::10/127",
			},
			want: output{
				ipv4Cidrs: "",
				ipv6Cidrs: "fe80::10/127",
			},
			wantErr: false,
		},
		{
			name: "multiple ipv6 cidrs",
			args: args{
				"fe80::10/127,fe80::fe/127",
			},
			want: output{
				ipv4Cidrs: "",
				ipv6Cidrs: "fe80::10/127,fe80::fe/127",
			},
			wantErr: false,
		},
		{
			name: "one ipv4 cidr and one ipv6 cidr",
			args: args{
				"192.168.0.200/30,fe80::10/127",
			},
			want: output{
				ipv4Cidrs: "192.168.0.200/30",
				ipv6Cidrs: "fe80::10/127",
			},
			wantErr: false,
		},
		{
			name: "multiple ipv4 cidrs and multiple ipv6 cidrs",
			args: args{
				"192.168.0.200/30,192.168.1.200/30,fe80::10/127,fe80::fe/127",
			},
			want: output{
				ipv4Cidrs: "192.168.0.200/30,192.168.1.200/30",
				ipv6Cidrs: "fe80::10/127,fe80::fe/127",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ipv4Cidrs, ipv6Cidrs, err := SplitCIDRsByIPFamily(tt.args.cidrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("SplitCIDRsByIPFamily() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if ipv4Cidrs != tt.want.ipv4Cidrs || ipv6Cidrs != tt.want.ipv6Cidrs {
				t.Errorf("SplitCIDRsByIPFamily() = {ipv4Cidrs: %v, ipv6Cidrs: %v}, want %+v", ipv4Cidrs, ipv6Cidrs, tt.want)
			}
		})
	}
}

func TestSplitRangesByIPFamily(t *testing.T) {
	type args struct {
		ipRangeString string
	}
	type output struct {
		ipv4Ranges string
		ipv6Ranges string
	}
	tests := []struct {
		name    string
		args    args
		want    output
		wantErr bool
	}{
		{
			name: "single ipv4 range",
			args: args{
				"192.168.0.10-192.168.0.12",
			},
			want: output{
				ipv4Ranges: "192.168.0.10-192.168.0.12",
				ipv6Ranges: "",
			},
			wantErr: false,
		},
		{
			name: "multiple ipv4 ranges",
			args: args{
				"192.168.0.10-192.168.0.12,192.168.0.100-192.168.0.120",
			},
			want: output{
				ipv4Ranges: "192.168.0.10-192.168.0.12,192.168.0.100-192.168.0.120",
				ipv6Ranges: "",
			},
			wantErr: false,
		},
		{
			name: "single ipv6 range",
			args: args{
				"fe80::13-fe80::14",
			},
			want: output{
				ipv4Ranges: "",
				ipv6Ranges: "fe80::13-fe80::14",
			},
			wantErr: false,
		},
		{
			name: "multiple ipv6 ranges",
			args: args{
				"fe80::13-fe80::14,fe80::130-fe80::140",
			},
			want: output{
				ipv4Ranges: "",
				ipv6Ranges: "fe80::13-fe80::14,fe80::130-fe80::140",
			},
			wantErr: false,
		},
		{
			name: "one ipv4 range and one ipv6 range",
			args: args{
				"192.168.0.10-192.168.0.12,fe80::13-fe80::14",
			},
			want: output{
				ipv4Ranges: "192.168.0.10-192.168.0.12",
				ipv6Ranges: "fe80::13-fe80::14",
			},
			wantErr: false,
		},
		{
			name: "multiple ipv4 ranges and multiple ipv6 ranges",
			args: args{
				"192.168.0.10-192.168.0.12,192.168.0.100-192.168.0.120,fe80::13-fe80::14,fe80::130-fe80::140",
			},
			want: output{
				ipv4Ranges: "192.168.0.10-192.168.0.12,192.168.0.100-192.168.0.120",
				ipv6Ranges: "fe80::13-fe80::14,fe80::130-fe80::140",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ipv4Ranges, ipv6Ranges, err := SplitRangesByIPFamily(tt.args.ipRangeString)
			if (err != nil) != tt.wantErr {
				t.Errorf("SplitRangesByIPFamily() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if ipv4Ranges != tt.want.ipv4Ranges || ipv6Ranges != tt.want.ipv6Ranges {
				t.Errorf("SplitRangesByIPFamily() = {ipv4Ranges: %v, ipv6Ranges: %v}, want %+v", ipv4Ranges, ipv6Ranges, tt.want)
			}
		})
	}
}

func TestFindAvailableHostFromRange(t *testing.T) {
	type args struct {
		namespace        string
		ipRange          string
		existingServices []string
		descOrder        bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "simple range",
			args: args{
				namespace:        "default",
				ipRange:          "192.168.0.10-192.168.0.10",
				existingServices: []string{},
			},
			want: "192.168.0.10",
		},
		{
			name: "simple range, reverse order",
			args: args{
				namespace:        "default",
				ipRange:          "192.168.0.10-192.168.0.10",
				existingServices: []string{},
				descOrder:        true,
			},
			want: "192.168.0.10",
		},
		{
			name: "single range, three addresses",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.12",
				existingServices: []string{},
			},
			want: "192.168.0.10",
		},
		{
			name: "single range, three addresses, reverse order",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.12",
				existingServices: []string{},
				descOrder:        true,
			},
			want: "192.168.0.12",
		},
		{
			name: "single range, across third octet",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.253-192.168.1.2",
				existingServices: []string{"192.168.0.253", "192.168.0.254"},
			},
			want: "192.168.1.1",
		},
		{
			name: "single range, across third octet, reverse order",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.253-192.168.1.2",
				existingServices: []string{"192.168.1.1", "192.168.1.2"},
				descOrder:        true,
			},
			want: "192.168.0.254",
		},
		{
			name: "two ranges, four addresses",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.11,192.168.1.20-192.168.1.21",
				existingServices: []string{"192.168.0.9", "192.168.0.10"},
			},
			want: "192.168.0.11",
		},
		{
			name: "two ranges, four addresses, reverse order",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.11,192.168.1.20-192.168.1.22",
				existingServices: []string{"192.168.1.21", "192.168.1.22"},
				descOrder:        true,
			},
			want: "192.168.1.20",
		},
		{
			name: "ipv6, simple range",
			args: args{
				namespace:        "default",
				ipRange:          "fe80::13-fe80::14",
				existingServices: []string{},
			},
			want: "fe80::13",
		},
		{
			name: "ipv6, simple range, reverse order",
			args: args{
				namespace:        "default",
				ipRange:          "fe80::13-fe80::14",
				existingServices: []string{},
				descOrder:        true,
			},
			want: "fe80::14",
		},
		{
			name: "ipv6, single range, three addresses",
			args: args{
				namespace:        "default2",
				ipRange:          "fe80::13-fe80::15",
				existingServices: []string{},
			},
			want: "fe80::13",
		},
		{
			name: "ipv6, single range, three addresses, reverse order",
			args: args{
				namespace:        "default2",
				ipRange:          "fe80::13-fe80::15",
				existingServices: []string{},
				descOrder:        true,
			},
			want: "fe80::15",
		},
		{
			name: "ipv6, single range, across third octet",
			args: args{
				namespace:        "default2",
				ipRange:          "fe80::ffff-fe80::1:3",
				existingServices: []string{"fe80::ffff"},
			},
			want: "fe80::1:0",
		},
		{
			name: "ipv6, single range, across third octet, reverse order",
			args: args{
				namespace:        "default2",
				ipRange:          "fe80::ffff-fe80::1:3",
				existingServices: []string{"fe80::1:3", "fe80::1:2", "fe80::1:1", "fe80::1:0"},
				descOrder:        true,
			},
			want: "fe80::ffff",
		},
		{
			name: "ipv6, two ranges, 5 addresses",
			args: args{
				namespace:        "default2",
				ipRange:          "fe80::10-fe80::12,fe81::20-fe81::21",
				existingServices: []string{"fe80::10", "fe80::11", "fe80::12"},
			},
			want: "fe81::20",
		},
		{
			name: "ipv6, two ranges, 5 addresses, reverse order",
			args: args{
				namespace:        "default2",
				ipRange:          "fe80::10-fe80::12,fe81::20-fe81::21",
				existingServices: []string{"fe81::21", "fe81::20"},
				descOrder:        true,
			},
			want: "fe80::12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &netipx.IPSetBuilder{}
			for i := range tt.args.existingServices {
				addr, err := netip.ParseAddr(tt.args.existingServices[i])
				if err != nil {
					t.Errorf("FindAvailableHostFromRange() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("FindAvailableHostFromRange() error = %v", err)
				return
			}

			got, err := FindAvailableHostFromRange(tt.args.namespace, tt.args.ipRange, s, &config.KubevipLBConfig{ReturnIPInDescOrder: tt.args.descOrder})
			if (err != nil) != tt.wantErr {
				t.Errorf("FindAvailableHostFromRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FindAvailableHostFromRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindAvailableHostFromCIDR(t *testing.T) {
	type args struct {
		namespace        string
		cidr             string
		existingServices []string
		kvlbc            *config.KubevipLBConfig
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "single entry, 4 addresses, allocate the first one",
			args: args{
				namespace:        "default",
				cidr:             "192.168.0.200/30",
				existingServices: []string{},
			},
			want: "192.168.0.200",
		},
		{
			name: "single entry, 4 addresses, allocate second address if SkipEndIPsInCIDR is set",
			args: args{
				namespace:        "default",
				cidr:             "192.168.0.200/30",
				existingServices: []string{},
				kvlbc:            &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want: "192.168.0.201",
		},
		{
			name: "single entry, two address, reverse ip order",
			args: args{
				namespace:        "default",
				cidr:             "192.168.0.200/30",
				existingServices: []string{},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			want: "192.168.0.203",
		},
		{
			name: "single entry, two address, reverse ip order, allocate second last address if SkipEndIPsInCIDR is set",
			args: args{
				namespace:        "default",
				cidr:             "192.168.0.200/30",
				existingServices: []string{},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true, SkipEndIPsInCIDR: true},
			},
			want: "192.168.0.202",
		},
		{
			name: "simple cidr, cidr contains .0 and .255",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.10/24",
				existingServices: []string{},
			},
			want: "192.168.0.1",
		},
		{
			name: "simple cidr, cidr contains .0 and .255, reverse order",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.10/24",
				existingServices: []string{},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			want: "192.168.0.254",
		},
		{
			name: "no ip available",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.255/30",
				existingServices: []string{"192.168.0.254", "192.168.0.252", "192.168.0.253"},
			},
			wantErr: true,
		},
		{
			name: "no ip available, reverse order",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.255/30",
				existingServices: []string{"192.168.0.254", "192.168.0.252", "192.168.0.253"},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			wantErr: true,
		},
		{
			name: "dual entry, overlap address",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.200/30,192.168.0.200/29",
				existingServices: []string{"192.168.0.201", "192.168.0.202"},
			},
			want: "192.168.0.200",
		},
		{
			name: "dual entry, overlap address, set SkipEndIPsInCIDR, pick next available address after first one",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.200/30,192.168.0.200/29",
				existingServices: []string{"192.168.0.201", "192.168.0.202"},
				kvlbc:            &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want: "192.168.0.203",
		},
		{
			name: "dual entry, overlap address, reverse order, pick next available address from last",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.200/30,192.168.0.200/29",
				existingServices: []string{"192.168.0.201", "192.168.0.202"},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			want: "192.168.0.207",
		},
		{
			name: "dual entry, overlap address, reverse order, set SkipEndIPsInCIDR, pick next available address before last",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.200/30,192.168.0.200/29",
				existingServices: []string{"192.168.0.201", "192.168.0.202"},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true, SkipEndIPsInCIDR: true},
			},
			want: "192.168.0.206",
		},
		{
			name: "ipv6, single entry, two address",
			args: args{
				namespace:        "default",
				cidr:             "2001::49fe/127",
				existingServices: []string{},
			},
			want: "2001::49fe",
		},
		{
			name: "ipv6, single entry, two address, set SkipEndIPsInCIDR no effect",
			args: args{
				namespace:        "default",
				cidr:             "2001::49fe/127",
				existingServices: []string{},
				kvlbc:            &config.KubevipLBConfig{SkipEndIPsInCIDR: true},
			},
			want: "2001::49fe",
		},
		{
			name: "ipv6, single entry, two address, reverse order",
			args: args{
				namespace:        "default",
				cidr:             "2001::49fe/127",
				existingServices: []string{},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			want: "2001::49ff",
		},
		{
			name: "ipv6, no ip available",
			args: args{
				namespace:        "default2",
				cidr:             "2001::49fe/127",
				existingServices: []string{"2001::49fe", "2001::49ff"},
			},
			wantErr: true,
		},
		{
			name: "ipv6, no ip available, reverse order",
			args: args{
				namespace:        "default2",
				cidr:             "2001::49fe/127",
				existingServices: []string{"2001::49fe", "2001::49ff"},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			wantErr: true,
		},
		{
			name: "ipv6, dual entry, overlap address",
			args: args{
				namespace:        "default2",
				cidr:             "2001::10/126,2001::12/127",
				existingServices: []string{"2001::10", "2001::11"},
			},
			want: "2001::12",
		},
		{
			name: "ipv6, dual entry, overlap address, reverse order",
			args: args{
				namespace:        "default2",
				cidr:             "2001::10/126,2001::12/127",
				existingServices: []string{"2001::10", "2001::11"},
				kvlbc:            &config.KubevipLBConfig{ReturnIPInDescOrder: true},
			},
			want: "2001::13",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &netipx.IPSetBuilder{}
			for i := range tt.args.existingServices {
				addr, err := netip.ParseAddr(tt.args.existingServices[i])
				if err != nil {
					t.Errorf("FindAvailableHostFromCIDR() error = %v", err)
					return
				}
				builder.Add(addr)
			}
			s, err := builder.IPSet()
			if err != nil {
				t.Errorf("FindAvailableHostFromCIDR() error = %v", err)
				return
			}
			got, err := FindAvailableHostFromCidr(tt.args.namespace, tt.args.cidr, s, tt.args.kvlbc)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindAvailableHostFromCIDR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FindAvailableHostFromCIDR() = %v, want %v", got, tt.want)
			}
			// clean up the ipManager so it doesn't impact other test
			Manager = []ipManager{}
		})
	}
}
