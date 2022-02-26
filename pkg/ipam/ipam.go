package ipam

import (
	"fmt"

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
	return "", fmt.Errorf("no addresses available in [%s] range [%s]", namespace, cidr)

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
