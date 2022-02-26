package ipam

import (
	"fmt"
	"net"
	"strings"

	"k8s.io/klog"
)

// buildHostsFromCidr - Builds a list of addresses in the cidr
func buildHostsFromCidr(cidr string) ([]string, error) {
	var ips []string

	// Split the ipranges (comma seperated)
	cidrs := strings.Split(cidr, ",")
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("unable to parse IP cidrs [%s]", cidr)
	}

	for x := range cidrs {

		ip, ipnet, err := net.ParseCIDR(cidrs[x])
		if err != nil {
			return nil, err
		}

		var cidrips []string
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			cidrips = append(cidrips, ip.String())
		}

		// remove network address and broadcast address
		lenIPs := len(cidrips)
		switch {
		case lenIPs < 2:
			ips = append(ips, cidrips...)

		default:
			ips = append(ips, cidrips[1:len(cidrips)-1]...)
		}
	}
	return removeDuplicateAddresses(ips), nil
}

// buildHostsFromRange - Builds a list of addresses in the cidr
func buildAddressesFromRange(ipRangeString string) ([]string, error) {
	var ips []string
	// Split the ipranges (comma seperated)
	ranges := strings.Split(ipRangeString, ",")
	if len(ranges) == 0 {
		return nil, fmt.Errorf("unable to parse IP ranges [%s]", ipRangeString)
	}

	for x := range ranges {
		ipRange := strings.Split(ranges[x], "-")
		// Make sure we have x.x.x.x-x.x.x.x
		if len(ipRange) != 2 {
			return nil, fmt.Errorf("unable to parse IP range [%s]", ranges[x])
		}
		startRange := net.ParseIP(ipRange[0]).To4()
		endRange := net.ParseIP(ipRange[1]).To4()
		//parse the ranges to make sure we don't end in a crazy loop
		if startRange[0] > endRange[0] {
			return nil, fmt.Errorf("first octet of start range [%d] is higher then the ending range [%d]", startRange[0], endRange[0])
		}
		if startRange[1] > endRange[1] {
			return nil, fmt.Errorf("second octet of start range [%d] is higher then the ending range [%d]", startRange[1], endRange[1])
		}
		ips = append(ips, startRange.String())

		// Break the lowerBytes to an array, also check for octet boundaries and increment other octets
		for !startRange.Equal(endRange) {
			if startRange[3] == 254 {
				startRange[2]++
				startRange[3] = 0
			}
			startRange[3]++
			ips = append(ips, startRange.String())
		}
		klog.Infof("Rebuilding addresse cache, [%d] addresses exist", len(ips))
	}
	return removeDuplicateAddresses(ips), nil
	//return ips, nil
}

func removeDuplicateAddresses(arr []string) []string {
	addresses := map[string]bool{}
	uniqueAddresses := []string{} // Keep all keys from the map into a slice.

	// iterate over all addresses from range, add them to a map if not found
	// then add them to the unique addresses afterwards
	for i := range arr {
		if !addresses[arr[i]] {
			addresses[arr[i]] = true
			uniqueAddresses = append(uniqueAddresses, arr[i])

		}
	}
	return uniqueAddresses
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
