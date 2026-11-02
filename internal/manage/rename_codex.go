package manage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// Use the native protocol rather than writing threads.title or session_index:
// https://github.com/openai/codex/blob/main/codex-rs/app-server/src/request_processors/thread_processor.rs
// thread_set_name_response_inner updates native metadata (include_archived=false).
// https://github.com/openai/codex/blob/main/codex-rs/app-server/tests/suite/v2/thread_name_persistence.rs
func (m *Manager) renameCodex(ctx context.Context, meta model.SessionMeta, title string) error {
	cm := m.codex
	if cm == nil || !uuidPattern.MatchString(meta.Ref.ID) || meta.Archived {
		return fmt.Errorf("%w: Codex rename requires a non-archived UUID session", ErrUnsupportedAction)
	}
	targets, err := cm.derivePaths(ctx, meta)
	if err != nil {
		return err
	}
	if len(targets) != 1 {
		return errors.New("rename: ambiguous Codex rollout")
	}
	mode, err := cm.ps.classify(targets[0])
	if err != nil {
		return safePathError(err)
	}
	if !mode.IsRegular() || filepath.Clean(meta.SourcePath) != targets[0] {
		return ErrPathOutsideRoot
	}
	root, err := cm.ps.resolvedRoot()
	if err != nil {
		return safePathError(err)
	}
	// Native SQLite also opens sidecars. Refuse symlinked DBs/sidecars and
	// index files rather than allowing the CLI to follow them outside its root.
	entries, err := os.ReadDir(root)
	if err != nil {
		return errors.New("rename: cannot inspect Codex root")
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "state_") || strings.HasPrefix(name, "logs_") || strings.HasPrefix(name, "session_index.jsonl") {
			if _, err := cm.ps.classify(filepath.Join(cm.root, name)); err != nil {
				return safePathError(err)
			}
		}
	}
	bin, err := resolveCLI("codex")
	if err != nil {
		return fmt.Errorf("%w: Codex CLI unavailable", ErrUnsupportedAction)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("%w: native Codex rename requires process-tree lifecycle support", ErrUnsupportedAction)
	}
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(key, "CODEX_HOME") && !strings.EqualFold(key, "CODEX_SQLITE_HOME") {
			env = append(env, kv)
		}
	}
	env = append(env, "CODEX_HOME="+root, "CODEX_SQLITE_HOME="+root)
	process, streams, err := codediagnostics.HostSessionLaunch(codediagnostics.Invocation{
		Executable: bin, Args: []string{"-c", "sqlite_home=" + strconv.Quote(root), "-c", "log_dir=" + strconv.Quote(filepath.Join(root, "log")), "app-server"}, Directory: root,
		Env: withPathPrefix(env, filepath.Dir(bin)),
	})
	if err != nil {
		return errors.New("rename: cannot start Codex app-server")
	}
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = process.Terminate()
			_ = process.Close()
		case <-finished:
		}
	}()
	go func() { _, _ = io.Copy(io.Discard, streams.Stderr) }()
	defer func() {
		close(finished)
		streams.Stdin.Close()
		_ = process.Terminate()
		_ = process.Close()
		_, _ = process.Wait()
	}()
	if err := codexRenameProtocol(streams.Stdin, streams.Stdout, meta.Ref.ID, title); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func codexRenameProtocol(input io.Writer, output io.Reader, id, title string) error {
	enc := json.NewEncoder(input)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	request := func(n int, method string, params any) error {
		if err := enc.Encode(map[string]any{"id": n, "method": method, "params": params}); err != nil {
			return errors.New("rename: Codex request failed")
		}
		for scanner.Scan() {
			var response struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &response) != nil {
				return errors.New("rename: invalid Codex response")
			}
			if response.ID == nil || *response.ID != n {
				continue
			}
			if len(response.Error) > 0 && string(response.Error) != "null" {
				return fmt.Errorf("%w: Codex app-server rejected %s", ErrUnsupportedAction, method)
			}
			if len(response.Result) == 0 {
				return errors.New("rename: missing Codex response")
			}
			return nil
		}
		return errors.New("rename: Codex app-server closed before confirmation")
	}
	if err := request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "agent-sessions", "version": "1"}, "capabilities": map[string]any{}}); err != nil {
		return err
	}
	if err := enc.Encode(map[string]any{"method": "initialized"}); err != nil {
		return errors.New("rename: Codex initialization failed")
	}
	return request(2, "thread/name/set", map[string]string{"threadId": id, "name": title})
}
