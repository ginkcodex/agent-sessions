package manage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// Opt-in real harness check: only a synthetic session in a temporary Codex home,
// without auth or a prompt. Never resumes a user's real session.
func TestRepairCodexSessionNativeResume(t *testing.T) {
	bin := os.Getenv("CODEX_ROLLOUT_TEST_BIN")
	if bin == "" {
		t.Skip("set CODEX_ROLLOUT_TEST_BIN for native no-inference resume verification")
	}
	roots := testRoots(t)
	dir := filepath.Join(roots.Codex, "sessions", "2026", "11", "06")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "rollout-restored-"+uuidA+".jsonl")
	raw, _ := json.Marshal(map[string]any{"ordinal": 0, "type": "session_meta", "timestamp": "2026-08-21T13:52:35Z", "payload": map[string]any{
		"id": uuidA, "timestamp": "2026-08-21T13:52:35Z", "cwd": roots.Codex, "originator": "restore_regression", "cli_version": "0.167.1", "source": "cli", "model_provider": "openai", "history_mode": "paginated", "base_instructions": map[string]string{"text": "Synthetic offline resume check."},
	}})
	raw = append(raw, '\n')
	if err := os.WriteFile(old, raw, 0600); err != nil {
		t.Fatal(err)
	}
	before := repairNativeResume(t, bin, roots.Codex, uuidA, old)
	if !bytes.Contains(before["error"], []byte("canonical rollout filename")) {
		t.Fatalf("did not reproduce filename failure: %s", before["error"])
	}
	m, _ := newTestManager(t, roots)
	ref := model.SessionRef{Agent: model.AgentCodex, ID: uuidA}
	result, err := m.RepairCodexSession(t.Context(), ref, []model.SessionMeta{{Ref: ref, SourcePath: old}})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(result.Path)
	if err != nil || !bytes.Equal(raw, saved) {
		t.Fatal("repair changed native history")
	}
	after := repairNativeResume(t, bin, roots.Codex, uuidA, "")
	if len(after["result"]) == 0 || string(after["result"]) == "null" {
		t.Fatalf("native resume failed after repair: %s", after["error"])
	}
}
func repairNativeResume(t *testing.T, bin, root, id, path string) map[string]json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "app-server")
	cmd.Dir = root
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key != "CODEX_HOME" && key != "CODEX_SQLITE_HOME" && key != "AGENT_SESSIONS_CODEX" {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "CODEX_HOME="+root, "CODEX_SQLITE_HOME="+root)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = input.Close()
		timer := time.AfterFunc(2*time.Second, func() { _ = cmd.Process.Kill() })
		_ = cmd.Wait()
		timer.Stop()
	}()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	request := func(number int, method string, params any) map[string]json.RawMessage {
		t.Helper()
		if err := json.NewEncoder(input).Encode(map[string]any{"id": number, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		for scanner.Scan() {
			var response map[string]json.RawMessage
			if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			var got int
			if json.Unmarshal(response["id"], &got) == nil && got == number {
				return response
			}
		}
		t.Fatalf("Codex app-server closed before response: %v", scanner.Err())
		return nil
	}
	initialized := request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "restore_regression", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}})
	if len(initialized["result"]) == 0 {
		t.Fatalf("initialize failed: %s", initialized["error"])
	}
	params := map[string]string{"threadId": id, "cwd": root}
	if path != "" {
		params["path"] = path
	}
	return request(2, "thread/resume", params)
}
