package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/neophenix/lxdepot/internal/dns"
	"github.com/neophenix/lxdepot/internal/lxd"
	"time"
)

// DeleteContainerHandler first stops a running container (there is no force like the lxc command line),
// then deletes any DNS entry for it from our 3rd party, and then deletes the container.
func DeleteContainerHandler(conn *websocket.Conn, mt int, msg IncomingMessage) {
	// Stop the container
	err := StopContainerHandler(conn, mt, msg)
	if err != nil {
		// The other handler would have taken care of the message
		return
	}

	// DNS? If this fails I don't think it is enough reason to bail, will see
	// -------------------------
	if !Conf.DNS.DHCP {
		id := time.Now().UnixNano()
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "Deleting DNS entry", Success: true})
		conn.WriteMessage(mt, data)

		d := dns.New(Conf)
		if d == nil {
			data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed to create DNS object for provider: " + Conf.DNS.Provider, Success: false})
			conn.WriteMessage(mt, data)
		} else {
			err := d.RemoveARecord(msg.Data["name"])
			if err != nil {
				data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
				conn.WriteMessage(mt, data)
			} else {
				data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "done", Success: true})
				conn.WriteMessage(mt, data)
			}
		}
	}
	// -------------------------

	// Delete the container
	id := time.Now().UnixNano()
	data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "Deleting container", Success: true})
	conn.WriteMessage(mt, data)

	err = lxd.DeleteContainer(msg.Data["host"], msg.Data["name"])
	if err != nil {
		data, _ := json.Marshal(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		conn.WriteMessage(mt, data)
		return
	}

	data, _ = json.Marshal(OutgoingMessage{ID: id, Message: "done", Success: true, Redirect: "/containers"})
	conn.WriteMessage(mt, data)
}
