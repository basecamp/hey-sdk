package hey

import (
	"context"
	"sync"
	"time"
)

// BulkheadConfig configures concurrency limiting.
type BulkheadConfig struct {
	MaxConcurrent int
	MaxWait       time.Duration
}

// DefaultBulkheadConfig returns production-ready defaults.
func DefaultBulkheadConfig() *BulkheadConfig {
	return &BulkheadConfig{
		MaxConcurrent: 10,
		MaxWait:       5 * time.Second,
	}
}

type bulkhead struct {
	config *BulkheadConfig
	sem    chan struct{}
}

func newBulkhead(config *BulkheadConfig) *bulkhead {
	if config == nil {
		config = DefaultBulkheadConfig()
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 10
	}
	if config.MaxWait < 0 {
		config.MaxWait = 5 * time.Second
	}

	return &bulkhead{
		config: config,
		sem:    make(chan struct{}, config.MaxConcurrent),
	}
}

func (b *bulkhead) Acquire(ctx context.Context) (release func(), err error) {
	if b.config.MaxWait == 0 {
		select {
		case b.sem <- struct{}{}:
			return func() { <-b.sem }, nil
		default:
			return nil, ErrBulkheadFull
		}
	}

	waitCtx := ctx
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > b.config.MaxWait {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, b.config.MaxWait)
		defer cancel()
	}

	select {
	case b.sem <- struct{}{}:
		return func() { <-b.sem }, nil
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrBulkheadFull
	}
}

func (b *bulkhead) TryAcquire() (release func(), ok bool) {
	select {
	case b.sem <- struct{}{}:
		return func() { <-b.sem }, true
	default:
		return nil, false
	}
}

func (b *bulkhead) Available() int {
	return b.config.MaxConcurrent - len(b.sem)
}

func (b *bulkhead) InUse() int {
	return len(b.sem)
}

type bulkheadRegistry struct {
	config    *BulkheadConfig
	mu        sync.RWMutex
	bulkheads map[string]*bulkhead
}

func newBulkheadRegistry(config *BulkheadConfig) *bulkheadRegistry {
	if config == nil {
		config = DefaultBulkheadConfig()
	}
	return &bulkheadRegistry{
		config:    config,
		bulkheads: make(map[string]*bulkhead),
	}
}

func (r *bulkheadRegistry) get(scope string) *bulkhead {
	r.mu.RLock()
	bh, ok := r.bulkheads[scope]
	r.mu.RUnlock()
	if ok {
		return bh
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if bh, ok = r.bulkheads[scope]; ok {
		return bh
	}

	bh = newBulkhead(r.config)
	r.bulkheads[scope] = bh
	return bh
}
