package dns

import (
    "fmt"
    "net"
    "time"
    "errors"
    "strings"
    "strconv"
    "io/ioutil"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    gdns "google.golang.org/api/dns/v2beta1"
)

type GoogleDNS struct {
    Creds []byte
    Project string
    Zone string
    Options map[string]string
}

type RrsetCache struct {
    Rrsets []*gdns.ResourceRecordSet
    CacheTime time.Time
}

var cache RrsetCache

func NewGoogleDNS(creds string, project string, zone string, options map[string]string) *GoogleDNS {
    data, _ := ioutil.ReadFile(creds)
    return &GoogleDNS{
        Creds: data,
        Project: project,
        Zone: zone,
        Options: options,
    }
}

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

func (g *GoogleDNS) getZoneRecordSet(token string) error {
    if token == "" && cache.CacheTime != (time.Time{}) {
        now := time.Now()
        if now.Sub(cache.CacheTime).Seconds() <= 30 {
            return nil
        } else {
            cache = RrsetCache{}
        }
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

func (g *GoogleDNS) createARecord(name string, ip string) error {
    service, err := g.getDNSService()
    if err != nil {
        return err
    }

    // This is all internal so this should be safe, but check anyway, if it doesn't have a . assume we need to
    // append the zone name to our hostname
    if !strings.Contains(name, ".") {
        name = name + "." + g.Options["zone"] + "."
    }

    ttl, err := strconv.Atoi(g.Options["ttl"])
    if err != nil {
        return err
    }

    recordset := gdns.ResourceRecordSet{
        Kind: "dns#resourceRecordSet",
        Name: name,
        Rrdatas: []string{ip},
        Ttl: int64(ttl),
        Type: "A",
    }

    change := gdns.Change{
        Kind: "dns#change",
        Additions: []*gdns.ResourceRecordSet{&recordset},
    }

    cs := gdns.NewChangesService(service)
    ccc := cs.Create(g.Project, g.Zone, &change)
    _, err = ccc.Do()
    return err // will either be an error or nil, either way what we want to return at this point
}

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

    // This is all internal so this should be safe, but check anyway, if it doesn't have a . assume we need to
    // append the zone name to our hostname
    if !strings.Contains(name, ".") {
        name = name + "." + g.Options["zone"] + "."
    }

    var rrset *gdns.ResourceRecordSet
    for _, set := range cache.Rrsets {
        if set.Name == name {
            rrset = set
            break
        }
    }

    if rrset != nil {
        change := gdns.Change{
            Kind: "dns#change",
            Deletions: []*gdns.ResourceRecordSet{rrset},
        }

        cs := gdns.NewChangesService(service)
        ccc := cs.Create(g.Project, g.Zone, &change)
        _, err = ccc.Do()
        if err != nil {
            return err
        }

        // Pop the cache
        cache.CacheTime = time.Time{}
    }

    return nil
}

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
        return "", errors.New("Could not parse CIDR address ["+network+"] : " + err.Error())
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
    for net2 := 0; net2 <= 255 - int(net.Mask[1]); net2++ {
        o2 := int(net.IP[1]) + net2

        for net3 := 0; net3 <= 255 - int(net.Mask[2]); net3++ {
            o3 := int(net.IP[2]) + net3

            // skip .0 and .255 since they aren't usable
            for net4 := 1; net4 <= 254 - int(net.Mask[3]); net4++ {
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

func (g *GoogleDNS) RemoveARecord(name string) error {
    err := g.deleteARecord(name)
    return err
}

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
