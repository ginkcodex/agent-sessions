package model

import "unsafe"

// TranscriptBytes estimates the in-memory structural size of t, or zero for nil.
// It is deterministic for a given Go layout and input, not a heap measurement.
// Fixed storage uses unsafe.Sizeof (including headers, pointers and padding);
// strings contribute their byte lengths and slices their backing-array capacity.
// Nested payloads are visited only in the live slice length. Consequently stale
// references in unused capacity are not counted. Sizes can differ by architecture.
//
// Each occurrence is counted independently: shared strings, backing arrays and
// pointees are not deduplicated, and substrings count only their visible bytes.
// Allocator rounding/metadata, GC overhead, external attachment contents and
// shared time.Time location internals are excluded. This is an approximate cache
// accounting weight, not an exact measure of uniquely retained memory.
func TranscriptBytes(t *Transcript) int64 {
	if t == nil {
		return 0
	}
	n := int64(unsafe.Sizeof(*t)) + sessionMetaDataBytes(&t.Meta)
	n += int64(cap(t.Messages)) * int64(unsafe.Sizeof(Message{}))
	for i := range t.Messages {
		n += messageDataBytes(&t.Messages[i])
	}
	return n
}

// TranscriptBytesSince returns TranscriptBytes(next) from prevBytes, the
// estimate of prev, when messages before from are unchanged from prev to next.
// Only the changed suffix and metadata are revisited.
func TranscriptBytesSince(prev *Transcript, prevBytes int64, next *Transcript, from int) int64 {
	if prev == nil || next == nil {
		return TranscriptBytes(next)
	}
	from = max(0, min(from, len(prev.Messages), len(next.Messages)))
	slot := int64(unsafe.Sizeof(Message{}))
	n := prevBytes - sessionMetaDataBytes(&prev.Meta) - int64(cap(prev.Messages))*slot
	n += sessionMetaDataBytes(&next.Meta) + int64(cap(next.Messages))*slot
	for i := from; i < len(prev.Messages); i++ {
		n -= messageDataBytes(&prev.Messages[i])
	}
	for i := from; i < len(next.Messages); i++ {
		n += messageDataBytes(&next.Messages[i])
	}
	return n
}

// MessageBytes estimates a message's fixed storage and nested data, using the
// same accounting rules as TranscriptBytes. It returns zero for nil. The fixed
// message storage is included even when the message lives in a slice; callers
// doing incremental accounting must account for slice capacity changes separately.
func MessageBytes(m *Message) int64 {
	if m == nil {
		return 0
	}
	return int64(unsafe.Sizeof(*m)) + messageDataBytes(m)
}

func sessionRefDataBytes(r *SessionRef) int64 {
	return int64(len(r.Agent)) + int64(len(r.ID))
}

func sessionMetaDataBytes(m *SessionMeta) int64 {
	return sessionRefDataBytes(&m.Ref) +
		int64(len(m.ParentID)) + int64(len(m.ParentToolCallID)) +
		int64(len(m.SourcePath)) + int64(len(m.CWD)) + int64(len(m.RepoRoot)) +
		int64(len(m.GitBranch)) + int64(len(m.Title)) + int64(len(m.FirstPrompt)) +
		int64(len(m.Model)) + int64(len(m.AgentName)) + int64(len(m.LiveStatus)) +
		int64(len(m.AgentVersion)) + int64(len(m.Turn))
}

// messageDataBytes excludes the inline Message storage already counted in a
// transcript's Messages backing array.
func messageDataBytes(m *Message) int64 {
	n := int64(len(m.ID)) + int64(len(m.Role)) + int64(len(m.Model))
	n += int64(cap(m.Parts)) * int64(unsafe.Sizeof(Part{}))
	for i := range m.Parts {
		p := &m.Parts[i]
		n += int64(len(p.Kind)) + int64(len(p.Text))
		n += int64(cap(p.Files)) * int64(unsafe.Sizeof(""))
		for _, file := range p.Files {
			n += int64(len(file))
		}
		if tool := p.Tool; tool != nil {
			n += int64(unsafe.Sizeof(*tool)) + int64(len(tool.ID)) +
				int64(len(tool.Name)) + int64(cap(tool.Input)) +
				int64(len(tool.Output)) + int64(len(tool.OutputRef)) + int64(len(tool.Status))
			if tool.Child != nil {
				n += int64(unsafe.Sizeof(*tool.Child)) + sessionRefDataBytes(tool.Child)
			}
		}
		if file := p.File; file != nil {
			n += int64(unsafe.Sizeof(*file)) + int64(len(file.Name)) +
				int64(len(file.Mime)) + int64(len(file.Ref))
		}
	}
	return n
}
