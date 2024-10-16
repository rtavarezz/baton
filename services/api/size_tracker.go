package api

import (
	"fmt"
	"sync"

	"github.com/AnomalyFi/hypersdk/consts"
)

const DefaultSizeLimit = consts.NetworkSizeLimit - 500*1024 // 2MB - 500 KB
const ToBTrackerTag = "TOB"

type Bids map[string]int

type SizeTracker struct {
	l sync.Mutex

	bids map[uint64]Bids
	size map[uint64]int

	// bids map[string]int // chainID -> txs size
	// size       int
	lowestSlot uint64

	limit int // limit in number of byte
}

func NewSizeTracker(limit int) *SizeTracker {
	return &SizeTracker{
		bids:  make(map[uint64]Bids),
		size:  make(map[uint64]int),
		limit: limit,
	}
}

func (t *SizeTracker) SetLowestSlot(slot uint64) {
	t.l.Lock()
	defer t.l.Unlock()

	t.lowestSlot = slot

	// clear slot-1 info
	t.Clear(slot - 1)
}

func (t *SizeTracker) TryUpdate(chainID string, slot uint64, size int) error {
	t.l.Lock()
	defer t.l.Unlock()

	if slot < t.lowestSlot {
		return fmt.Errorf("slot too low to update, have: %d, want: %d", slot, t.lowestSlot)
	}
	// new slot found, prepare
	if _, ok := t.bids[slot]; !ok {
		t.prepareNewSlot(slot)
	}

	if size+t.size[slot] > t.limit {
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

	if slot < t.lowestSlot {
		return fmt.Errorf("slot too low to update, have: %d, want: %d", slot, t.lowestSlot)
	}

	// new slot found, prepare
	if _, ok := t.bids[slot]; !ok {
		t.prepareNewSlot(slot)
	}

	if size+t.size[slot] > t.limit {
		return fmt.Errorf("cannot update since exceeding limitation, limit: %d, tracker: %d, added: %d", t.limit, t.size, size)
	}

	// make updates
	chainBids := t.bids[slot]
	if _, ok := chainBids[chainID]; ok {
		size := t.size[slot]
		size -= chainBids[chainID]
		t.size[slot] = size
	}

	slotSize := t.size[slot]
	slotSize += size
	t.size[slot] = slotSize

	chainBids[chainID] = size
	t.bids[slot] = chainBids

	return nil
}

func (t *SizeTracker) Clear(slot uint64) {
	delete(t.bids, slot)
	delete(t.size, slot)
}

func (t *SizeTracker) prepareNewSlot(slot uint64) {
	t.bids[slot] = make(Bids)
	t.size[slot] = 0
}
