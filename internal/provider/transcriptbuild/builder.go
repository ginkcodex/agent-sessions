// Package transcriptbuild provides copy-on-write storage for transcript parsers.
// A builder has one writer; its frozen snapshots may have concurrent readers.
package transcriptbuild

import (
	"slices"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// ToolLoc identifies a tool without retaining a pointer into a frozen snapshot.
type ToolLoc struct{ Message, Part int }

// Builder owns messages supplied to Append. Callers must not retain mutable
// pointers across Freeze or Fork, or mutate Append's input after transferring it.
// Touch/Tool pointers are valid only until the next builder operation.
type Builder struct {
	messages []model.Message
	frozen   int
	detached bool
	touched  map[int]bool
	first    int
	// Weights include each message's fixed layout and nested content, not spare
	// capacity in the top-level message array. Freeze reconciles changed weights.
	weights []int64
	bytes   int64
}

// Len is the current message count.
func (b *Builder) Len() int { return len(b.messages) }

// At returns a read-only message value; its nested storage must not be mutated.
func (b *Builder) At(i int) model.Message { return b.messages[i] }

func (b *Builder) detach() {
	if b.detached {
		return
	}
	b.messages = slices.Clone(b.messages)
	b.detached = true
}

// Append transfers ownership of msg and returns its index.
func (b *Builder) Append(msg model.Message) int {
	i := len(b.messages)
	// Forked writers may share spare capacity: detach before writing any of it.
	b.detach()
	b.messages = append(b.messages, msg)
	b.weights = append(b.weights, 0)
	b.changed(i)
	return i
}

func (b *Builder) changed(i int) {
	if b.touched == nil {
		b.touched = make(map[int]bool)
	}
	if len(b.touched) == 0 || i < b.first {
		b.first = i
	}
	b.touched[i] = true
}

// Touch returns a writable message, deeply detaching nested storage on the first
// touch of a frozen message. The top-level array is copied at most once per freeze.
func (b *Builder) Touch(i int) *model.Message {
	_ = b.messages[i]
	if i < b.frozen && !b.touched[i] {
		b.detach()
		b.messages[i] = cloneMessage(b.messages[i])
	}
	b.changed(i)
	return &b.messages[i]
}

// Tool detaches and returns the tool at loc. Invalid locations panic, like Touch.
func (b *Builder) Tool(loc ToolLoc) *model.ToolCall {
	return b.Touch(loc.Message).Parts[loc.Part].Tool
}

// Freeze publishes a clipped immutable snapshot and the earliest changed index.
// No changes gives Len(). It reconciles sizes only for touched/appended messages.
func (b *Builder) Freeze() ([]model.Message, int) {
	first := b.Changed()
	for i := range b.touched {
		size := model.MessageBytes(&b.messages[i])
		b.bytes += size - b.weights[i]
		b.weights[i] = size
	}
	b.frozen = len(b.messages)
	b.detached = false
	clear(b.touched)
	return slices.Clip(b.messages), first
}

// Changed is the earliest index changed since the last Freeze, or Len() if none.
func (b *Builder) Changed() int {
	if len(b.touched) == 0 {
		return len(b.messages)
	}
	return b.first
}

// Bytes is the sum of message weights at the last Freeze. It excludes transcript
// metadata and spare capacity of the top-level message array.
func (b *Builder) Bytes() int64 { return b.bytes }

// Overhead estimates storage a snapshot never exposes: the weight table and
// spare capacity of the top-level message array.
func (b *Builder) Overhead() int64 {
	return int64(cap(b.weights))*8 + int64(cap(b.messages)-len(b.messages))*int64(unsafe.Sizeof(model.Message{}))
}

// Fork creates an independent writer over the current snapshot. It freezes the
// original first; both writers subsequently copy touched messages. Parser maps
// containing ToolLoc values must be cloned by the caller as well.
func (b *Builder) Fork() *Builder {
	b.Freeze()
	return &Builder{messages: b.messages, frozen: len(b.messages), weights: slices.Clone(b.weights), bytes: b.bytes}
}

func cloneMessage(m model.Message) model.Message {
	m.Parts = slices.Clone(m.Parts)
	for i := range m.Parts {
		p := &m.Parts[i]
		p.Files = slices.Clone(p.Files)
		if p.File != nil {
			file := *p.File
			p.File = &file
		}
		if p.Tool != nil {
			tool := *p.Tool
			tool.Input = slices.Clone(tool.Input)
			if tool.Child != nil {
				child := *tool.Child
				tool.Child = &child
			}
			p.Tool = &tool
		}
	}
	return m
}
