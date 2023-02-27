package circularbuffer

// a simple circular buffer to store messages that our web clients can read later, allowing command results to be stored
// and read later so new page loads can read what they may have requested previously.

import (
	"sync"
	"time"
)

// BUFLEN is the length of our buffer, not sure what a good value is here, doesn't need to hold everything but does need
// to be enough to maintain context on what users are seeing when they read it
const BUFLEN = 20

// RECENTACCESS is the number of seconds that we compare againt the lastAccess time of a buffer to determine if it was
// "recent" or not
const RECENTACCESS = 86400

type CircularBuffer[T any] struct {
	head       int8 // "write" pointer
	tail       int8 // "read" pointer
	buffer     [BUFLEN]T
	lastAccess time.Time
	lock       sync.Mutex
}

// Enqueue adds a string to our buffer and moves the head foward.  If the new head pointer is going to equal our tail
// then we are overwriting unread messages and need to push the tail ahead to maintain our circle
func (c *CircularBuffer[T]) Enqueue(msg T) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// record that we have accessed the buffer
	c.lastAccess = time.Now()
	// put our message in
	c.buffer[c.head] = msg
	// get our new head pointer
	newHead := (c.head + 1) % BUFLEN
	if newHead == c.tail {
		// maintain our circle if we have caught up to the tail, technically we could still read this tail value and
		// move on, but it greatly simplifies things to just move it
		c.tail = (c.tail + 1) % BUFLEN
	}
	// finally move the head forward
	c.head = newHead
}

// Dequeue takes the first unread message, moves our tail pointer ahead and returns the message.  We also return an "ok"
// boolean here so we can distinguish between an actual empty string and no message
func (c *CircularBuffer[T]) Dequeue() (T, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var msg T
	// record that we have accessed the buffer
	c.lastAccess = time.Now()
	// if our head and tail are equal there is nothing in our buffer to return
	if c.head == c.tail {
		return msg, false
	}

	msg = c.buffer[c.tail]
	c.tail = (c.tail + 1) % BUFLEN
	return msg, true
}

// HasRecentAccess returns true / false if a buffer has been "recently" accessed.  Check the value of RECENTACCESS for
// what we consider recent
func (c *CircularBuffer[T]) HasRecentAccess() bool {
	// buffers shouldn't be created until they are about to be used, this is internal so that should be fine.  So that
	// being the case we would make it and then immediately put something in it which would set the lastAccess.  So if
	// its not set then we assume its very new.  This could lead to memory leaks and we will need to move to a constructor
	// but for now lets experiment with this.
	if c.lastAccess.IsZero() {
		return true
	}
	diff := time.Now().Sub(c.lastAccess)
	// we don't need to go too crazy, just compare to seconds in a day
	if diff.Seconds() < RECENTACCESS {
		return true
	}
	return false
}
