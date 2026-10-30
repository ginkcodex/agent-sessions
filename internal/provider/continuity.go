package provider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

const continuityWindow = 4096

// ContinuityMark validates an append-only prefix without rereading the full file.
// It detects identity, same-size mtime, head and committed-tail changes. Interior
// rewrites preserving both windows can go undetected; this is not a content proof.
// Marks hold only file metadata and two hashes, never open descriptors.
type ContinuityMark struct {
	path       string
	info       os.FileInfo
	offset     int64
	headLen    int64
	head, tail [sha256.Size]byte
}

// MarkContinuity records the prefix through offset on an already opened source.
// The caller must pass the last complete-line boundary, not a best-effort tail.
func MarkContinuity(ctx context.Context, f *os.File, offset int64) (ContinuityMark, error) {
	if err := ctx.Err(); err != nil {
		return ContinuityMark{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return ContinuityMark{}, err
	}
	if offset < 0 || offset > info.Size() {
		return ContinuityMark{}, fmt.Errorf("provider: invalid committed offset %d", offset)
	}
	m := ContinuityMark{path: f.Name(), info: info, offset: offset, headLen: min(offset, continuityWindow)}
	m.head, err = continuityHash(ctx, f, 0, m.headLen)
	if err != nil {
		return ContinuityMark{}, err
	}
	m.tail, err = continuityHash(ctx, f, max(0, offset-continuityWindow), min(offset, continuityWindow))
	if err != nil {
		return ContinuityMark{}, err
	}
	return m, nil
}

// Validate returns false when continuity is lost, including a missing source.
// I/O and cancellation errors are returned rather than interpreted as a match.
// Use ValidateFile after opening when the validated descriptor will be parsed.
func (m ContinuityMark) Validate(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if m.info == nil || path != m.path {
		return false, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	return m.ValidateFile(ctx, f)
}

// ValidateFile validates the descriptor that the caller will continue reading.
func (m ContinuityMark) ValidateFile(ctx context.Context, f *os.File) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if m.info == nil || f.Name() != m.path {
		return false, nil
	}
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if !os.SameFile(m.info, info) || info.Size() < m.offset || (info.Size() == m.info.Size() && !info.ModTime().Equal(m.info.ModTime())) {
		return false, nil
	}
	head, err := continuityHash(ctx, f, 0, m.headLen)
	if err != nil {
		return false, err
	}
	tail, err := continuityHash(ctx, f, max(0, m.offset-continuityWindow), min(m.offset, continuityWindow))
	if err != nil {
		return false, err
	}
	return head == m.head && tail == m.tail, nil
}

func continuityHash(ctx context.Context, f *os.File, offset, size int64) ([sha256.Size]byte, error) {
	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	var buf [continuityWindow]byte
	n, err := f.ReadAt(buf[:size], offset)
	if work := LoadWorkFromContext(ctx); work != nil {
		work.ContinuityBytes.Add(int64(n))
	}
	if err != nil && !(err == io.EOF && size == 0) {
		return [sha256.Size]byte{}, err
	}
	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(buf[:size]), nil
}
