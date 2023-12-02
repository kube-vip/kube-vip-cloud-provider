package ipam

import (
	"net/netip"
	"testing"

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
		cidr string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "single entry, 4 address",
			args: args{
				"192.168.0.200/30",
			},
			want:    []string{"192.168.0.201", "192.168.0.202"},
			wantErr: false,
		},
		{
			name: "dual entry, overlap address",
			args: args{
				"192.168.0.200/30,192.168.0.200/29",
			},
			want:    []string{"192.168.0.201", "192.168.0.202", "192.168.0.203", "192.168.0.204", "192.168.0.205", "192.168.0.206"},
			wantErr: false,
		},
		{
			name: "ipv6, two ips",
			args: args{
				"fe80::10/127",
			},
			want:    []string{"fe80::10", "fe80::11"},
			wantErr: false,
		},
		{
			name: "ipv6, two cidrs",
			args: args{
				"fe80::10/127,fe80::fe/127",
			},
			want:    []string{"fe80::10", "fe80::11", "fe80::fe", "fe80::ff"},
			wantErr: false,
		},
		{
			name: "ipv6, two cidrs with overlap",
			args: args{
				"fe80::10/126,fe80::12/127",
			},
			want:    []string{"fe80::10", "fe80::11", "fe80::12", "fe80::13"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHostsFromCidr(tt.args.cidr)
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
			name: "simple range, revert",
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
			name: "single range, three addresses, revert",
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
			name: "single range, across third octet, revert",
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
			name: "two ranges, four addresses, revert",
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
			name: "ipv6, simple range, revert",
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
			name: "ipv6, single range, three addresses, revert",
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
			name: "ipv6, single range, across third octet, revert",
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
			name: "ipv6, two ranges, 5 addresses, revert",
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

			got, err := FindAvailableHostFromRange(tt.args.namespace, tt.args.ipRange, s, tt.args.descOrder)
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
		descOrder        bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "single entry, two address",
			args: args{
				namespace:        "default",
				cidr:             "192.168.0.200/30",
				existingServices: []string{},
			},
			want: "192.168.0.201",
		},
		{
			name: "single entry, two address, revert",
			args: args{
				namespace:        "default",
				cidr:             "192.168.0.200/30",
				existingServices: []string{},
				descOrder:        true,
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
			name: "simple cidr, cidr contains .0 and .255, revert",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.10/24",
				existingServices: []string{},
				descOrder:        true,
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
			name: "no ip available, revert",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.255/30",
				existingServices: []string{"192.168.0.254", "192.168.0.252", "192.168.0.253"},
				descOrder:        true,
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
			want: "192.168.0.203",
		},
		{
			name: "dual entry, overlap address, revert",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.200/30,192.168.0.200/29",
				existingServices: []string{"192.168.0.201", "192.168.0.202"},
				descOrder:        true,
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
			name: "ipv6, single entry, two address, revert",
			args: args{
				namespace:        "default",
				cidr:             "2001::49fe/127",
				existingServices: []string{},
				descOrder:        true,
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
			name: "ipv6, no ip available, revert",
			args: args{
				namespace:        "default2",
				cidr:             "2001::49fe/127",
				existingServices: []string{"2001::49fe", "2001::49ff"},
				descOrder:        true,
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
			name: "ipv6, dual entry, overlap address, revert",
			args: args{
				namespace:        "default2",
				cidr:             "2001::10/126,2001::12/127",
				existingServices: []string{"2001::10", "2001::11"},
				descOrder:        true,
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

			got, err := FindAvailableHostFromCidr(tt.args.namespace, tt.args.cidr, s, tt.args.descOrder)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindAvailableHostFromCIDR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FindAvailableHostFromCIDR() = %v, want %v", got, tt.want)
			}
		})
	}
}
