package ws

import (
	"time"

	"github.com/neophenix/lxdepot/internal/circularbuffer"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// StopContainerHandler stops a running container
func StopContainerHandler(buffer *circularbuffer.CircularBuffer[OutgoingMessage], msg IncomingMessage) error {
	// Stop the container
	id := time.Now().UnixNano()
	if buffer != nil {
		buffer.Enqueue(OutgoingMessage{ID: id, Message: "Stopping container", Success: true})
	}

	err := lxd.StopContainer(msg.Data["host"], msg.Data["name"])
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
