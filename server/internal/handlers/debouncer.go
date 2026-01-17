package handlers

import (
	"sync"
	"time"

	"github.com/zhaobenny/cctop/server/internal/database"
)

// SummaryDebouncer delays summary updates to batch multiple syncs together
type SummaryDebouncer struct {
	db      *database.DB
	delay   time.Duration
	mu      sync.Mutex
	pending map[string]*pendingUpdate
}

type pendingUpdate struct {
	generation int
	billingDay int
	records    []database.UsageRecord
}

// NewSummaryDebouncer creates a debouncer with the specified delay
func NewSummaryDebouncer(db *database.DB, delay time.Duration) *SummaryDebouncer {
	return &SummaryDebouncer{
		db:      db,
		delay:   delay,
		pending: make(map[string]*pendingUpdate),
	}
}

// Schedule queues a summary update for a user, resetting the timer if already pending
func (d *SummaryDebouncer) Schedule(userID string, billingDay int, records []database.UsageRecord) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if p, exists := d.pending[userID]; exists {
		// Append records and bump generation (invalidates old timer)
		p.records = append(p.records, records...)
		p.billingDay = billingDay
		p.generation++
		gen := p.generation
		time.AfterFunc(d.delay, func() {
			d.flush(userID, gen)
		})
		return
	}

	// Create new pending update
	d.pending[userID] = &pendingUpdate{
		generation: 1,
		billingDay: billingDay,
		records:    records,
	}
	time.AfterFunc(d.delay, func() {
		d.flush(userID, 1)
	})
}

func (d *SummaryDebouncer) flush(userID string, generation int) {
	d.mu.Lock()
	p, exists := d.pending[userID]
	if !exists || p.generation != generation {
		// Stale timer or already flushed
		d.mu.Unlock()
		return
	}
	delete(d.pending, userID)
	d.mu.Unlock()

	// Run the actual summary update
	d.db.UpdateSummaries(userID, p.billingDay, p.records)
}
