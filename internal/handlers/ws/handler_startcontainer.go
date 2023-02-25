package ws

import (
	"time"

	"github.com/neophenix/lxdepot/internal/circularbuffer"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// StartContainerHandler starts a stopped container
func StartContainerHandler(buffer *circularbuffer.CircularBuffer[OutgoingMessage], msg IncomingMessage) error {
	// Start the container
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Starting container", Success: true})
	}

	err := lxd.StartContainer(msg.Data["host"], msg.Data["name"])
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
