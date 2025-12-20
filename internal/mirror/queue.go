package mirror

import (
	"container/heap"
	"sync"
)

// URLQueue is a thread-safe priority queue for URLs.
type URLQueue struct {
	mu    sync.RWMutex
	items priorityQueue
	seen  map[string]bool // Track URLs already in queue
}

// NewURLQueue creates a new URL queue.
func NewURLQueue() *URLQueue {
	q := &URLQueue{
		items: make(priorityQueue, 0),
		seen:  make(map[string]bool),
	}
	heap.Init(&q.items)
	return q
}

// Push adds a URL to the queue with the given priority and depth.
func (q *URLQueue) Push(url string, priority URLPriority, depth int, parent string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Skip if already in queue
	if q.seen[url] {
		return false
	}

	item := &QueueItem{
		URL:      url,
		Priority: priority,
		Depth:    depth,
		Parent:   parent,
	}

	heap.Push(&q.items, item)
	q.seen[url] = true
	return true
}

// Pop removes and returns the highest priority URL from the queue.
func (q *URLQueue) Pop() *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.items.Len() == 0 {
		return nil
	}

	item := heap.Pop(&q.items).(*QueueItem)
	delete(q.seen, item.URL)
	return item
}

// Len returns the number of items in the queue.
func (q *URLQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.items.Len()
}

// IsEmpty returns true if the queue is empty.
func (q *URLQueue) IsEmpty() bool {
	return q.Len() == 0
}

// Contains checks if a URL is in the queue.
func (q *URLQueue) Contains(url string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.seen[url]
}

// Clear removes all items from the queue.
func (q *URLQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make(priorityQueue, 0)
	q.seen = make(map[string]bool)
	heap.Init(&q.items)
}

// priorityQueue implements heap.Interface for QueueItem.
type priorityQueue []*QueueItem

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority first
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// Same priority: shallower depth first
	return pq[i].Depth < pq[j].Depth
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
	item := x.(*QueueItem)
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*pq = old[0 : n-1]
	return item
}
