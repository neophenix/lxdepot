// Package ws is for our websocket handlers
// All the websocket handlers send 2 messages to the UI.
// The first is what we are attempting, running a command, etc.  The next is the status or output of that item
package ws

import(
    "log"
    "time"
    "strings"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "github.com/gorilla/websocket"
    "github.com/neophenix/lxdepot/internal/config"
    "github.com/neophenix/lxdepot/internal/lxd"
)

// IncomingMessage is for messages from the client to us
type IncomingMessage struct {
    Action string           // what type of request: create, start, etc.
    Data map[string]string  // in the UI this is a single level JSON object so requests can have varying options
}

// OutgoingMessage is from us to the UI
type OutgoingMessage struct {
    Id int64        // ID to keep messages and their status together
    Message string  // message to show the user
    Success bool    // success is used to give a visual hint to the user how the command went (true = green, false = red)
    Redirect string // If we want to suggest a redirect to another page, like back to /containers after we create a new one
}

// I need to see if I still need this, I think it was for when I was testing websockets using static assets served
// elsewhere, I think it can be removed
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

// our config from the main function
var Conf *config.Config

// Handler is our overall websocket router, it unmarshals the request and then sends it to
// the appropriate handler
func Handler(w http.ResponseWriter, r *http.Request) {
    // upgrade to a websocket
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Print("upgrade:", err)
        return
    }
    defer conn.Close()
    for {
        // read out message and unmarshal it, log out what it was for debugging.
        mt, enc_msg, err := conn.ReadMessage()
        if err != nil {
            log.Println("read:", err)
            break
        }
        log.Printf("ws recv: %s\n", enc_msg)
        var msg IncomingMessage
        err = json.Unmarshal(enc_msg, &msg)
        if err != nil {
            log.Println("unmarshal:", err)
            break
        }

        // Action tells us what we want to do, so this is a pretty simple router for the various requests
        // Each handler should be in its own handler_* file in the ws package
        switch msg.Action {
            case "start":
                StartContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
                conn.WriteMessage(mt, data)
            case "stop":
                StopContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
                conn.WriteMessage(mt, data)
            case "create":
                CreateContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
                conn.WriteMessage(mt, data)
            case "delete":
                DeleteContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/containers"})
                conn.WriteMessage(mt, data)
            case "playbook":
                ContainerPlaybookHandler(conn, mt, msg)
            default:
                id := time.Now().UnixNano()
                data, _ := json.Marshal(OutgoingMessage{Id: id, Message:"Request not understood", Success: false})
                conn.WriteMessage(mt, data)
        }
    }
}

// ContainerCreateFile operates on a Type = file bootstrap / playbook step.
// If there is a local_path, it reads the contents of that file from disk.
// The contents are then sent to the lxd.CreateFile with the path on the container and permissions to "do the right thing"
func ContainerCreateFile(conn *websocket.Conn, mt int, host string, name string, info config.FileOrCommand) error {
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

// ContainerExecCommand operates on a Type = command bootstrap / playbook step.
// This is really just a wrapper around lxd.ExecCommand
func ContainerExecCommand(conn *websocket.Conn, mt int, host string, name string, info config.FileOrCommand) error {
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
