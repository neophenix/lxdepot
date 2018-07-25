package dns

import (
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gdns "google.golang.org/api/dns/v2beta1"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"
)

// GoogleDNS stores all the options we need to talk to GCP
type GoogleDNS struct {
	Creds   []byte // the contents of our service account json credentials file
	Project string // the GCP project name we are operating on
	Zone    string // the GCP DNS zone we are using

	// various other options we need:
	//      ttl: the TTL to use for our entries
	//      zone: the zone portion of our fqdn (dev.example.com), will be appended to hostnames
	//      network: CIDR of the network we are using to find a free address (10.0.0.0/24)
	Options map[string]string
}

// RrsetCache is the cache of all the recordsets, figure if the system is in
// regular use its better to store these for a few minutes than make a call each time
type RrsetCache struct {
	Rrsets    []*gdns.ResourceRecordSet
	CacheTime time.Time
}

var cache RrsetCache

// NewGoogleDNS will return our GCP DNS interface
// The creds, project, and zone here are actually in the options as well, but they are important
// enough to warrant being "top level" items
func NewGoogleDNS(creds string, project string, zone string, options map[string]string) *GoogleDNS {
	data, _ := ioutil.ReadFile(creds)
	return &GoogleDNS{
		Creds:   data,
		Project: project,
		Zone:    zone,
		Options: options,
	}
}

// getDNSService takes the credentials and should return a GCP DNS Service, provided the creds are still good
func (g *GoogleDNS) getDNSService() (*gdns.Service, error) {
	conf, err := google.JWTConfigFromJSON(g.Creds, "https://www.googleapis.com/auth/ndev.clouddns.readwrite")
	if err != nil {
		return nil, errors.New("Could not create Google JWT config: " + err.Error())
	}

	client := conf.Client(oauth2.NoContext)

	s, err := gdns.New(client)
	if err != nil {
		return nil, errors.New("Could not make Google DNS service: " + err.Error())
	}

	return s, nil
}

// getZoneRecordSet either returns our cache of records or fetches new ones.
// This is recursive if we run into pagination
func (g *GoogleDNS) getZoneRecordSet(token string) error {
	if token == "" && cache.CacheTime != (time.Time{}) {
		now := time.Now()
		if now.Sub(cache.CacheTime).Seconds() <= 30 {
			return nil
		}
		cache = RrsetCache{}
	}

	service, err := g.getDNSService()
	if err != nil {
		return err
	}

	rrs := gdns.NewResourceRecordSetsService(service)
	rrsl := rrs.List(g.Project, g.Zone)
	if token != "" {
		rrsl = rrsl.PageToken(token)
	}
	resp, err := rrsl.Do()
	if err != nil {
		return errors.New("Error fetching record set: " + err.Error())
	}

	cache.Rrsets = append(cache.Rrsets, resp.Rrsets...)
	cache.CacheTime = time.Now()
	if resp.NextPageToken != "" {
		return g.getZoneRecordSet(resp.NextPageToken)
	}

	return nil
}

// createARecord creates the entry in GCP
func (g *GoogleDNS) createARecord(name string, ip string) error {
	service, err := g.getDNSService()
	if err != nil {
		return err
	}

	// This is all internal so this should be safe, but check anyway, if it doesn't have a . assume we need to
	// append the zone name to our hostname, name needs to end in . for GCP to accept it
	if !strings.Contains(name, ".") {
		name = name + "." + g.Options["zone"] + "."
	}

	// Since our options is a string map, convert out TTL to an int64 for actual usage.
	ttl, err := strconv.ParseInt(g.Options["ttl"], 10, 64)
	if err != nil {
		return err
	}

	recordset := gdns.ResourceRecordSet{
		Kind:    "dns#resourceRecordSet",
		Name:    name,
		Rrdatas: []string{ip},
		Ttl:     ttl,
		Type:    "A",
	}

	// Standard GCP API usage is make the change opject, ask for a change service based on our overall service
	// then pass the change to the change service to perform the operation
	change := gdns.Change{
		Kind:      "dns#change",
		Additions: []*gdns.ResourceRecordSet{&recordset},
	}

	cs := gdns.NewChangesService(service)
	ccc := cs.Create(g.Project, g.Zone, &change)
	_, err = ccc.Do()
	return err // will either be an error or nil, either way what we want to return at this point
}

