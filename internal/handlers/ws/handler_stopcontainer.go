package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/neophenix/lxdepot/internal/lxd"
	"time"
)

// StopContainerHandler stops a running container
func StopContainerHandler(conn *websocket.Conn, mt int, msg IncomingMessage) error {
	// Stop the container
	id := time.Now().UnixNano()
	data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Stopping container", Success: true})
	conn.WriteMessage(mt, data)

	err := lxd.StopContainer(msg.Data["host"], msg.Data["name"])
	if err != nil {
		data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return err
	}

	data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: true})
	conn.WriteMessage(mt, data)

	return nil
}
