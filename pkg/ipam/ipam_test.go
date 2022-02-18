package ipam

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.11",
				existingServices: []string{""},
			},
			want: "192.168.0.10",
		},
		{
			name: "simple range",
			args: args{
				namespace:        "default2",
				ipRange:          "192.168.0.10-192.168.0.11",
				existingServices: []string{""},
			},
			want: "192.168.0.10",
		},
	}
	// Manager = append(Manager, ipManager{
	// 	ipRange:   "192.168.0.10-192.168.0.11",
	// 	namespace: "default",
	// })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindAvailableHostFromRange(tt.args.namespace, tt.args.ipRange, tt.args.existingServices)
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
