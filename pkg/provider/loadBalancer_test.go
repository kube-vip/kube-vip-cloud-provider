package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
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
	dummy.Data["cidr-dummyend"] = "172.16.0.2/24"

	tests := []struct {
		name     string
		args     args
		want     string
		wantBool bool
		wantErr  bool
	}{
		{
			name: "cidr lookup for known namespace",
			args: args{
				*dummy,
				"system",
			},
			want:     "10.10.10.8/29",
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
			gotString, gotBool, err := discoverPool(&tt.args.data, tt.args.cidr, "")
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
			gotString, gotBool, err := discoverPool(&tt.args.data, tt.args.ipRange, "")
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
				"system",
				"192.168.1.1/24",
				[]string{"10.10.10.8", "172.16.0.3", "192.168.1.1", "10.10.10.9", "10.10.10.10", "172.16.0.4", "192.168.1.2", "10.10.10.12"},
			},
			want:    "192.168.1.3",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotString, err := discoverAddress(tt.args.namespace, tt.args.pool, tt.args.existingServiceIPS)
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
				"system",
				"192.168.1.1-192.168.1.254",
				[]string{"10.10.10.8", "172.16.0.3", "192.168.1.1", "10.10.10.9", "10.10.10.10", "172.16.0.4", "192.168.1.2", "10.10.10.12"},
			},
			want:    "192.168.1.3",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotString, err := discoverAddress(tt.args.namespace, tt.args.pool, tt.args.existingServiceIPS)
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
