package queue

import (
	"container/list"
	"fmt"
	"sync"

)

// this usecase implements EventQueue interface with a memory stored queue (FIFO)
type eventQueueMem struct {
	mutex                *sync.Mutex
	cond                 *sync.Cond
	cache                *list.List
	maxAmountOfEvents    int // max number of cached events possible
	eventsCacheMemSizeMB int
	verbose              string
}

func NewEventQueueMem(queueSizeMb int) CacheConfig {
	q := &eventQueueMem{
		eventsCacheMemSizeMB: queueSizeMb,
	}
	q.setup()
	q.verbose = fmt.Sprintf("In-Memory Event Queue (Size = %d MB)", q.eventsCacheMemSizeMB)
	return q
}

func (q *eventQueueMem) String() string {
	return q.verbose
}

func (q *eventQueueMem) setup() {
	q.mutex = new(sync.Mutex)
	q.cond = sync.NewCond(q.mutex)

	// set queue size and init queue
	q.maxAmountOfEvents = q.getQueueSizeInEvents()
	q.cache = new(list.List)
}

// Enqueue pushes an event into the queue (may block until queue is available)
func (q *eventQueueMem) Enqueue(evt *trace.Event) {
	q.cond.L.Lock()
	// enqueue waits for de-queuing if cache is full (using >= instead of == to be in the safe side...)
	for q.cache.Len() >= q.maxAmountOfEvents {
		q.cond.Wait()
	}

	q.cache.PushBack(*evt)
	q.cond.L.Unlock()
	q.cond.Signal() // unblock dequeue if needed

	evt = nil
}

// Dequeue pops an event from the queue
func (q *eventQueueMem) Dequeue() *trace.Event {
	q.cond.L.Lock()

	// dequeue waits for en-queueing if cache is empty
	for q.cache.Len() == 0 {
		q.cond.Wait()
	}

	e := q.cache.Front()
	event, ok := e.Value.(trace.Event)
	if !ok {
		q.cond.L.Unlock()
		return nil
	}
	q.cache.Remove(e)
	q.cond.L.Unlock()
	q.cond.Signal() // unblock enqueue if needed

	return &event
}


func (q *eventQueueMem) getQueueSizeInEvents() int {
	
	eventSize := 1024

	KBtoB := func(amountInKB int) int {
		return amountInKB * 1024
	}
	MBtoKB := func(amountInMB int) int {
		return amountInMB * 1024
	}
	GBtoMB := func(amountInGB int) int {
		return amountInGB * 1024
	}
	AmountOfEvents := func(amountInMB int) int {
		return MBtoKB(KBtoB(amountInMB)) / eventSize
	}
	if q.eventsCacheMemSizeMB > 0 {
		return AmountOfEvents(q.eventsCacheMemSizeMB)
	}

	switch {
	case q.eventsCacheMemSizeMB <= GBtoMB(1): // up to 1GB, cache = ~256MB in events #
		return AmountOfEvents(256)
	case q.eventsCacheMemSizeMB <= GBtoMB(4): // up to 4GB, cache = ~512MB in events #
		return AmountOfEvents(512)
	case q.eventsCacheMemSizeMB <= GBtoMB(8): // up to 8GB, cache = ~1GB in events #
		return AmountOfEvents(GBtoMB(1))
	case q.eventsCacheMemSizeMB <= GBtoMB(16): // up to 16GB, cache = ~2GB in events #
		return AmountOfEvents(GBtoMB(2))
	}

	// bigger hosts, cache = ~4GB in events #
	return AmountOfEvents(GBtoMB(4))
}
