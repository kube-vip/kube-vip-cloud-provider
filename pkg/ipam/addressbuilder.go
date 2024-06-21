package ipam

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/config"
	"go4.org/netipx"
)

// parseCidr - Builds an IPSet constructed from the cidrs
func parseCidrs(cidr string) (*netipx.IPSet, error) {
	// Split the ipranges (comma separated)
	cidrs := strings.Split(cidr, ",")
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("unable to parse IP cidrs [%s]", cidr)
	}

	builder := &netipx.IPSetBuilder{}

	for x := range cidrs {
		prefix, err := netip.ParsePrefix(cidrs[x])
		if err != nil {
			return nil, err
		}
		builder.AddPrefix(prefix)
	}
	return builder.IPSet()
}

// buildHostsFromCidr - Builds a IPSet constructed from the cidr and filters out
// the broadcast IP and network IP for IPv4 networks
func buildHostsFromCidr(cidr string, kubevipLBConfig *config.KubevipLBConfig) (*netipx.IPSet, error) {
	unfilteredSet, err := parseCidrs(cidr)
	if err != nil {
		return nil, err
	}

	builder := &netipx.IPSetBuilder{}
	for _, prefix := range unfilteredSet.Prefixes() {
		// If the prefix is IPv6 address, add it to the builder directly
		if !prefix.Addr().Is4() {
			builder.AddPrefix(prefix)
			continue
		}

		// Only skip the end IPs if skip-end-ips-in-cidr in configMap is set to true.
		if prefix.IsSingleIP() && kubevipLBConfig != nil && kubevipLBConfig.SkipEndIPsInCIDR {
			builder.Add(prefix.Addr())
			continue
		}

		if r := netipx.RangeOfPrefix(prefix); r.IsValid() {
			if prefix.Bits() == 31 {
				// rfc3021 Using 31-Bit Prefixes on IPv4 Point-to-Point Links
				builder.AddRange(netipx.IPRangeFrom(r.From(), r.To()))
				continue
			}

			from, to := r.From(), r.To()
			// For 192.168.0.200/23, 192.168.0.206 is the BroadcastIP, and 192.168.0.201 is the NetworkID
			if kubevipLBConfig != nil && kubevipLBConfig.SkipEndIPsInCIDR {
				from, to = from.Next(), to.Prev()
			}
			builder.AddRange(netipx.IPRangeFrom(from, to))
		}
	}
	return builder.IPSet()
}

// buildHostsFromRange - Builds a IPSet constructed from the Range
func buildAddressesFromRange(ipRangeString string) (*netipx.IPSet, error) {
	// Split the ipranges (comma separated)

	ranges := strings.Split(ipRangeString, ",")
	if len(ranges) == 0 {
		return nil, fmt.Errorf("unable to parse IP ranges [%s]", ipRangeString)
	}

	builder := &netipx.IPSetBuilder{}

	for x := range ranges {
		ipRange := strings.Split(ranges[x], "-")
		// Make sure we have x.x.x.x-x.x.x.x or x:x:x:x:x:x:x:x:x-x:x:x:x:x:x:x:x:x
		if len(ipRange) != 2 {
			return nil, fmt.Errorf("unable to parse IP range [%s]", ranges[x])
		}

		start, err := netip.ParseAddr(ipRange[0])
		if err != nil {
			return nil, err
		}
		end, err := netip.ParseAddr(ipRange[1])
		if err != nil {
			return nil, err
		}

		builder.AddRange(netipx.IPRangeFrom(start, end))
	}

	return builder.IPSet()
}

// SplitCIDRsByIPFamily splits the cidrs into separate lists of ipv4
// and ipv6 CIDRs
func SplitCIDRsByIPFamily(cidrs string) (ipv4 string, ipv6 string, err error) {
	ipPools, err := parseCidrs(cidrs)
	if err != nil {
		return "", "", err
	}
	ipv4Cidrs := strings.Builder{}
	ipv6Cidrs := strings.Builder{}
	for _, prefix := range ipPools.Prefixes() {
		cidrsToEdit := &ipv4Cidrs
		if prefix.Addr().Is6() {
			cidrsToEdit = &ipv6Cidrs
		}
		if cidrsToEdit.Len() > 0 {
			cidrsToEdit.WriteByte(',')
		}
		_, _ = cidrsToEdit.WriteString(prefix.String())
	}
	return ipv4Cidrs.String(), ipv6Cidrs.String(), nil
}

// SplitRangesByIPFamily splits the ipRangeString into separate lists of ipv4
// and ipv6 ranges
func SplitRangesByIPFamily(ipRangeString string) (ipv4 string, ipv6 string, err error) {
	ipPools, err := buildAddressesFromRange(ipRangeString)
	if err != nil {
		return "", "", err
	}
	ipv4Ranges := strings.Builder{}
	ipv6Ranges := strings.Builder{}
	for _, ipRange := range ipPools.Ranges() {
		rangeToEdit := &ipv4Ranges
		if ipRange.From().Is6() {
			rangeToEdit = &ipv6Ranges
		}
		if rangeToEdit.Len() > 0 {
			rangeToEdit.WriteByte(',')
		}
		_, _ = rangeToEdit.WriteString(ipRange.From().String())
		_ = rangeToEdit.WriteByte('-')
		_, _ = rangeToEdit.WriteString(ipRange.To().String())
	}
	return ipv4Ranges.String(), ipv6Ranges.String(), nil
}
