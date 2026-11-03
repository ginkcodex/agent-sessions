package app

import (
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
)

// catalogChangedEvent is the desktop event name carrying CatalogChanged.
const catalogChangedEvent = engine.CatalogChangedEvent

// catalogDebounce is the trailing window that coalesces rapid scan commits.
const catalogDebounce = 100 * time.Millisecond

// CatalogChanged delivers coalesced catalog updates to the desktop UI.
type CatalogChanged = engine.CatalogChanged

// Emitter sends named events to subscribers.
type Emitter = engine.Emitter

// stopper is the subset of *time.Timer the bus needs; tests substitute it.
type stopper = engine.Stopper

// catalogBus aliases engine.CatalogBus.
type catalogBus = engine.CatalogBus

// newCatalogBusWith allows custom emission callback and timer (for tests).
func newCatalogBusWith(emit func(CatalogChanged), afterFunc func(time.Duration, func()) stopper, delay time.Duration) *catalogBus {
	emitter := engine.EmitterFunc(func(name string, payload any) {
		if ev, ok := payload.(CatalogChanged); ok && emit != nil {
			emit(ev)
		}
	})
	return engine.NewCatalogBusWith(emitter, afterFunc, delay)
}
