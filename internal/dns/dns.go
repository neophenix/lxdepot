// Package dns is for our 3rd party DNS integrations
package dns

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/neophenix/lxdepot/internal/config"
)

// RecordList is a simple look at DNS records used as a common return for our interface
type RecordList struct {
	Name      string   // the name of the entry
	RecordSet []string // the values in the entry
}

// The DNS interface provides the list of functions all our 3rd party integrations should
// support.  I don't like that I coded the record type in the name, but until I decide
// I need IPv6, etc its good enough
type DNS interface {
	GetARecord(name string, networkBlocks []string) (string, error) // returns a string representation of an IPv4 address
	RemoveARecord(name string) error                                // removes the record from our 3rd party
	ListARecords() ([]RecordList, error)                            // returns a list of all the A records
}

// DNSOptions holds the various options from the main config we might want to use, this does
// mean these values are in multiple places, which is odd but they dont' change execpt on restart (today)
var DNSOptions config.DNS

// New should just hand back the appropriate interface for our config settings,
// returning from the correct "New" function for our integration
func New(conf *config.Config) DNS {
	DNSOptions = conf.DNS

	if conf.DNS.Provider == "google" {
		return NewGoogleDNS(conf.DNS.Options["gcp_creds_file"], conf.DNS.Options["gcp_project_name"], conf.DNS.Options["gcp_zone_name"])
	} else if conf.DNS.Provider == "amazon" {
		return NewAmazonDNS(conf.DNS.Options["aws_creds_file"], conf.DNS.Options["aws_creds_profile"], conf.DNS.Options["aws_zone_id"])
	}

	return nil
}

// findFreeARecord takes a populated list of octets 2->4 and a list of network blocks, looks through the list
// to find an entry != 0 indicating that IP is free and returns it.  Blocks are used in order and we skip
// 0 and 255 for octet4
func findFreeARecord(list *[256][256][256]int, networkBlocks []string) (string, error) {
	for _, block := range networkBlocks {
		ips := strings.Split(block, ",")
		_, startnet, err := net.ParseCIDR(strings.TrimSpace(ips[0]))
		if err != nil {
			return "", err
		}
		_, endnet, err := net.ParseCIDR(strings.TrimSpace(ips[1]))
		if err != nil {
			return "", err
		}

		octet1 := int(startnet.IP[0])
		octet2 := int(startnet.IP[1])
		octet3 := int(startnet.IP[2])
		octet4 := int(startnet.IP[3])

		// don't hand back a .0
		if octet4 == 0 {
			octet4 = 1
		}

		for ; octet2 <= 255; octet2++ {
			if octet2 > int(endnet.IP[1]) {
				break
			}

			for ; octet3 <= 255; octet3++ {
				if octet3 > int(endnet.IP[2]) {
					break
				}

				// don't hand out a .255 so only look up to .254
				for ; octet4 <= 254; octet4++ {
					if octet4 > int(endnet.IP[3]) {
						break
					}

					if list[octet2][octet3][octet4] == 0 {
						return fmt.Sprintf("%v.%v.%v.%v", octet1, octet2, octet3, octet4), nil
					}
				}
				octet4 = 1
			}
			octet3 = 0
		}
	}

	return "", errors.New("Could not find a free A record")
}
