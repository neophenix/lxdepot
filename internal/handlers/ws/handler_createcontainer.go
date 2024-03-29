package ws

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"
	"time"

	"github.com/neophenix/lxdepot/internal/circularbuffer"
	"github.com/neophenix/lxdepot/internal/dns"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// CreateContainerHandler creates the container on our host, then if we are using a 3rd
// party DNS gets an A record from there.
// It then uploads the appropriate network config file to the container before starting it by calling setupContainerNetwork
// Finally if any bootstrapping configuration is set, it to perform that by calling BootstrapContainer.
func CreateContainerHandler(buffer *circularbuffer.CircularBuffer[OutgoingMessage], msg IncomingMessage) {
	// Create the container
	// -------------------------
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Creating container", Success: true})
	}

	// options will be whatever the user wants set like container limits, priority, etc.  Its
	// called config in LXD land, but since we use config for our config I'm calling it options in here
	var options map[string]string
	err := json.Unmarshal([]byte(msg.Data["options"]), &options)
	if err != nil {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		}
		return
	}

	err = lxd.CreateContainer(msg.Data["host"], msg.Data["name"], msg.Data["image"], msg.Data["storagepool"], options)
	if err != nil {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		}
		return
	}
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "done", Success: true})
	}
	// -------------------------

	// DNS Previously we would fail here and continue, but that has been shown to lead to multiple containers being assigned
	// the same IP, which turns out is a bad idea.  So now we will fail, and let the user cleanup.
	// -------------------------
	if strings.ToLower(Conf.DNS.Provider) != "dhcp" {
		id := time.Now().UnixNano()
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "Creating DNS entry", Success: true})
		}

		d := dns.New(Conf)
		if d == nil {
			if buffer != nil {
				buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed to create DNS object for provider: " + Conf.DNS.Provider, Success: false})
			}
			return
		} else {
			ip, err := d.GetARecord(msg.Data["name"], Conf.DNS.NetworkBlocks)
			if err != nil {
				if buffer != nil {
					buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
				}
				return
			} else {
				if buffer != nil {
					buffer.Enqueue(OutgoingMessage{ID: id, Message: ip, Success: true})
				}

				// upload our network config
				setupContainerNetwork(buffer, msg.Data["host"], msg.Data["name"], ip)
			}
		}
	}
	// -------------------------

	// Start the container
	err = StartContainerHandler(buffer, msg)
	if err != nil {
		// The other handler would have taken care of the message
		return
	}

	id = time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Waiting for networking", Success: true})
	}

	// We will try 10 times to see if the networking comes up by asking LXD for the container state
	// and checking to see if we found an ipv4 address
	networkUp := false
	i := 0
	for !networkUp && i < 10 {
		// this isn't exactly as efficient as it could be but don't feel like making a new call just for this at the moment
		containerInfo, err := lxd.GetContainers(msg.Data["host"], msg.Data["name"], true)
		if err != nil {
			if buffer != nil {
				buffer.Enqueue(OutgoingMessage{ID: id, Message: err.Error(), Success: false})
			}
			return
		}
		// look through the container state for an address in the inet family, right not we aren't worried about comparing
		// this address to what we got from DNS if we are using that, maybe in the future if it becomes an issue
		for iface, info := range containerInfo[0].State.Network {
			if iface != "lo" {
				for _, addr := range info.Addresses {
					if addr.Family == "inet" && addr.Address != "" {
						networkUp = true
					}
				}
			}
		}
		i++
		time.Sleep(1 * time.Second)
	}
	if !networkUp {
		// we will bail if we didn't get an address since if we plan on bootstrapping we won't get far
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "no ip detected", Success: false})
		}
		return
	}

	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "network is up", Success: true})
	}

	BootstrapContainer(buffer, msg.Data["host"], msg.Data["name"])
}

// setupContainerNetwork looks at the OS of a container and then looks up any network template in our config.
// It then parses that template through text/template passing the IP and uploads it to the container
func setupContainerNetwork(buffer *circularbuffer.CircularBuffer[OutgoingMessage], host string, name string, ip string) {
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Configuring container networking", Success: true})
	}

	// Even though we just created it, lxd doesn't give us a lot of info back about the image, etc.
	// hell the GetImages call doesn't give us back a lot either.  So we are going to pull the container state
	// to be able to figure out what OS we are on, so we can then use the right network setup
	containerInfo, err := lxd.GetContainers(host, name, true)
	if err != nil {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		}
		return
	}

	// Given the OS reported by LXD, check to see if we have any networking config defined, and if so loop
	// over that array of templates and upload each one
	os := strings.ToLower(containerInfo[0].Container.ExpandedConfig["image.os"] + containerInfo[0].Container.ExpandedConfig["image.release"])
	if networking, ok := Conf.Networking[os]; ok {
		for _, file := range networking {
			var contents bytes.Buffer
			tmpl, err := template.New(file.RemotePath).Parse(file.Template)
			if err != nil {
				if buffer != nil {
					buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
				}
				return
			}
			tmpl.Execute(&contents, map[string]interface{}{
				"IP": ip,
			})

			err = lxd.CreateFile(host, name, file.RemotePath, 0644, contents.String())
			if err != nil {
				if buffer != nil {
					buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
				}
				return
			}
			if buffer != nil {
				buffer.Enqueue(OutgoingMessage{ID: id, Message: "done", Success: true})
			}
		}
	}
}
