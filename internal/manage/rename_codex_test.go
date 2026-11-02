package manage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
)

func TestCodexRenameSubprocess(t *testing.T) {
	if os.Getenv("AS_CODEX_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	cwd, _ := os.Getwd()
	data, _ := json.Marshal(struct {
		Args []string
		Env  []string
		Dir  string
	}{args[1:], os.Environ(), cwd})
	_ = os.WriteFile(os.Getenv("AS_CODEX_TRACE"), data, 0600)
	if os.Getenv("AS_CODEX_MODE") == "hang" {
		for {
			time.Sleep(time.Hour)
		}
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		var req map[string]any
		_ = json.Unmarshal(in.Bytes(), &req)
		if id, ok := req["id"]; ok {
			if req["method"] == "thread/name/set" {
				data, _ := json.Marshal(req)
				_ = os.WriteFile(os.Getenv("AS_CODEX_TRACE")+".request", data, 0600)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": id, "result": map[string]any{}})
		}
	}
	os.Exit(0)
}

func codexRenameFixture(t *testing.T, mode string) (*Manager, model.SessionMeta, string, string) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("owned process launcher fails closed on this platform")
	}
	root := filepath.Join(t.TempDir(), "codex")
	id := "12345678-1234-1234-1234-123456789abc"
	path := filepath.Join(root, "sessions", "2026", "11", "03", "rollout-2026-11-03T00-00-00-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(path, old, old)
	cm, err := newCodexManager(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(bin, "'", "'\\''") + "' -test.run=^TestCodexRenameSubprocess$ -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "trace.json")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AS_CODEX_HELPER", "1")
	t.Setenv("AS_CODEX_MODE", mode)
	t.Setenv("AS_CODEX_TRACE", trace)
	t.Setenv("CODEX_HOME", "/must/not/use")
	t.Setenv("CODEX_SQLITE_HOME", "/must/not/use")
	return &Manager{codex: cm}, model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentCodex, ID: id}, SourcePath: path}, root, trace
}

func TestRenameCodexNativeTransportRootScoped(t *testing.T) {
	m, meta, root, trace := codexRenameFixture(t, "")
	if err := m.renameCodex(t.Context(), meta, "native title"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args []string
		Env  []string
		Dir  string
	}
	data, err := os.ReadFile(trace)
	if err != nil || json.Unmarshal(data, &got) != nil {
		t.Fatalf("trace=%s %v", data, err)
	}
	resolved, _ := filepath.EvalSymlinks(root)
	wantArgs := []string{"-c", "sqlite_home=\"" + resolved + "\"", "-c", "log_dir=\"" + filepath.Join(resolved, "log") + "\"", "app-server"}
	if !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("args=%q want %q", got.Args, wantArgs)
	}
	cwd, _ := filepath.EvalSymlinks(got.Dir)
	if cwd != resolved {
		t.Fatalf("dir=%s want %s", got.Dir, resolved)
	}
	for _, key := range []string{"CODEX_HOME", "CODEX_SQLITE_HOME"} {
		var values []string
		for _, kv := range got.Env {
			if k, v, _ := strings.Cut(kv, "="); k == key {
				values = append(values, v)
			}
		}
		if !reflect.DeepEqual(values, []string{resolved}) {
			t.Fatalf("%s=%q", key, values)
		}
	}
	data, err = os.ReadFile(trace + ".request")
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Params map[string]string `json:"params"`
	}
	if json.Unmarshal(data, &req) != nil || !reflect.DeepEqual(req.Params, map[string]string{"threadId": meta.Ref.ID, "name": "native title"}) {
		t.Fatalf("request=%s", data)
	}
}

func TestRenameCodexHangingChildCanceled(t *testing.T) {
	m, meta, _, trace := codexRenameFixture(t, "hang")
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := m.renameCodex(ctx, meta, "title"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("hanging child was not terminated promptly")
	}
	if _, err := os.Stat(trace); err != nil {
		t.Fatalf("child did not start: %v", err)
	}
}

func TestRenameCodexRejectsArchivedAndSymlinkedState(t *testing.T) {
	m, meta, root, _ := codexRenameFixture(t, "")
	archived := meta
	archived.Archived = true
	if err := m.renameCodex(t.Context(), archived, "title"); !errors.Is(err, ErrUnsupportedAction) {
		t.Fatalf("archived err=%v", err)
	}
	outside := filepath.Join(t.TempDir(), "state.sqlite")
	if err := os.WriteFile(outside, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "state_5.sqlite")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := m.renameCodex(t.Context(), meta, "title"); err == nil {
		t.Fatal("accepted symlinked native database")
	}
}
