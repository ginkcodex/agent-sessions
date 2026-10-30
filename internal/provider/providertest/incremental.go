package providertest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// IncrementalHarness checks a sequence of source snapshots against cold loads.
// Callers control the source explicitly, so it works with files, sidecars and
// metadata-only changes. Keep one harness per sequence and call Check after each
// mutation. State is consumed exactly once, including after errors. Saved deep
// copies detect later mutation of any previously published snapshot.
// Tests should use every complete-line boundary for small fixtures and sampled
// boundaries for large fixtures. Each cold provider must resolve the same source.
type IncrementalHarness struct {
	state     provider.TranscriptState
	previous  *model.Transcript
	snapshots []*model.Transcript
	copies    []*model.Transcript
}

// Check loads the current source incrementally and cold, checking equivalence,
// the earliest changed index, and all earlier snapshots' immutability.
func (h *IncrementalHarness) Check(ctx context.Context, p, cold provider.Provider, ref model.SessionRef) error {
	prev := h.state
	h.state = nil
	result, err := provider.LoadIncremental(ctx, p, ref, prev)
	if err != nil {
		if immutableErr := h.CheckImmutable(); immutableErr != nil {
			return immutableErr
		}
		return err
	}
	if err := h.CheckImmutable(); err != nil {
		return err
	}
	want, err := cold.Load(ctx, ref)
	if err != nil {
		return fmt.Errorf("cold load: %w", err)
	}
	if result.Transcript == nil || !reflect.DeepEqual(result.Transcript, want) {
		return fmt.Errorf("incremental transcript differs from cold load")
	}
	if result.FirstChanged < 0 || result.FirstChanged > len(result.Transcript.Messages) {
		return fmt.Errorf("invalid FirstChanged %d", result.FirstChanged)
	}
	if !result.Resumed && result.FirstChanged != 0 {
		return fmt.Errorf("rebuild FirstChanged must be zero")
	}
	if h.previous != nil {
		first := FirstMessageDifference(h.previous.Messages, result.Transcript.Messages)
		if result.FirstChanged > first {
			return fmt.Errorf("FirstChanged %d above true difference %d", result.FirstChanged, first)
		}
	} else if result.FirstChanged != 0 || result.Resumed {
		return fmt.Errorf("initial load must rebuild")
	}
	copy, err := CloneTranscript(result.Transcript)
	if err != nil {
		return err
	}
	h.snapshots = append(h.snapshots, result.Transcript)
	h.copies = append(h.copies, copy)
	h.previous = copy
	h.state = result.State
	return nil
}

// CheckImmutable can also be called after failed or cancelled loads.
func (h *IncrementalHarness) CheckImmutable() error {
	for i := range h.snapshots {
		if !reflect.DeepEqual(h.snapshots[i], h.copies[i]) {
			return fmt.Errorf("published snapshot %d was mutated", i)
		}
	}
	return nil
}

// FirstMessageDifference is the common-prefix length, including append-only and
// truncation cases. It deliberately compares all nested data.
func FirstMessageDifference(a, b []model.Message) int {
	i := 0
	for i < len(a) && i < len(b) && reflect.DeepEqual(a[i], b[i]) {
		i++
	}
	return i
}

// CloneTranscript preserves nil/empty distinctions by recursively copying slices
// and pointer targets rather than going through the omitempty JSON wire shape.
func CloneTranscript(tr *model.Transcript) (*model.Transcript, error) {
	if tr == nil {
		return nil, nil
	}
	out := *tr
	if tr.Messages != nil {
		out.Messages = make([]model.Message, len(tr.Messages))
		copy(out.Messages, tr.Messages)
	}
	for i := range out.Messages {
		m := &out.Messages[i]
		if m.Parts != nil {
			parts := make([]model.Part, len(m.Parts))
			copy(parts, m.Parts)
			m.Parts = parts
		}
		for j := range m.Parts {
			part := &m.Parts[j]
			if part.Files != nil {
				files := make([]string, len(part.Files))
				copy(files, part.Files)
				part.Files = files
			}
			if part.File != nil {
				file := *part.File
				part.File = &file
			}
			if part.Tool != nil {
				tool := *part.Tool
				if tool.Input != nil {
					tool.Input = append(make(json.RawMessage, 0, len(tool.Input)), tool.Input...)
				}
				if tool.Child != nil {
					child := *tool.Child
					tool.Child = &child
				}
				part.Tool = &tool
			}
		}
	}
	return &out, nil
}
