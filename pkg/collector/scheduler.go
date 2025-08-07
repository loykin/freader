package collector

import (
	"container/list"
	"freader/pkg/tailer"
	"log/slog"
	"sync"
)

type TailScheduler struct {
	available *list.List
	cursor    *list.Element
	index     map[string]*list.Element
	mu        sync.Mutex
	running   map[string]bool
}

func NewTailScheduler() *TailScheduler {
	return &TailScheduler{
		available: list.New(),
		running:   make(map[string]bool),
		index:     make(map[string]*list.Element),
	}
}

func (t *TailScheduler) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if elem, exists := t.index[id]; exists {
		t.available.Remove(elem)
		delete(t.index, id)
		delete(t.running, id)

		if t.cursor == elem {
			t.cursor = elem.Next()
			if t.cursor == nil {
				t.cursor = t.available.Front()
			}
		}
	}
}

func (t *TailScheduler) GetCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.available.Len()
}

func (t *TailScheduler) Add(id string, fileTail *tailer.TailReader, update bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if update == false {
		if _, exists := t.index[id]; exists {
			slog.Debug("file already exists", "id", id)
			return
		}
	}

	elem := t.available.PushBack(fileTail)
	t.index[id] = elem

	if t.cursor == nil {
		t.cursor = t.available.Front()
	}
}

func (t *TailScheduler) SetIdle(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.index[id]; ok {
		t.running[id] = false
	}
}

func (t *TailScheduler) getNextAvailable() (*tailer.TailReader, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.available.Len() == 0 {
		return nil, false
	}

	startCursor := t.cursor
	for {
		if t.cursor == nil {
			t.cursor = t.available.Front()
		}

		if fileTail, ok := t.cursor.Value.(*tailer.TailReader); ok {
			if running, exists := t.running[fileTail.FileId]; !exists || !running {
				t.running[fileTail.FileId] = true
				t.cursor = t.cursor.Next()
				return fileTail, true
			}
		}

		t.cursor = t.cursor.Next()

		if t.cursor == startCursor {
			break
		}
	}

	return nil, false
}
