package ipam

import (
	"fmt"
	"net"
	"strings"

	"k8s.io/klog"
)

// Manager - handles the addresses for each namespace/vip
var Manager []ipManager

// ipManager defines the mapping to a namespace and address pool
type ipManager struct {
	// Identifies the manager
	namespace string

	// The network configuration
	cidr    string
	ipRange string

	// todo - This confuses me ...
	addresses []string
}

// FindAvailableHostFromRange - will look through the cidr and the address Manager and find a free address (if possible)
func FindAvailableHostFromRange(namespace, ipRange string, existingServiceIPS []string) (string, error) {

	// Look through namespaces and update one if it exists
	for x := range Manager {
		if Manager[x].namespace == namespace {
			// Check that the address range is the same
			if Manager[x].ipRange != ipRange {
				klog.Infof("Updating IP address range from [%s] to [%s]", Manager[x].ipRange, ipRange)

				// If not rebuild the available hosts
				ah, err := buildAddressesFromRange(ipRange)
				if err != nil {
					return "", err
				}
				Manager[x].addresses = ah
				Manager[x].ipRange = ipRange
			}

			// TODO - currently we search (incrementally) through the list of hosts
			for y := range Manager[x].addresses {
				found := false
				for z := range existingServiceIPS {
					if Manager[x].addresses[y] == existingServiceIPS[z] {
						found = true
					}
				}
				if !found {
					return Manager[x].addresses[y], nil
				}
			}
			// If we have found the manager for this namespace and not returned an address then we've expired the range
			return "", fmt.Errorf("no addresses available in [%s] range [%s]", namespace, ipRange)

		}
	}
	ah, err := buildAddressesFromRange(ipRange)
	if err != nil {
		return "", err
	}
	// If it doesn't exist then it will need adding
	newManager := ipManager{
		namespace: namespace,
		addresses: ah,
		ipRange:   ipRange,
	}
	Manager = append(Manager, newManager)
	// TODO - currently we search (incrementally) through the list of hosts
	for y := range newManager.addresses {
		found := false
		for z := range existingServiceIPS {
			if newManager.addresses[y] == existingServiceIPS[z] {
				found = true
			}
		}
		if !found {
			return newManager.addresses[y], nil
		}
	}

	return "", fmt.Errorf("no addresses available in [%s] range [%s]", namespace, ipRange)

}

// FindAvailableHostFromCidr - will look through the cidr and the address Manager and find a free address (if possible)
func FindAvailableHostFromCidr(namespace, cidr string, existingServiceIPS []string) (string, error) {

	// Look through namespaces and update one if it exists
	for x := range Manager {
		if Manager[x].namespace == namespace {
			// Check that the address range is the same
			if Manager[x].cidr != cidr {
				// If not rebuild the available hosts
				ah, err := buildHostsFromCidr(cidr)
				if err != nil {
					return "", err
				}
				Manager[x].addresses = ah
				Manager[x].cidr = cidr

			}
			// TODO - currently we search (incrementally) through the list of hosts
			for y := range Manager[x].addresses {
				found := false
				for z := range existingServiceIPS {
					if Manager[x].addresses[y] == existingServiceIPS[z] {
						found = true
					}
				}
				if !found {
					return Manager[x].addresses[y], nil
				}
			}
			// If we have found the manager for this namespace and not returned an address then we've expired the range
			return "", fmt.Errorf("no addresses available in [%s] range [%s]", namespace, cidr)

		}
	}
	ah, err := buildHostsFromCidr(cidr)
	if err != nil {
		return "", err
	}
	// If it doesn't exist then it will need adding
	newManager := ipManager{
		namespace: namespace,
		addresses: ah,
		cidr:      cidr,
	}
	Manager = append(Manager, newManager)

	// TODO - currently we search (incrementally) through the list of hosts
	for y := range newManager.addresses {
		found := false
		for z := range existingServiceIPS {
			if newManager.addresses[y] == existingServiceIPS[z] {
				found = true
			}
		}
		if !found {
			return newManager.addresses[y], nil
		}
	}
	return "", fmt.Errorf("No addresses available in [%s] range [%s]", namespace, cidr)

}

// // RenewAddress - removes the mark on an address
// func RenewAddress(namespace, address string) {
// 	for x := range Manager {
// 		if Manager[x].namespace == namespace {
// 			// Make sure we update the address manager to mark this address in use.
// 			Manager[x].addressManager[address] = true
// 		}
// 	}
// }

// // ReleaseAddress - removes the mark on an address
// func ReleaseAddress(namespace, address string) error {
// 	for x := range Manager {
// 		if Manager[x].namespace == namespace {
// 			Manager[x].addressManager[address] = false
// 			return nil
// 		}
// 	}
// 	return fmt.Errorf("unable to release address [%s] in namespace [%s]", address, namespace)
// }

// buildHostsFromCidr - Builds a list of addresses in the cidr
func buildHostsFromCidr(cidr string) ([]string, error) {
	var ips []string

	// Split the ipranges (comma seperated)
	cidrs := strings.Split(cidr, ",")
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("Unable to parse IP cidrs [%s]", cidr)
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
