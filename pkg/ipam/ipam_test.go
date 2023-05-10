package ipam

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
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
			want:    []string{"192.168.0.253", "192.168.0.254", "192.168.1.1", "192.168.1.2"},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAddressesFromRange(tt.args.ipRangeString)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildHostsFromRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.ElementsMatch(t, got, tt.want) {
				t.Errorf("buildHostsFromRange() = %v, want %v", got, tt.want)
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
			name: "single entry, two address",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHostsFromCidr(tt.args.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildHostsFromCidr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.ElementsMatch(t, got, tt.want) {
				t.Errorf("buildHostsFromCidr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindAvailableHostFromRange(t *testing.T) {
	type args struct {
		namespace        string
		ipRange          string
		existingServices []string
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
			name: "single range, three addresses",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.12",
				existingServices: []string{},
			},
			want: "192.168.0.10",
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
			name: "two ranges, four addresses",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.11,192.168.1.20-192.168.1.21",
				existingServices: []string{"192.168.0.9", "192.168.0.10"},
			},
			want: "192.168.0.11",
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

			got, err := FindAvailableHostFromRange(tt.args.namespace, tt.args.ipRange, s)
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
			want: "192.168.0.200",
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
			name: "no ip available",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.255/30",
				existingServices: []string{"192.168.0.254", "192.168.0.252", "192.168.0.253"},
			},
			wantErr: true,
		},
		{
			name: "dual entry, overlap address",
			args: args{
				namespace:        "default2",
				cidr:             "192.168.0.200/30,192.168.0.200/29",
				existingServices: []string{"192.168.0.9", "192.168.0.10"},
			},
			want: "192.168.0.200",
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

			got, err := FindAvailableHostFromCidr(tt.args.namespace, tt.args.cidr, s)
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
