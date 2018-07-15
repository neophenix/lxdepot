package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/neophenix/lxdepot/internal/lxd"
	"time"
)

// MoveContainerHandler wraps lxd.MoveContainer and reports any errors it returns
func MoveContainerHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
	id := time.Now().UnixNano()
	data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "Moving container", Success: true})
	conn.WriteMessage(mt, data)

	err := lxd.MoveContainer(msg.Data["host"], msg.Data["dst_host"], msg.Data["name"])
	if err != nil {
		data, _ := json.Marshal(OutgoingMessage{Id: id, Message: "failed: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return
	}

	data, _ = json.Marshal(OutgoingMessage{Id: id, Message: "done", Success: false})
	conn.WriteMessage(mt, data)
}
