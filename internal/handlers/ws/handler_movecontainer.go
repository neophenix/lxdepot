package ws

import (
	"time"

	"github.com/neophenix/lxdepot/internal/circularbuffer"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// MoveContainerHandler wraps lxd.MoveContainer and reports any errors it returns
func MoveContainerHandler(buffer *circularbuffer.CircularBuffer[OutgoingMessage], msg IncomingMessage) {
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Moving container", Success: true})
	}

	err := lxd.MoveContainer(msg.Data["host"], msg.Data["dst_host"], msg.Data["name"])
	if err != nil {
		if buffer != nil {
			buffer.Enqueue(OutgoingMessage{ID: id, Message: "failed: " + err.Error(), Success: false})
		}
		return
	}

	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "done", Success: false})
	}
}
