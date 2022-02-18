package ipam

import (
	"fmt"
	"net"
	"strings"

	"k8s.io/klog"
)

func (i *ipManager) rebuildRange() error {
	var ips []string
	// Split the ipranges (comma seperated)
	ranges := strings.Split(i.ipRange, ",")
	if len(ranges) == 0 {
		return fmt.Errorf("unable to parse IP ranges [%s]", i.ipRange)
	}

	for x := range ranges {
		ipRange := strings.Split(ranges[x], "-")
		// Make sure we have x.x.x.x-x.x.x.x
		if len(ipRange) != 2 {
			return fmt.Errorf("unable to parse IP range [%s]", ranges[x])
		}
		startRange := net.ParseIP(ipRange[0]).To4()
		endRange := net.ParseIP(ipRange[1]).To4()
		//parse the ranges to make sure we don't end in a crazy loop
		if startRange[0] > endRange[0] {
			return fmt.Errorf("first octet of start range [%d] is higher then the ending range [%d]", startRange[0], endRange[0])
		}
		if startRange[1] > endRange[1] {
			return fmt.Errorf("second octet of start range [%d] is higher then the ending range [%d]", startRange[1], endRange[1])
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
	return nil
}
