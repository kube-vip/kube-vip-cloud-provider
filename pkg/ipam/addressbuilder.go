package ipam

import (
	"fmt"
	"net/netip"
	"strings"

	"go4.org/netipx"
)

// buildHostsFromCidr - Builds a IPSet constructed from the cidr
func buildHostsFromCidr(cidr string) (*netipx.IPSet, error) {
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
