package codex

import (
	"bytes"
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

const (
	checkpointCacheBytes   = 16 << 20
	checkpointCacheEntries = 1024
)

// Entries own their raw bytes and immutable decoded state. Mutable append
// parsing must decode separately, rather than sharing the checkpoint's maps.
type checkpointEntry struct {
	raw      string
	cp       *checkpoint
	retained int64
}

type checkpointCache struct {
	mu       sync.Mutex
	entries  map[string]checkpointEntry
	retained int64
}

func (c *checkpointCache) lookup(path string, raw []byte) *checkpoint {
	c.mu.Lock()
	e, ok := c.entries[path]
	c.mu.Unlock()
	if ok && e.cp.Version == checkpointVersion && bytes.Equal([]byte(e.raw), raw) {
		return e.cp
	}
	return nil
}

// Budget includes owned JSON, decoded strings (bounded by JSON length), struct,
// slice storage, and a conservative 256 bytes per map entry/bucket. Path and
// entry/map bookkeeping get a further 512 bytes. This is an admission estimate,
// not a measurement of the Go allocator's retained heap.
func checkpointRetained(path string, raw []byte, cp *checkpoint) int64 {
	return int64(2*len(raw) + len(path) + 512 + int(unsafe.Sizeof(*cp)) + int(unsafe.Sizeof(model.SessionMeta{})) +
		256*(len(cp.AssistantIDs)+len(cp.CallIDs)) + 16*cap(cp.PendingCalls))
}

// Publish a bounded snapshot in sorted discovery order. Concurrent scans may
// evict useful entries, but exact raw identity makes an older publication safe.
func (c *checkpointCache) publish(paths []string, state provider.ScanState, decoded map[string]*checkpoint) {
	c.mu.Lock()
	previous := c.entries
	c.mu.Unlock()
	entries := make(map[string]checkpointEntry)
	var retained int64
	for _, path := range paths {
		cp := decoded[path]
		if cp == nil || cp.Version != checkpointVersion {
			continue
		}
		raw := state.Sources[path].Checkpoint
		n := checkpointRetained(path, raw, cp)
		if len(entries) >= checkpointCacheEntries || n > checkpointCacheBytes-retained {
			continue
		}
		if old, ok := previous[path]; ok && bytes.Equal([]byte(old.raw), raw) {
			entries[path] = old
		} else {
			// Canonical comes from live metadata, whose short strings can pin
			// large source buffers. Round-trip just this overlay to own strings
			// and match JSON's invalid-UTF-8 normalization exactly.
			owned := *cp
			if cp.Canonical != nil {
				encoded, err := json.Marshal(cp.Canonical)
				if err != nil {
					continue
				}
				owned.Canonical = new(model.SessionMeta)
				if json.Unmarshal(encoded, owned.Canonical) != nil {
					continue
				}
			}
			entries[path] = checkpointEntry{raw: string(raw), cp: &owned, retained: n}
		}
		retained += n
	}
	c.mu.Lock()
	c.entries, c.retained = entries, retained
	c.mu.Unlock()
}
