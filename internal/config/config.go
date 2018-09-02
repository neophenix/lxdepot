// Package config provides all the structure and functions for parsing and dealing
// with the yaml config file
package config

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"strings"
)

// structs here are all in reverse order with our main config last

// LXDhost is where the details of each host we are going to talk to lives
type LXDhost struct {
	Host string `yaml:"host"` // The ip or fqdn we use to actually talk to the host
	Name string `yaml:"name"` // A human readable name / "alias" for the UI
	Port string `yaml:"port"` // The port that LXD is listening on
	Cert string `yaml:"cert"` // The server cert typically found in /var/lib/lxd/server.crt
}

// DNS settings, or are we using DHCP or a 3rd party provider
type DNS struct {
	DHCP          bool              `yaml:"dhcp"`           // Are we using DHCP on our network, if true we skip any provider settings
	Provider      string            `yaml:"provider"`       // Provider name, current supported: google
	NetworkBlocks []string          `yaml:"network_blocks"` // List of blocks that we can use for IPs, if not defined we can use any IP in the network
	TTL           int               `yaml:"ttl"`            // Default TTL of DNS entries
	Zone          string            `yaml:"zone"`           // DNS zone
	Options       map[string]string `yaml:"options"`        // Providers options documented at the top of a provider implementation
}

// FileOrCommand is for bootstrapping or other setup, used as an array of sequential "things to do"
// file will upload a file to the container, command will run a command on it
type FileOrCommand struct {
	Type           string    `yaml:"type"`             // file or command, what we are going to do
	Perms          int       `yaml:"perms"`            // for Type=file, the permissions of the file in the container
	LocalPath      string    `yaml:"local_path"`       // for Type=file, the local path to the file we want to upload
	RemotePath     string    `yaml:"remote_path"`      // for Type=file, where the file will live in the container
	Command        []string  `yaml:"command"`          // for Type=command, the command broken apart like ["yum", "-y", "install", "foo"]
	OkReturnValues []float64 `yaml:"ok_return_values"` // list of return values (other than 0) we accept as ok, 0 is always acceptable
}

// NetworkingConfig holds a network file template and the location where it should be placed in the container
type NetworkingConfig struct {
	RemotePath string `yaml:"remote_path"` // path of the file in the container
	Template   string `yaml:"template"`    // text/template parsable version of the file
}

// Config is the main config structure mostly pulling together the above items, also holds our client PKI
type Config struct {
	Cert       string                                `yaml:"cert"`       // client cert, which can either be the cert contents or file:/path/here that we will read in later
	Key        string                                `yaml:"key"`        // client key, same as cert, contents or file:/path/here
	LXDhosts   []*LXDhost                            `yaml:"lxdhosts"`   // array of all the hosts we will operate on
	DNS        DNS                                   `yaml:"dns"`        // DNS settings
	Networking map[string][]NetworkingConfig         `yaml:"networking"` // map of OS -> network template files
	Bootstrap  map[string][]FileOrCommand            `yaml:"bootstrap"`  // map to the OS type, and then an array of things to do
	Playbooks  map[string]map[string][]FileOrCommand `yaml:"playbooks"`  // map of OS -> playbook name -> list of things to do
}

// ParseConfig is the only function that external users need to know about.
// This will read the file from disk and, as its name implies, parse and unmarshal it using yaml
// We then call verifyConfig to make sure all the needed settings are there, any error we encounter
// a user should know about on startup so we log and die
func ParseConfig(configFile string) *Config {
	bytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("Could not read config [" + configFile + "] : " + err.Error() + "\n")
	}

	var config Config
	err = yaml.Unmarshal(bytes, &config)
	if err != nil {
		log.Fatal("Could not parse config [" + configFile + "] : " + err.Error() + "\n")
	}

	config.verifyConfig()

	return &config
}

// verifyConfig checks to make sure that the absolutely needed parts are here.
// If you are using bootstrapping or a third party DNS more checking should be
// added for those items
func (c *Config) verifyConfig() {
	if c.Cert == "" {
		log.Fatal("cert (client certificate) missing from config\n")
	}
	c.Cert = getValueOrFileContents(c.Cert)

	if c.Key == "" {
		log.Fatal("key (client key) missing from config\n")
	}
	c.Key = getValueOrFileContents(c.Key)

	if len(c.LXDhosts) == 0 {
		log.Fatal("no lxdhosts defined\n")
	}

	for idx, lxdh := range c.LXDhosts {
		if lxdh.Host == "" {
			log.Fatal("missing host param for lxdhost at index: " + string(idx) + "\n")
		}
		if lxdh.Cert == "" {
			log.Fatal("missing certificate for lxdhost: " + lxdh.Host + "\n")
		}
		lxdh.Cert = getValueOrFileContents(lxdh.Cert)
	}
}

// getValueOrFileContents is used by verifyConfig to check if the value of a param is file:/path
// or not.  If it is, we read the file from disk and return the contents, if it isn't we just return
// the value we were passed
func getValueOrFileContents(value string) string {
	if strings.HasPrefix(value, "file:") {
		data, err := ioutil.ReadFile(strings.TrimPrefix(value, "file:"))
		if err != nil {
			log.Fatal("Could not read file " + strings.TrimPrefix(value, "file:") + " : " + err.Error())
		}
		return string(data)
	}

	return value
}
