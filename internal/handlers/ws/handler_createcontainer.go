package ws

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/neophenix/lxdepot/internal/dns"
	"github.com/neophenix/lxdepot/internal/lxd"
	"github.com/sparrc/go-ping"
	"strings"
	"text/template"
	"time"
)

// CreateContainerHandler creates the container on our host, then if we are using a 3rd
// party DNS gets an A record from there.
// It then uploads the appropriate network config file to the container before starting it by calling setupContainerNetwork
// Finally if any bootstrapping configuration is set, it to perform that by calling BootstrapContainer.
func CreateContainerHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
	// Create the container
	// -------------------------
	id := time.Now().UnixNano()
	data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "Creating container", Success: true})
	conn.WriteMessage(mt, data)

	// options will be whatever the user wants set like container limits, priority, etc.  Its
	// called config in LXD land, but since we use config for our config I'm calling it options in here
	var options map[string]string
	err := json.Unmarshal([]byte(msg.Data["options"]), &options)
	if err != nil {
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return
	}

	err = lxd.CreateContainer(msg.Data["host"], msg.Data["name"], msg.Data["image"], msg.Data["storagepool"], options)
	if err != nil {
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return
	}
	data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "done", Success: true})
	conn.WriteMessage(mt, data)
	// -------------------------

	// DNS Previously we would fail here and continue, but that has been shown to lead to multiple containers being assigned
	// the same IP, which turns out is a bad idea.  So now we will fail, and let the user cleanup.
	// -------------------------
	var ip string
	if strings.ToLower(Conf.DNS.Provider) != "dhcp" {
		id := time.Now().UnixNano()
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "Creating DNS entry", Success: true})
		conn.WriteMessage(mt, data)

		d := dns.New(Conf)
		if d == nil {
			data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed to create DNS object for provider: " + Conf.DNS.Provider, Success: false})
			conn.WriteMessage(mt, data)
			return
		} else {
			ip, err = d.GetARecord(msg.Data["name"], Conf.DNS.NetworkBlocks)
			if err != nil {
				data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
				conn.WriteMessage(mt, data)
				return
			} else {
				data, _ = json.Marshal(OutgoingMessage{ID: id, Message: ip, Success: true})
				conn.WriteMessage(mt, data)

				// upload our network config
				setupContainerNetwork(conn, mt, msg.Data["host"], msg.Data["name"], ip)
			}
		}
	}
	// -------------------------

	// Start the container
	err = StartContainerHandler(conn, mt, msg)
	if err != nil {
		// The other handler would have taken care of the message
		return
	}

	id = time.Now().UnixNano()
	// This is to ensure the networking is up before we continue on.
	// Otherwise because we can't really check the command status, we will think things
	// like yum installed what we wanted, when really it bailed due to a network issue.
	if strings.ToLower(Conf.DNS.Provider) != "dhcp" {
		// if we aren't using DHCP we should know the IP we are trying to use so ping it
		pinger, err := ping.NewPinger(ip)
		if err != nil {
			// if we somehow got an error just sleep for 5 seconds like the DHCP case
			time.Sleep(5 * time.Second)
		} else {
			data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "Pinging container", Success: true})
			conn.WriteMessage(mt, data)

			// slightly longer timeout as if this doens't work we are going to bail
			pinger.Timeout = 10 * time.Second
			hostAlive := false
			pinger.OnRecv = func(pkt *ping.Packet) {
				hostAlive = true
				pinger.Stop()
			}
			pinger.Run()

			if !hostAlive {
				// error and return, the next step would be bootstrapping which likely involves network access
				// and since we more or less determined that isn't working, no sense continuing
				data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "failed to ping host", Success: false})
				conn.WriteMessage(mt, data)
				return
			}
		}
	} else {
		data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "Sleeping while networking starts", Success: true})
		conn.WriteMessage(mt, data)
		// we won't know the IP to ping for DHCP so just sleep
		time.Sleep(5 * time.Second)
	}
	data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "done", Success: true})
	conn.WriteMessage(mt, data)

	err = BootstrapContainer(conn, mt, msg.Data["host"], msg.Data["name"])
	if err != nil {
		return
	}
}

// setupContainerNetwork looks at the OS of a container and then looks up any network template in our config.
// It then parses that template through text/template passing the IP and uploads it to the container
func setupContainerNetwork(conn *websocket.Conn, mt int, host string, name string, ip string) {
	id := time.Now().UnixNano()
	data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "Configuring container networking", Success: true})
	conn.WriteMessage(mt, data)

	// Even though we just created it, lxd doesn't give us a lot of info back about the image, etc.
	// hell the GetImages call doesn't give us back a lot either.  So we are going to pull the container state
	// to be able to figure out what OS we are on, so we can then use the right network setup
	containerInfo, err := lxd.GetContainers(host, name, true)
	if err != nil {
		data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return
	}

	// Given the OS reported by LXD, check to see if we have any networking config defined, and if so loop
	// over that array of templates and upload each one
	os := containerInfo[0].Container.ExpandedConfig["image.os"] + containerInfo[0].Container.ExpandedConfig["image.release"]
	// we won't do this if we are using DHCP as the default templates will work for that.  This needs more thought
	// put into it honestly as there could be a reason to have a template for DHCP
	if strings.ToLower(Conf.DNS.Provider) != "dhcp" {
		if networking, ok := Conf.Networking[os]; ok {
			for _, file := range networking {
				var contents bytes.Buffer
				tmpl, err := template.New(file.RemotePath).Parse(file.Template)
				if err != nil {
					data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
					conn.WriteMessage(mt, data)
					return
				}
				tmpl.Execute(&contents, map[string]interface{}{
					"IP": ip,
				})

				err = lxd.CreateFile(host, name, file.RemotePath, 0644, contents.String())
				if err != nil {
					data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
					conn.WriteMessage(mt, data)
					return
				}
				data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "done", Success: true})
				conn.WriteMessage(mt, data)
			}
		}
	}
}
