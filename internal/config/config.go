package config

import(
    "log"
    "strings"
    "io/ioutil"
    "gopkg.in/yaml.v2"
)

type LXDhost struct {
    Host string `yaml:"host"`
    Name string `yaml:"name"`
    Port string `yaml:"port"`
    Cert string `yaml:"cert"`
}

type DNS struct {
    DHCP bool `yaml:"dhcp"`
    Provider string `yaml:"provider"`
    Options map[string]string `yaml:"options"`
}

type FileOrCommand struct {
    Type string `yaml:"type"`
    Perms int `yaml:"perms"`
    LocalPath string `yaml:"local_path"`
    RemotePath string `yaml:"remote_path"`
    Command []string `yaml:"command"`
}

type Config struct {
    Cert string `yaml:"cert"`
    Key string `yaml:"key"`
    LXDhosts []*LXDhost `yaml:"lxdhosts"`
    DNS DNS `yaml:"dns"`
    Networking map[string]map[string]string `yaml:"networking"`
    Bootstrap map[string][]FileOrCommand `yaml:"bootstrap"`
}

func ParseConfig(configFile string) *Config {
    bytes, err := ioutil.ReadFile(configFile)
    if err != nil {
        log.Fatal("Could not read config ["+configFile+"] : " + err.Error() + "\n")
    }

    var config Config
    err = yaml.Unmarshal(bytes, &config)
    if err != nil {
        log.Fatal("Could not parse config ["+configFile+"] : " + err.Error() + "\n")
    }

    config.verifyConfig()

    return &config
}

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
            log.Fatal("missing host param for lxdhost at index: "+string(idx)+"\n")
        }
        if lxdh.Cert == "" {
            log.Fatal("missing certificate for lxdhost: "+lxdh.Host+"\n")
        }
        lxdh.Cert = getValueOrFileContents(lxdh.Cert)
    }
}

func getValueOrFileContents(value string) string {
    // Certain values can be in the format file: <path>, if so read in those files and put their
    // contents into the value, replacing the file location

    if strings.HasPrefix(value, "file:") {
        data, err := ioutil.ReadFile(strings.TrimPrefix(value, "file:"))
        if err != nil {
            log.Fatal("Could not read file " + strings.TrimPrefix(value, "file:") + " : " + err.Error())
        }
        return string(data)
    }

    return value
}
