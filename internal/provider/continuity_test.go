package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContinuity(t *testing.T) {
	for _, mode := range []string{"unchanged", "growth", "truncate", "replace", "same-size-mtime", "head", "tail", "rename", "interior-limit", "same-length-interior-limit", "missing"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.jsonl")
			original := strings.Repeat("a", 20000)
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			mark, err := MarkContinuity(t.Context(), f, 18000)
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
			want := false
			data := original
			switch mode {
			case "unchanged":
				want = true
			case "growth":
				data += "more"
				want = true
			case "truncate":
				data = data[:100]
			case "replace":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
			case "same-size-mtime":
			case "head":
				data = "b" + data[1:] + "more"
			case "tail":
				data = data[:17999] + "b" + data[18000:] + "more"
			case "interior-limit":
				data = data[:8000] + "b" + data[8001:] + "more"
				want = true
			case "same-length-interior-limit":
				data = data[:8000] + "b" + data[8001:]
				want = true
			case "rename":
				next := path + ".renamed"
				if err := os.Rename(path, next); err != nil {
					t.Fatal(err)
				}
				path = next
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "missing" && mode != "rename" && mode != "unchanged" {
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
				// No clock-resolution assumptions or sleeps for the rewrite rule.
				changed := mark.info.ModTime().Add(time.Second)
				if mode == "same-length-interior-limit" {
					changed = mark.info.ModTime()
				}
				if err := os.Chtimes(path, changed, changed); err != nil {
					t.Fatal(err)
				}
			}
			got, err := mark.Validate(t.Context(), path)
			if err != nil || got != want {
				t.Fatalf("resume=%v want=%v err=%v", got, want, err)
			}
		})
	}
}

func TestContinuityShortPrefixAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short")
	if err := os.WriteFile(path, []byte("abc\ntail"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, offset := range []int64{0, 4, 8} {
		mark, err := MarkContinuity(t.Context(), f, offset)
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := mark.ValidateFile(t.Context(), f); err != nil || !ok {
			t.Fatalf("offset=%d ok=%v err=%v", offset, ok, err)
		}
	}
	for _, offset := range []int64{-1, 9} {
		if _, err := MarkContinuity(t.Context(), f, offset); err == nil {
			t.Fatal("invalid offset accepted")
		}
	}
	mark, err := MarkContinuity(t.Context(), f, 4)
	if err != nil {
		t.Fatal(err)
	}
	// Hash only the committed prefix: completing an uncommitted tail may change it.
	if _, err := f.WriteAt([]byte("changed-tail\n"), 4); err != nil {
		t.Fatal(err)
	}
	if ok, err := mark.ValidateFile(t.Context(), f); err != nil || !ok {
		t.Fatalf("tail completion: %v %v", ok, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mark.Validate(ctx, path); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := MarkContinuity(ctx, f, 0); err != context.Canceled {
		t.Fatal(err)
	}
	if ok, err := (ContinuityMark{}).Validate(t.Context(), path); ok || err != nil {
		t.Fatal("zero mark resumed", err)
	}
}
