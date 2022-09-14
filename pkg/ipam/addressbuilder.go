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

	// Split the ipranges (comma separated)
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

// IPStr2Int - Converts the IP address in string format to an integer
func IPStr2Int(ip string) uint {
	b := net.ParseIP(ip).To4()
	if b == nil {
		return 0
	}
	return uint(b[3]) | uint(b[2])<<8 | uint(b[1])<<16 | uint(b[0])<<24
}

// IPInt2Str - Converts the IP address in integer format to an string
func IPInt2Str(i uint) string {
	ip := make(net.IP, net.IPv4len)
	ip[0] = byte(i >> 24)
	ip[1] = byte(i >> 16)
	ip[2] = byte(i >> 8)
	ip[3] = byte(i)
	return ip.String()
}

// buildHostsFromRange - Builds a list of addresses in the cidr
func buildAddressesFromRange(ipRangeString string) ([]string, error) {
	var ips []string
	// Split the ipranges (comma separated)

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

		firstIP := IPStr2Int(ipRange[0])
		lastIP := IPStr2Int(ipRange[1])
		if firstIP > lastIP {
			// swap
			firstIP, lastIP = lastIP, firstIP
		}

		for ip := firstIP; ip <= lastIP; ip++ {
			if ipInvalid(ip) {
				ips = append(ips, IPInt2Str(ip))
			}
		}

		klog.Infof("Rebuilding addresse cache, [%d] addresses exist", len(ips))
	}
	return removeDuplicateAddresses(ips), nil
	//return ips, nil
}

func ipInvalid(ip uint) bool {
	var lastOctet = ip & 0x000000ff
	if lastOctet == 0x00 || lastOctet == 0xff {
		return false
	}
	return true
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