// deleteARecord removes the host from DNS.  At the moment it removes all the records for the host, so the
// name is a little bit misleading.  It does this by pulling the records sets into cache, and just matching
// the correct record set by name, and passing that back as a deletion.
func (g *GoogleDNS) deleteARecord(name string) error {
	service, err := g.getDNSService()
	if err != nil {
		return err
	}

	// Make sure our cache is up to date
	err = g.getZoneRecordSet("")
	if err != nil {
		return err
	}

	// Like in createARecord, if we don't have a . in the name assume we need to append everything.  I think
	// ideally we should reject hostnames with a . in them and just force us to the the arbiter of a good name
	if !strings.Contains(name, ".") {
		name = name + "." + g.Options["zone"] + "."
	}

	// Loop over our cache and grab the recordset by name, we will pass this to our delete request
	var rrset *gdns.ResourceRecordSet
	for _, set := range cache.Rrsets {
		if set.Name == name {
			rrset = set
			break
		}
	}

	// if we found a record set, remove it
	if rrset != nil {
		change := gdns.Change{
			Kind:      "dns#change",
			Deletions: []*gdns.ResourceRecordSet{rrset},
		}

		cs := gdns.NewChangesService(service)
		ccc := cs.Create(g.Project, g.Zone, &change)
		_, err = ccc.Do()
		if err != nil {
			return err
		}

		// Pop the cache instead of trying to be clever
		cache.CacheTime = time.Time{}
	}

	return nil
}

// GetARecord returns an A record for our host.  If the host already has one,
// this will return the first record encountered, it does not currently ensure that
// record is in the network we are asking for.  If there is no existing record, it will
// loop over a 3 dimensional array looking for a free entry to use.
func (g *GoogleDNS) GetARecord(name string, network string) (string, error) {
	// Make sure our cache is up to date
	err := g.getZoneRecordSet("")
	if err != nil {
		return "", err
	}

	// Make sure we are looking for the fqdn
	if !strings.Contains(name, ".") {
		name = name + "." + g.Options["zone"] + "."
	}

	_, net, err := net.ParseCIDR(network)
	if err != nil {
		return "", errors.New("Could not parse CIDR address [" + network + "] : " + err.Error())
	}

	// This is going to "mark off" all the records we have, so then we can loop over it and find a free spot
	var list [256][256][256]int
	for _, set := range cache.Rrsets {
		if set.Type == "A" {
			for _, ip := range set.Rrdatas {
				octets := strings.Split(ip, ".")
				o2, _ := strconv.Atoi(octets[1])
				o3, _ := strconv.Atoi(octets[2])
				o4, _ := strconv.Atoi(octets[3])
				list[o2][o3][o4] = 1
			}

			// We already have our host in DNS
			if set.Name == name {
				return set.Rrdatas[0], nil
			}
		}
	}

	// Given the network we want to operate on (that we parse above) take each octet of it and then
	// loop over our marked off list, when we find an entry that = 0 (we didn't mark it) its free, return
	o1 := int(net.IP[0])
	for net2 := 0; net2 <= 255-int(net.Mask[1]); net2++ {
		o2 := int(net.IP[1]) + net2

		for net3 := 0; net3 <= 255-int(net.Mask[2]); net3++ {
			o3 := int(net.IP[2]) + net3

			// skip .0 and .255 since they aren't usable
			for net4 := 1; net4 <= 254-int(net.Mask[3]); net4++ {
				o4 := int(net.IP[3]) + net4

				if list[o2][o3][o4] == 0 {
					err = g.createARecord(name, fmt.Sprintf("%v.%v.%v.%v", o1, o2, o3, o4))
					// just return the IP we found and err which will be an error or nil, as one should check that first
					return fmt.Sprintf("%v.%v.%v.%v", o1, o2, o3, o4), err
				}
			}
		}
	}

	return "", errors.New("Could not find a free A record")
}

// RemoveARecord passes our name to deleteARecord as it doesn't have to do any additional processing
func (g *GoogleDNS) RemoveARecord(name string) error {
	err := g.deleteARecord(name)
	return err
}

// ListARecords repopulates the internal cache and then appends any A records it finds to a
/// RecordList array and returns that
func (g *GoogleDNS) ListARecords() ([]RecordList, error) {
	var list []RecordList

	// Make sure our cache is up to date
	err := g.getZoneRecordSet("")
	if err != nil {
		return list, err
	}

	for _, set := range cache.Rrsets {
		if set.Type == "A" {
			list = append(list, RecordList{Name: set.Name, RecordSet: set.Rrdatas})
		}
	}

	return list, nil
}
