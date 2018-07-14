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

func ContainerBootstrapHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
    err := bootstrapContainer(conn, mt, msg.Data["host"], msg.Data["name"])
    if err != nil {
        return
    }
}

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
    // just going to sleep for now, maybe ping later?
    time.Sleep(5 * time.Second)
    data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
    conn.WriteMessage(mt, data)

    err = bootstrapContainer(conn, mt, msg.Data["host"], msg.Data["name"])
    if err != nil {
        return
    }
}

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

    if containerInfo[0].Container.ExpandedConfig["image.os"] == "Centos" {
        var contents bytes.Buffer
        tmpl, err := template.New("centos-ifcfg-eth0").Parse(Conf.Networking["centos"]["ifcfg-eth0"])
        if err != nil {
            data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
            conn.WriteMessage(mt, data)
            return
        }
        tmpl.Execute(&contents, map[string]interface{}{
            "IP": ip,
        })

        err = lxd.CreateFile(host, name, "/etc/sysconfig/network-scripts/ifcfg-eth0", 0644, contents.String())
        if err != nil {
            data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
            conn.WriteMessage(mt, data)
            return
        }
        data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
        conn.WriteMessage(mt, data)
    }
}

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

    if containerInfo[0].Container.ExpandedConfig["image.os"] == "Centos" {
        for _, step := range Conf.Bootstrap["centos"] {
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
