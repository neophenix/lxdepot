package dns

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

// AmazonDNS stores all the options we need to talk to Route 53
type AmazonDNS struct {
	CredsFile string // the shared credentials filename (full path)
	Profile   string // profile within the creds file to use, "" for default
	ZoneID    string // the Route 53 DNS zone we are using
}

// amazonRrsetCache is the cache of all the recordsets, figure if the system is in
// regular use its better to store these for a few minutes than make a call each time
type amazonRrsetCache struct {
	Rrsets    []*route53.ResourceRecordSet
	CacheTime time.Time
}

var acache amazonRrsetCache

// NewAmazonDNS will return our Amazon Route 53 DNS interface
func NewAmazonDNS(credsfile string, profile string, zoneid string) *AmazonDNS {
	return &AmazonDNS{
		CredsFile: credsfile,
		Profile:   profile,
		ZoneID:    zoneid,
	}
}

// getDNSService takes the credentials and should return a Route 53 DNS service
func (a *AmazonDNS) getDNSService() (*route53.Route53, error) {
	session, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewSharedCredentials(a.CredsFile, a.Profile),
	})
	if err != nil {
		return nil, errors.New("Could not create new Route 53 session: " + err.Error())
	}

	service := route53.New(session)

	return service, nil
}

// getZoneRecordSet either returns our cache of records or fetches new ones.
func (a *AmazonDNS) getZoneRecordSet() error {
	if acache.CacheTime != (time.Time{}) {
		now := time.Now()
		if now.Sub(acache.CacheTime).Seconds() <= 30 {
			return nil
		}
		acache = amazonRrsetCache{}
	}

	service, err := a.getDNSService()
	if err != nil {
		return err
	}

	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(a.ZoneID),
	}

	err = service.ListResourceRecordSetsPages(params,
		func(page *route53.ListResourceRecordSetsOutput, lastPage bool) bool {
			acache.Rrsets = append(acache.Rrsets, page.ResourceRecordSets...)
			return lastPage
		})
	if err != nil {
		return err
	}

	acache.CacheTime = time.Now()
	return nil
}

// createARecord creates the entry in Route 53
func (a *AmazonDNS) createARecord(name string, ip string) error {
	service, err := a.getDNSService()
	if err != nil {
		return err
	}

	// This is all internal so this should be safe, but check anyway, if it doesn't have a . assume we need to
	// append the zone name to our hostname, name needs to end in . for AWS to accept it
	if !strings.Contains(name, ".") {
		name = name + "." + DNSOptions.Zone + "."
	}

	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(name),
						Type: aws.String("A"),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ip),
							},
						},
						TTL:           aws.Int64(int64(DNSOptions.TTL)),
						Weight:        aws.Int64(1),
						SetIdentifier: aws.String("lxdepot"),
					},
				},
			},
			Comment: aws.String("Adding A record for " + name),
		},
		HostedZoneId: aws.String(a.ZoneID),
	}
	_, err = service.ChangeResourceRecordSets(params)

	return err // will either be an error or nil, either way what we want to return at this point
}

// deleteARecord removes the host from DNS.  At the moment it removes all the records for the host, so the
// name is a little bit misleading.  It does this by pulling the records sets into cache, and just matching
// the correct record set by name, and passing that back as a deletion.
func (a *AmazonDNS) deleteARecord(name string) error {
	service, err := a.getDNSService()
	if err != nil {
		return err
	}

	// Make sure our cache is up to date
	err = a.getZoneRecordSet()
	if err != nil {
		return err
	}

	// Like in createARecord, if we don't have a . in the name assume we need to append everything.  I think
	// ideally we should reject hostnames with a . in them and just force us to the the arbiter of a good name
	if !strings.Contains(name, ".") {
		name = name + "." + DNSOptions.Zone + "."
	}

	// Loop over our cache and grab the recordset by name, we will pass this to our delete request
	var rrset *route53.ResourceRecordSet
	for _, set := range acache.Rrsets {
		if *set.Type == "A" && *set.Name == name {
			rrset = set
			break
		}
	}

	// if we found a record set, remove it
	if rrset != nil {
		params := &route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{
					{
						Action:            aws.String("DELETE"),
						ResourceRecordSet: rrset,
					},
				},
				Comment: aws.String("Deleting A record for " + name),
			},
			HostedZoneId: aws.String(a.ZoneID),
		}
		_, err := service.ChangeResourceRecordSets(params)
		if err != nil {
			return err
		}

		// Pop the cache instead of trying to be clever
		acache.CacheTime = time.Time{}
	}

	return nil
}

// GetARecord returns an A record for our host.  If the host already has one,
// this will return the first record encountered, it does not currently ensure that
// record is in the network we are asking for.  If there is no existing record, it will
// loop over a 3 dimensional array looking for a free entry to use.
func (a *AmazonDNS) GetARecord(name string, networkBlocks []string) (string, error) {
	// Make sure our cache is up to date
	err := a.getZoneRecordSet()
	if err != nil {
		return "", err
	}

	// Make sure we are looking for the fqdn
	if !strings.Contains(name, ".") {
		name = name + "." + DNSOptions.Zone + "."
	}

	// This is going to "mark off" all the records we have, so then we can loop over it and find a free spot
	var list [256][256][256]int
	for _, set := range acache.Rrsets {
		if *set.Type == "A" {
			// We already have our host in DNS
			if *set.Name == name {
				return *set.ResourceRecords[0].Value, nil
			}

			for _, rr := range set.ResourceRecords {
				octets := strings.Split(*rr.Value, ".")
				o2, _ := strconv.Atoi(octets[1])
				o3, _ := strconv.Atoi(octets[2])
				o4, _ := strconv.Atoi(octets[3])
				list[o2][o3][o4] = 1
			}
		}
	}

	ip, err := findFreeARecord(&list, networkBlocks)
	if err != nil {
		return "", err
	}

	err = a.createARecord(name, ip)
	// just return the IP we found and err which will be an error or nil, as one should check that first
	return ip, err
}

// RemoveARecord passes our name to deleteARecord as it doesn't have to do any additional processing
func (a *AmazonDNS) RemoveARecord(name string) error {
	err := a.deleteARecord(name)
	return err
}

// ListARecords repopulates the internal cache and then appends any A records it finds to a
/// RecordList array and returns that
func (a *AmazonDNS) ListARecords() ([]RecordList, error) {
	var list []RecordList

	// Make sure our cache is up to date
	err := a.getZoneRecordSet()
	if err != nil {
		return list, err
	}

	for _, set := range acache.Rrsets {
		if *set.Type == "A" {
			records := make([]string, len(set.ResourceRecords))
			for idx, rr := range set.ResourceRecords {
				records[idx] = *rr.Value
			}
			list = append(list, RecordList{Name: *set.Name, RecordSet: records})
		}
	}

	return list, nil
}
