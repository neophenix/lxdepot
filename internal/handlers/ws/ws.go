// Package ws is for our websocket handlers
// All the websocket handlers send 2 messages to the UI.
// The first is what we are attempting, running a command, etc.  The next is the status or output of that item
package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/neophenix/lxdepot/internal/circularbuffer"
	"github.com/neophenix/lxdepot/internal/config"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// IncomingMessage is for messages from the client to us
type IncomingMessage struct {
	Action    string            `json:"action"` // what type of request: create, start, etc.
	BrowserID string            `json:"id"`     // ID of our users browser
	Data      map[string]string `json:"data"`   // in the UI this is a single level JSON object so requests can have varying options
}

// OutgoingMessage is from us to the UI
type OutgoingMessage struct {
	ID       int64  // ID to keep messages and their status together
	Message  string // message to show the user
	Success  bool   // success is used to give a visual hint to the user how the command went (true = green, false = red)
	Redirect string // If we want to suggest a redirect to another page, like back to /containers after we create a new one
}

// MessageBuffer will house our outgoing messages so clients can navigate around and get updates
var MessageBuffer = map[string]*circularbuffer.CircularBuffer[OutgoingMessage]{}

// I need to see if I still need this, I think it was for when I was testing websockets using static assets served
// elsewhere, I think it can be removed
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Conf is our main config
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
		mt, encmsg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("ws recv: %s\n", encmsg)
		var msg IncomingMessage
		err = json.Unmarshal(encmsg, &msg)
		if err != nil {
			log.Println("unmarshal:", err)
			break
		}

		var buffer *circularbuffer.CircularBuffer[OutgoingMessage]
		if msg.BrowserID != "" && msg.BrowserID != "none" {
			// make sure we have a buffer setup for this id
			var ok bool
			if buffer, ok = MessageBuffer[msg.BrowserID]; !ok {
				buffer = &circularbuffer.CircularBuffer[OutgoingMessage]{}
				MessageBuffer[msg.BrowserID] = buffer
			}
		}

		// any action is going to kickstart consuming messages in the background
		go func() {
			consumeMessages(conn, buffer)
		}()

		// Action tells us what we want to do, so this is a pretty simple router for the various requests
		// Each handler should be in its own handler_* file in the ws package
		switch msg.Action {
		case "start":
			StartContainerHandler(buffer, msg)
			data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
			conn.WriteMessage(mt, data)
		case "stop":
			StopContainerHandler(buffer, msg)
			data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
			conn.WriteMessage(mt, data)
		case "create":
			CreateContainerHandler(buffer, msg)
			data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
			conn.WriteMessage(mt, data)
		case "delete":
			DeleteContainerHandler(buffer, msg)
			data, _ := json.Marshal(OutgoingMessage{Redirect: "/containers"})
			conn.WriteMessage(mt, data)
		case "move":
			MoveContainerHandler(buffer, msg)
			data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
			conn.WriteMessage(mt, data)
		case "playbook":
			ContainerPlaybookHandler(buffer, msg)
		case "consume":
			// a noop since we always kickstart consuming when we get a message
		default:
			id := time.Now().UnixNano()
			data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "Request not understood", Success: false})
			conn.WriteMessage(mt, data)
		}
	}
}

func consumeMessages(conn *websocket.Conn, buffer *circularbuffer.CircularBuffer[OutgoingMessage]) {
	if buffer != nil {
		pingWait := 0
		for {
			msg, ok := buffer.Dequeue()
			if ok {
				data, err := json.Marshal(msg)
				if err == nil {
					// outgoing messages should always be of a TextMessage type
					err := conn.WriteMessage(websocket.TextMessage, data)
					if err != nil {
						// we lost the message, probably because the connection went away, so we will stop consuming
						break
					}
				}
			} else {
				pingWait++
				// 4 since we sleep for 250ms then the number of seconds we want to wait
				if pingWait == 4*10 {
					err := conn.WriteMessage(websocket.PingMessage, nil)
					if err != nil {
						// connection likely broken here, stop consuming
						break
					}
					pingWait = 0
				}
			}
			// if we have nothing to consume, wait some amount of time, 1/4 second seems like a good start
			time.Sleep(250 * time.Millisecond)
		}
	}
}

// BootstrapContainer loops over all the FileOrCommand objects in the bootstrap section of the config
// and performs each item sequentially
func BootstrapContainer(buffer *circularbuffer.CircularBuffer[OutgoingMessage], host string, name string) {
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Getting container state", Success: true})
	}

	// Get the container state again, should probably just grab this once but for now lets be expensive
	containerInfo, err := lxd.GetContainers(host, name, true)
	if err != nil {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		}
		return
	}
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "done", Success: true})
	}

	// if we have a bootstrap section for this OS, run it
	os := strings.ToLower(containerInfo[0].Container.ExpandedConfig["image.os"] + containerInfo[0].Container.ExpandedConfig["image.release"])
	if bootstrap, ok := Conf.Bootstrap[os]; ok {
		go func() {
			for _, step := range bootstrap {
				// depending on the type, call the appropriate helper
				if step.Type == "file" {
					err = containerCreateFile(buffer, host, name, step)
					if err != nil {
						return
					}
				} else if step.Type == "command" {
					err = containerExecCommand(buffer, host, name, step)
					if err != nil {
						return
					}
				}
			}
		}()
	}
}

// containerCreateFile operates on a Type = file bootstrap / playbook step.
// If there is a local_path, it reads the contents of that file from disk.
// The contents are then sent to the lxd.CreateFile with the path on the container and permissions to "do the right thing"
func containerCreateFile(buffer *circularbuffer.CircularBuffer[OutgoingMessage], host string, name string, info config.FileOrCommand) error {
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Creating " + info.RemotePath, Success: true})
	}

	// log what we are doing so anyone looking at the server will know
	log.Printf("creating file on container %v: %v\n", name, info.RemotePath)

	var contents []byte
	var err error
	if info.LocalPath != "" {
		contents, err = os.ReadFile(info.LocalPath)
		if err != nil {
			if buffer != nil {
				buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
			}
			return err
		}
	}

	err = lxd.CreateFile(host, name, info.RemotePath, info.Perms, string(contents))
	if err != nil {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		}
		return err
	}

	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "done", Success: true})
	}
	return nil
}

// containerExecCommand operates on a Type = command bootstrap / playbook step.
// This is really just a wrapper around lxd.ExecCommand
func containerExecCommand(buffer *circularbuffer.CircularBuffer[OutgoingMessage], host string, name string, info config.FileOrCommand) error {
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Executing " + strings.Join(info.Command, " "), Success: true})
	}

	// log what we are doing so anyone looking at the server will know
	log.Printf("running command on container %v: %v\n", name, info.Command)

	success := false
	attempt := 1
	var rv float64
	var err error
	for !success && attempt <= 2 {
		rv, err = lxd.ExecCommand(host, name, info.Command)
		if err != nil {
			if buffer != nil {
				buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
			}
			return err
		}

		// check our return value for real ok (0) or acceptable ok (info.OkReturnValues)
		if rv == 0 {
			success = true
		} else {
			for _, okrv := range info.OkReturnValues {
				if rv == okrv {
					success = true
				}
			}
		}

		attempt++
	}

	if !success {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: fmt.Sprintf("failed with return value: %v", rv), Success: false})
		}
		return errors.New("command failed")
	}

	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "done", Success: true})
	}
	return nil
}
