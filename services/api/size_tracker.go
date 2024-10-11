package api

import (
	"fmt"
	"sync"

	"github.com/AnomalyFi/hypersdk/consts"
)

const DefaultSizeLimit = consts.NetworkSizeLimit - 500*1024 // 2MB - 500 KB
const ToBTrackerTag = "TOB"

type SizeTracker struct {
	l sync.Mutex

	bids map[string]int // chainID -> txs size
	size int
	slot uint64

	limit int // limit in number of byte
}

func NewSizeTracker(limit int) *SizeTracker {
	return &SizeTracker{
		bids:  make(map[string]int),
		size:  0,
		limit: limit,
	}
}

func (t *SizeTracker) SetSlot(slot uint64) {
	t.l.Lock()
	defer t.l.Unlock()

	t.slot = slot
}

func (t *SizeTracker) TryUpdate(chainID string, slot uint64, size int) error {
	t.l.Lock()
	defer t.l.Unlock()

	if slot != t.slot {
		return fmt.Errorf("incorrect slot to update, have: %d, want: %d", slot, t.slot)
	}

	if size+t.size > t.limit {
		return fmt.Errorf("cannot update since exceeding limitation")
	}

	return nil
}

func (t *SizeTracker) UpdateToB(slot uint64, size int) error {
	return t.Update(ToBTrackerTag, slot, size)
}

func (t *SizeTracker) Update(chainID string, slot uint64, size int) error {
	t.l.Lock()
	defer t.l.Unlock()

	if slot != t.slot {
		return fmt.Errorf("incorrect slot to update, have: %d, want: %d", slot, t.slot)
	}

	if size+t.size > t.limit {
		return fmt.Errorf("cannot update since exceeding limitation, limit: %d, tracker: %d, added: %d", t.limit, t.size, size)
	}
	if _, ok := t.bids[chainID]; ok {
		t.size -= t.bids[chainID]
	}

	t.size += size
	t.bids[chainID] = size

	return nil
}

func (t *SizeTracker) Clear() {
	t.bids = make(map[string]int)
	t.size = 0
}
