package manage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

func TestRenameClaudeNativeAppend(t *testing.T) {
	root := t.TempDir()
	id := "12345678-1234-1234-1234-123456789abc"
	path := filepath.Join(root, "projects", "fixture", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := "{\"type\":\"user\",\"message\":{\"content\":\"synthetic\"}}\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(paths.Roots{Claude: root}, filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	meta := model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentClaude, ID: id}, SourcePath: path}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.RenameSession(context.Background(), meta.Ref, "  native title  ", []model.SessionMeta{meta}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte(original)) {
		t.Fatal("transcript rewritten")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 13 {
		t.Fatalf("lines=%d", len(lines))
	}
	for _, line := range lines[1:] {
		var v map[string]string
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatal(err)
		}
		if v["type"] != "custom-title" || v["customTitle"] != "native title" || v["sessionId"] != id || len(v) != 3 {
			t.Fatalf("record=%v", v)
		}
	}
	for _, title := range []string{"", " \t ", "bad\nname", string([]byte{0xff}), strings.Repeat("a", 1001)} {
		if err := m.RenameSession(context.Background(), meta.Ref, title, []model.SessionMeta{meta}); err == nil {
			t.Fatalf("accepted %q", title)
		}
	}
	bad := meta
	bad.SourcePath = filepath.Join(root, id+".jsonl")
	if err := m.RenameSession(context.Background(), bad.Ref, "x", []model.SessionMeta{bad}); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("path err=%v", err)
	}
	child := meta
	child.ParentID = id
	if err := m.RenameSession(context.Background(), child.Ref, "x", []model.SessionMeta{child}); !errors.Is(err, ErrUnsupportedAction) {
		t.Fatalf("child err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.RenameSession(ctx, meta.Ref, "x", []model.SessionMeta{meta}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRenameClaudeRefusesSymlinkAndEmpty(t *testing.T) {
	root := t.TempDir()
	id := "12345678-1234-1234-1234-123456789abc"
	path := filepath.Join(root, "projects", "fixture", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, []byte("synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(paths.Roots{Claude: root}, filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	meta := model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentClaude, ID: id}, SourcePath: path}
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := m.RenameSession(t.Context(), meta.Ref, "title", []model.SessionMeta{meta}); err == nil {
		t.Fatal("accepted symlink")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "synthetic\n" {
		t.Fatal("outside file modified")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.RenameSession(t.Context(), meta.Ref, "title", []model.SessionMeta{meta}); err == nil {
		t.Fatal("accepted empty stub")
	}
	if err := m.RenameSession(t.Context(), model.SessionRef{Agent: model.AgentClaude, ID: "unknown"}, "title", []model.SessionMeta{meta}); err == nil {
		t.Fatal("accepted unknown ref")
	}
}

func TestRenameClaudeRefusesIncompleteTrailingRecord(t *testing.T) {
	root := t.TempDir()
	id := "12345678-1234-1234-1234-123456789abc"
	path := filepath.Join(root, "projects", "fixture", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	partial := "{\"type\":\"user\"}\n{\"type\":\"assist"
	if err := os.WriteFile(path, []byte(partial), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(paths.Roots{Claude: root}, filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	meta := model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentClaude, ID: id}, SourcePath: path}
	if err := m.RenameSession(t.Context(), meta.Ref, "title", []model.SessionMeta{meta}); err == nil {
		t.Fatal("appended after a partial record")
	}
	if data, _ := os.ReadFile(path); string(data) != partial {
		t.Fatal("partial transcript modified")
	}
}

func TestCodexRenameProtocol(t *testing.T) {
	var input bytes.Buffer
	output := strings.NewReader("{\"id\":1,\"result\":{}}\n{\"method\":\"notice\"}\n{\"id\":2,\"result\":{}}\n")
	if err := codexRenameProtocol(&input, output, "fixture-id", "native name"); err != nil {
		t.Fatal(err)
	}
	var requests []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(input.String()), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, v)
	}
	if len(requests) != 3 || requests[0]["method"] != "initialize" || requests[1]["method"] != "initialized" || requests[2]["method"] != "thread/name/set" {
		t.Fatalf("requests=%v", requests)
	}
	params := requests[2]["params"].(map[string]any)
	if params["threadId"] != "fixture-id" || params["name"] != "native name" {
		t.Fatal(params)
	}
	for _, output := range []string{"", "bad\n", "{\"id\":1,\"error\":{\"code\":-32601}}\n", "{\"id\":1,\"result\":{}}\n{\"id\":2,\"error\":{\"code\":-32601}}\n"} {
		if err := codexRenameProtocol(&bytes.Buffer{}, strings.NewReader(output), "id", "name"); err == nil {
			t.Fatal("accepted failed protocol")
		}
	}
}
