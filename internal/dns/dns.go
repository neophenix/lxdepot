package dns

import(
    "github.com/neophenix/lxdepot/internal/config"
)

type RecordList struct {
    Name string
    RecordSet []string
}

type Dns interface {
    GetARecord(name string, network string) (string, error)
    RemoveARecord(name string) error
    ListARecords() ([]RecordList, error)
}

func New(conf *config.Config) Dns {
    if conf.DNS.Provider == "google" {
        return NewGoogleDNS(conf.DNS.Options["gcp_creds_file"], conf.DNS.Options["gcp_project_name"], conf.DNS.Options["gcp_zone_name"], conf.DNS.Options)
    }

    return nil
}
