package circularbuffer

import (
	"strings"
	"testing"
	"time"
)

type bufTest struct {
	op    string // operation to perform (enqueue,dequeue)
	value string // value or csv list of what to enqueue or values that will come from dequeues
	ok    bool   // only for dequeue, the ok result
	head  int8   // expected head value
	tail  int8   // expected tail value
}

// test enqueue and dequeue operations
func TestBuffer(t *testing.T) {
	buffer := &CircularBuffer[string]{}

	tests := []bufTest{
		{op: "dequeue", value: "", ok: false, head: 0, tail: 0},
		{op: "enqueue", value: "a", head: 1, tail: 0},
		{op: "enqueue", value: "b,c,d,e,f,g,h", head: 8, tail: 0},
		{op: "dequeue", value: "a", ok: true, head: 8, tail: 1},
		{op: "dequeue", value: "b", ok: true, head: 8, tail: 2},
		{op: "enqueue", value: "i,j,k,l,m,n,o,p,q,r,s,t", head: 0, tail: 2},
		{op: "dequeue", value: "c,d,e,f,g,h,i,j,k,l,m,n,o,p,q,r,s", ok: true, head: 0, tail: 19},
		{op: "dequeue", value: "t", ok: true, head: 0, tail: 0},
		{op: "dequeue", value: "", ok: false, head: 0, tail: 0},
	}

	for tidx, test := range tests {
		if test.op == "enqueue" {
			for _, v := range strings.Split(test.value, ",") {
				buffer.Enqueue(v)
			}
		} else if test.op == "dequeue" {
			for _, v := range strings.Split(test.value, ",") {
				bufval, ok := buffer.Dequeue()

				if test.ok != ok {
					t.Errorf("%v: expected ok %v got %v", tidx, test.ok, ok)
				}
				if v != bufval {
					t.Errorf("%v: expected %v got %v", tidx, v, bufval)
				}
			}
		}

		if test.head != buffer.head {
			t.Errorf("%v: expected head to be %v got %v", tidx, test.head, buffer.head)
		}
		if test.tail != buffer.tail {
			t.Errorf("%v: expected tail to be %v got %v", tidx, test.tail, buffer.tail)
		}
	}
}

// some basic tests for recent access where we will force various time objects into a buffer
func TestRecentAccess(t *testing.T) {
	buffer := &CircularBuffer[string]{}
	if !buffer.HasRecentAccess() {
		t.Error("expected new buffer HasRecentAccess to be true, but it is false")
	}

	buffer.lastAccess = time.Now().Add(-100 * time.Second)
	if !buffer.HasRecentAccess() {
		t.Error("expected recent buffer HasRecentAccess to be true, but it is false")
	}

	buffer.lastAccess = time.Now().Add(-86401 * time.Second)
	if buffer.HasRecentAccess() {
		t.Error("expected old buffer HasRecentAccess to be false, but it is true")
	}
}
