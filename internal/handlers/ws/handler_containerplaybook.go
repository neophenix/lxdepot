package ws

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// ContainerPlaybookHandler handles requests to run various playbooks on the container, including
// re-bootstrapping it if asked.  Playbooks and bootstrap should be idempotent so no harm should come
// from running these multiple times.
func ContainerPlaybookHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
	containerInfo, err := lxd.GetContainers(msg.Data["host"], msg.Data["name"], false)
	if err != nil {
		id := time.Now().UnixNano()
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed to get container info: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return
	}

	// Check first to make sure the container exists and we are allowed to manage it
	if len(containerInfo) > 0 {
		if !lxd.IsManageable(containerInfo[0]) {
			id := time.Now().UnixNano()
			data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "lock flag set, remote management denied", Success: false})
			conn.WriteMessage(mt, data)
			return
		}
	} else {
		id := time.Now().UnixNano()
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "container does not exist", Success: false})
		conn.WriteMessage(mt, data)
		return
	}

	os := strings.ToLower(containerInfo[0].Container.ExpandedConfig["image.os"] + containerInfo[0].Container.ExpandedConfig["image.release"])
	// bootstrap is a special playbook in that it has its own section of the config.  If we are asked to
	// do this again, just call the bootstrap "handler" in handler_createcontainer
	if msg.Data["playbook"] == "bootstrap" {
		BootstrapContainer(conn, mt, msg.Data["host"], msg.Data["name"])
	} else if playbooks, ok := Conf.Playbooks[os]; ok {
		if playbook, ok := playbooks[msg.Data["playbook"]]; ok {
			// Once we are sure the OS for this image exists in or config and we have the requested playbook
			// run it in basically the same fashion we run a boostrap
			go func() {
				for _, step := range playbook {
					// depending on the type, call the appropriate helper
					if step.Type == "file" {
						err = containerCreateFile(conn, mt, msg.Data["host"], msg.Data["name"], step)
						if err != nil {
							return
						}
					} else if step.Type == "command" {
						err = containerExecCommand(conn, mt, msg.Data["host"], msg.Data["name"], step)
						if err != nil {
							return
						}
					}
				}
			}()
		}
	}
}
