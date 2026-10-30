package transcriptbuild

import (
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// Unshared estimates storage that committed retains beyond published, a
// snapshot forked from it: the message array and nested content the fork
// replaced. Shared storage is detected by identity and counted once.
func Unshared(committed, published []model.Message) int64 {
	if len(committed) == 0 || &committed[0] == &published[0] {
		return 0
	}
	n := int64(len(committed)) * int64(unsafe.Sizeof(model.Message{}))
	for i := range committed {
		if !sameParts(committed[i], published[i]) {
			n += retainedParts(committed[i], published[i])
		}
	}
	return n
}

func sameParts(a, b model.Message) bool {
	if len(a.Parts) == 0 || len(b.Parts) == 0 {
		return len(a.Parts) == len(b.Parts)
	}
	return &a.Parts[0] == &b.Parts[0]
}

func retainedParts(a, b model.Message) int64 {
	n := int64(cap(a.Parts)) * int64(unsafe.Sizeof(model.Part{}))
	for i, p := range a.Parts {
		var q model.Part
		if i < len(b.Parts) {
			q = b.Parts[i]
		}
		if unsafe.StringData(p.Text) != unsafe.StringData(q.Text) {
			n += int64(len(p.Text))
		}
		if p.Tool != nil && p.Tool != q.Tool {
			n += int64(unsafe.Sizeof(*p.Tool))
			if q.Tool == nil {
				n += model.MessageBytes(&model.Message{Parts: []model.Part{p}})
				continue
			}
			if unsafe.SliceData(p.Tool.Input) != unsafe.SliceData(q.Tool.Input) {
				n += int64(cap(p.Tool.Input))
			}
			if unsafe.StringData(p.Tool.Output) != unsafe.StringData(q.Tool.Output) {
				n += int64(len(p.Tool.Output))
			}
			if p.Tool.Child != nil && p.Tool.Child != q.Tool.Child {
				n += int64(unsafe.Sizeof(*p.Tool.Child)) + int64(len(p.Tool.Child.ID))
			}
		}
		if p.File != nil && p.File != q.File {
			n += int64(unsafe.Sizeof(*p.File))
		}
		if unsafe.SliceData(p.Files) != unsafe.SliceData(q.Files) {
			n += int64(cap(p.Files)) * 16
		}
	}
	return n
}
