package ws

import(
    "time"
    "bytes"
    "strings"
    "io/ioutil"
    "text/template"
    "encoding/json"
    "github.com/gorilla/websocket"
    "github.com/neophenix/lxdepot/internal/lxd"
    "github.com/neophenix/lxdepot/internal/dns"
    "github.com/neophenix/lxdepot/internal/config"
)

// ContainerBootstrapHandler calls bootstrapContainer to bootstrap an already
// running container. Unused at the moment on the UI
func ContainerBootstrapHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
    err := bootstrapContainer(conn, mt, msg.Data["host"], msg.Data["name"])
    if err != nil {
        return
    }
}

// CreateContainerHandler creates the container on our host, then if we are using a 3rd
// party DNS gets an A record from there.
// It then uploads the appropriate network config file to the container before starting it by calling setupContainerNetwork
// Finally if any bootstrapping configuration is set, it to perform that by calling bootstrapContainer.
func CreateContainerHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
    // Create the container
    // -------------------------
    id := time.Now().UnixNano()
    data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Creating container", Success: true})
    conn.WriteMessage(mt, data)

    err := lxd.CreateContainer(msg.Data["host"], msg.Data["name"], msg.Data["image"])
    if err != nil {
        data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
        conn.WriteMessage(mt, data)
        return
    }
    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
    conn.WriteMessage(mt, data)
    // -------------------------

    // DNS? If this fails I don't think it is enough reason to bail, will see
    // -------------------------
    if ! Conf.DNS.DHCP {
        id := time.Now().UnixNano()
        data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Creating DNS entry", Success: true})
        conn.WriteMessage(mt, data)

        d := dns.New(Conf)
        if d == nil {
            data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "failed to create DNS object for provider: " + Conf.DNS.Provider, Success: false})
            conn.WriteMessage(mt, data)
        } else {
            ip, err := d.GetARecord(msg.Data["name"], Conf.DNS.Options["network"])
            if err != nil {
                data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
                conn.WriteMessage(mt, data)
            } else {
                data, _ = json.Marshal(OutgoingMessage{Id: id, Message: ip, Success: true})
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
    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "Waiting for networking", Success: true})
    conn.WriteMessage(mt, data)
    // just going to sleep for now, maybe ping later?  This is to ensure the networking is up before
    // we continue on.  Otherwise because we can't really check the command status, we will think things
    // like yum installed what we wanted, when really it bailed due to a network issue.
    time.Sleep(5 * time.Second)
    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
    conn.WriteMessage(mt, data)

    err = bootstrapContainer(conn, mt, msg.Data["host"], msg.Data["name"])
    if err != nil {
        return
    }
}

// setupContainerNetwork looks at the OS of a container and then looks up any network template in our config.
// It then parses that template through text/template passing the IP and uploads it to the container
func setupContainerNetwork(conn *websocket.Conn, mt int, host string, name string, ip string) {
    id := time.Now().UnixNano()
    data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Configuring container networking", Success: true})
    conn.WriteMessage(mt, data)

    // Even though we just created it, lxd doesn't give us a lot of info back about the image, etc.
    // hell the GetImages call doesn't give us back a lot either.  So we are going to pull the container state
    // to be able to figure out what OS we are on, so we can then use the right network setup
    containerInfo, err := lxd.GetContainers(host, name, true)
    if err != nil {
        data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
        conn.WriteMessage(mt, data)
        return
    }

    // Given the OS reported by LXD, check to see if we have any networking config defined, and if so loop
    // over that array of templates and upload each one
    os := containerInfo[0].Container.ExpandedConfig["image.os"]
    if networking, ok := Conf.Networking[os]; ok {
        for _, file := range networking {
            var contents bytes.Buffer
            tmpl, err := template.New(file.RemotePath).Parse(file.Template)
            if err != nil {
                data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
                conn.WriteMessage(mt, data)
                return
            }
            tmpl.Execute(&contents, map[string]interface{}{
                "IP": ip,
            })

            err = lxd.CreateFile(host, name, file.RemotePath, 0644, contents.String())
            if err != nil {
                data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
                conn.WriteMessage(mt, data)
                return
            }
            data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
            conn.WriteMessage(mt, data)
        }
    }
}

// bootstrapContainer loops over all the FileOrCommand objects in the bootstrap section of the config
// and performs each item sequentially
func bootstrapContainer(conn *websocket.Conn, mt int, host string, name string) error {
    id := time.Now().UnixNano()
    data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Getting container state", Success: true})
    conn.WriteMessage(mt, data)

    // Get the container state again, should probably just grab this once but for now lets be expensive
    containerInfo, err := lxd.GetContainers(host, name, true)
    if err != nil {
        data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
        conn.WriteMessage(mt, data)
        return err
    }
    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
    conn.WriteMessage(mt, data)

    // if we have a bootstrap section for this OS, run it
    os := containerInfo[0].Container.ExpandedConfig["image.os"]
    if bootstrap, ok := Conf.Bootstrap[os]; ok {
        for _, step := range bootstrap {
            // depending on the type, call the appropriate helper
            if step.Type == "file" {
                err = bootstrapCreateFile(conn, mt, host, name, step)
                if err != nil {
                    return err
                }
            } else if step.Type == "command" {
                err = bootstrapExecCommand(conn, mt, host, name, step)
                if err != nil {
                    return err
                }
            }
        }
    }

    return nil
}

// bootstrapCreateFile operates on a Type = file bootstrap step.
// If there is a local_path, it reads the contents of that file from disk.
// The contents are then sent to the lxd.CreateFile with the path on the container and permissions to "do the right thing"
func bootstrapCreateFile(conn *websocket.Conn, mt int, host string, name string, info config.FileOrCommand) error {
    id := time.Now().UnixNano()
    data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Creating " + info.RemotePath, Success: true})
    conn.WriteMessage(mt, data)

    var contents []byte
    var err error
    if info.LocalPath != "" {
        contents, err = ioutil.ReadFile(info.LocalPath)
        if err != nil {
            data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
            conn.WriteMessage(mt, data)
            return err
        }
    }

    err = lxd.CreateFile(host, name, info.RemotePath, info.Perms, string(contents))
    if err != nil {
        data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
        conn.WriteMessage(mt, data)
        return err
    }

    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
    conn.WriteMessage(mt, data)
    return nil
}

// bootstrapExecCommand operates on a Type = command bootstrap step.
// This is really just a wrapper around lxd.ExecCommand
func bootstrapExecCommand(conn *websocket.Conn, mt int, host string, name string, info config.FileOrCommand) error {
    id := time.Now().UnixNano()
    data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Executing " + strings.Join(info.Command, " "), Success: true})
    conn.WriteMessage(mt, data)

    err := lxd.ExecCommand(host, name, info.Command)
    if err != nil {
        data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
        conn.WriteMessage(mt, data)
        return err
    }

    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
    conn.WriteMessage(mt, data)
    return nil
}
