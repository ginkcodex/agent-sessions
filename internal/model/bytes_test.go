package model

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestTranscriptBytesEmpty(t *testing.T) {
	if got := TranscriptBytes(nil); got != 0 {
		t.Fatalf("nil transcript: got %d, want 0", got)
	}
	if got, want := TranscriptBytes(&Transcript{}), int64(unsafe.Sizeof(Transcript{})); got != want {
		t.Fatalf("empty transcript: got %d, want %d", got, want)
	}
	if got := MessageBytes(nil); got != 0 {
		t.Fatalf("nil message: got %d, want 0", got)
	}
	if got, want := MessageBytes(&Message{}), int64(unsafe.Sizeof(Message{})); got != want {
		t.Fatalf("empty message: got %d, want %d", got, want)
	}
}

// Check each string field independently, including defined string types. This
// also catches new string fields added to these model types without accounting.
func TestBytesStringFields(t *testing.T) {
	tr := &Transcript{}
	m := &Message{}
	tool := &ToolCall{}
	file := &FileRef{}
	child := &SessionRef{}
	m.Parts = []Part{{}}
	part := &m.Parts[0]
	part.Tool, part.File, tool.Child = tool, file, child
	cases := []struct {
		name  string
		value any
		size  func() int64
	}{
		{"metadata", &tr.Meta, func() int64 { return TranscriptBytes(tr) }},
		{"session ref", &tr.Meta.Ref, func() int64 { return TranscriptBytes(tr) }},
		{"message", m, func() int64 { return MessageBytes(m) }},
		{"part", part, func() int64 { return MessageBytes(m) }},
		{"tool", tool, func() int64 { return MessageBytes(m) }},
		{"file", file, func() int64 { return MessageBytes(m) }},
		{"child", child, func() int64 { return MessageBytes(m) }},
	}
	for _, tc := range cases {
		v := reflect.ValueOf(tc.value).Elem()
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).Kind() != reflect.String {
				continue
			}
			t.Run(tc.name+"/"+v.Type().Field(i).Name, func(t *testing.T) {
				before := tc.size()
				v.Field(i).SetString("generated-世界")
				if got, want := tc.size()-before, int64(len("generated-世界")); got != want {
					t.Fatalf("string contribution: got %d, want %d", got, want)
				}
				v.Field(i).SetString("")
			})
		}
	}
}

func TestBytesNestedStorage(t *testing.T) {
	m := Message{Parts: make([]Part, 1, 3)}
	m.Parts[0] = Part{
		Tool:  &ToolCall{Input: make(json.RawMessage, 2, 17), Child: &SessionRef{}},
		File:  &FileRef{Size: 1 << 40}, // External attachment bytes are not retained.
		Files: make([]string, 2, 5),
	}
	m.Parts[0].Files[0], m.Parts[0].Files[1] = "abc", "世界"
	want := int64(unsafe.Sizeof(m)) + 3*int64(unsafe.Sizeof(Part{})) +
		int64(unsafe.Sizeof(ToolCall{})) + 17 + int64(unsafe.Sizeof(SessionRef{})) +
		int64(unsafe.Sizeof(FileRef{})) + 5*int64(unsafe.Sizeof("")) + int64(len("abc世界"))
	if got := MessageBytes(&m); got != want {
		t.Fatalf("nested message: got %d, want %d", got, want)
	}
	tr := Transcript{Messages: make([]Message, 1, 4)}
	tr.Messages[0] = m
	want += int64(unsafe.Sizeof(tr)) + 3*int64(unsafe.Sizeof(m))
	if got := TranscriptBytes(&tr); got != want {
		t.Fatalf("nested transcript: got %d, want %d", got, want)
	}
	// The value is repeatable and reading it does not modify the input.
	for i := 0; i < 3; i++ {
		if got := TranscriptBytes(&tr); got != want {
			t.Fatalf("repeat %d: got %d, want %d", i, got, want)
		}
	}
}

func TestBytesSharingAndUnusedCapacity(t *testing.T) {
	tool := &ToolCall{Output: "shared payload"}
	parts := []Part{{Tool: tool}, {Tool: tool}, {Text: "unused payload"}}
	m := Message{Parts: parts[:2]}
	want := int64(unsafe.Sizeof(m)) + 3*int64(unsafe.Sizeof(Part{})) +
		2*(int64(unsafe.Sizeof(*tool))+int64(len(tool.Output)))
	if got := MessageBytes(&m); got != want {
		t.Fatalf("sharing and unused capacity: got %d, want %d", got, want)
	}
	tr := Transcript{Messages: []Message{m, m}}
	if got, want := TranscriptBytes(&tr), int64(unsafe.Sizeof(tr))+2*want; got != want {
		t.Fatalf("repeated message: got %d, want %d", got, want)
	}
}

func TestBytesNonNilEmptySlicesAndScalarChanges(t *testing.T) {
	tr := Transcript{Messages: make([]Message, 0, 2)}
	want := int64(unsafe.Sizeof(tr)) + 2*int64(unsafe.Sizeof(Message{}))
	if got := TranscriptBytes(&tr); got != want {
		t.Fatalf("empty reserved messages: got %d, want %d", got, want)
	}
	tr.Meta.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("generated", 3600))
	tr.Meta.Tokens.Input = 123
	tr.Meta.Counts.Assistant = 42
	tr.Meta.Live = true
	tr.Meta.CostUSD = 10
	if got := TranscriptBytes(&tr); got != want {
		t.Fatalf("scalar values and time location must not affect size: got %d, want %d", got, want)
	}
	m := Message{Parts: []Part{{Files: make([]string, 0, 2), Tool: &ToolCall{Input: make(json.RawMessage, 0, 8)}}}}
	want = int64(unsafe.Sizeof(m)) + int64(unsafe.Sizeof(Part{})) +
		2*int64(unsafe.Sizeof("")) + int64(unsafe.Sizeof(ToolCall{})) + 8
	if got := MessageBytes(&m); got != want {
		t.Fatalf("empty reserved nested slices: got %d, want %d", got, want)
	}
}

// Deterministic random suffix edits, truncations, appends, capacity changes and
// metadata changes must match a full recompute from every changed index.
func TestTranscriptBytesSince(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	text := func() string { return strings.Repeat("x", rng.IntN(64)) }
	msg := func() Message {
		parts := make([]Part, rng.IntN(3))
		for i := range parts {
			parts[i] = Part{Text: text()}
			if rng.IntN(2) == 0 {
				parts[i].Tool = &ToolCall{Output: text()}
			}
		}
		return Message{ID: text(), Parts: parts}
	}
	prev := &Transcript{}
	bytes := TranscriptBytes(prev)
	if got := TranscriptBytesSince(nil, 0, prev, 0); got != bytes {
		t.Fatal("no previous snapshot")
	}
	for range 500 {
		keep := rng.IntN(len(prev.Messages) + 1)
		next := &Transcript{Meta: prev.Meta, Messages: make([]Message, keep, keep+rng.IntN(8))}
		copy(next.Messages, prev.Messages)
		for range rng.IntN(5) {
			next.Messages = append(next.Messages, msg())
		}
		if rng.IntN(4) == 0 {
			next.Meta.Title = text()
		}
		// Any index up to the first change is a valid conservative FirstChanged.
		from := rng.IntN(keep + 1)
		bytes = TranscriptBytesSince(prev, bytes, next, from)
		if want := TranscriptBytes(next); bytes != want {
			t.Fatalf("estimate %d, recompute %d", bytes, want)
		}
		prev = next
	}
}
