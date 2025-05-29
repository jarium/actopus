package actopus

import "sync"

type MessageQueue interface {
	Add(m Message)
	Pop(n int) ([]Message, bool)
	Len() int
}

type RingBufferMessageQueue struct {
	mu     sync.Mutex
	buffer []Message
	size   int
	read   int
	write  int
	count  int
}

func NewRingBufferMessageQueue(size int) *RingBufferMessageQueue {
	return &RingBufferMessageQueue{
		buffer: make([]Message, size),
		size:   size,
	}
}

func (rb *RingBufferMessageQueue) Add(m Message) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.buffer[rb.write] = m
	rb.write = (rb.write + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	}
}

func (rb *RingBufferMessageQueue) Pop(n int) ([]Message, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 || n <= 0 {
		return nil, false
	}
	if n > rb.count {
		n = rb.count
	}

	rb.count -= n

	items := make([]Message, n)
	for i := 0; i < n; i++ {
		items[i] = rb.buffer[rb.read]
		rb.read = (rb.read + 1) % rb.size
	}

	return items, true
}

func (rb *RingBufferMessageQueue) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}
