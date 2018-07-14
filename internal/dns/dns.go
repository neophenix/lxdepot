// Package dns is for our 3rd party DNS integrations
package dns

import(
    "github.com/neophenix/lxdepot/internal/config"
)

// RecordList is a simple look at DNS records used as a common return for our interface
type RecordList struct {
    Name string         // the name of the entry
    RecordSet []string  // the values in the entry
}

// The Dns interface provides the list of functions all our 3rd party integrations should
// support.  I don't like that I coded the record type in the name, but until I decide
// I need IPv6, etc its good enough
type Dns interface {
    GetARecord(name string, network string) (string, error) // returns a string representation of an IPv4 address
    RemoveARecord(name string) error                        // removes the record from our 3rd party
    ListARecords() ([]RecordList, error)                    // returns a list of all the A records
}

// New should just hand back the appropriate interface for our config settings,
// returning from the correct "New" function for our integration
func New(conf *config.Config) Dns {
    if conf.DNS.Provider == "google" {
        return NewGoogleDNS(conf.DNS.Options["gcp_creds_file"], conf.DNS.Options["gcp_project_name"], conf.DNS.Options["gcp_zone_name"], conf.DNS.Options)
    }

    return nil
}
