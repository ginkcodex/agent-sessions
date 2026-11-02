package manage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider/sqliteread"
)

// renameOpenCode starts a private foreground server, never a managed service.
// Caller holds m.mu. Only OpenCode writes the title (and its native event log).
func (m *Manager) renameOpenCode(ctx context.Context, meta model.SessionMeta, title string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	om := m.opencode
	if om == nil || meta.Ref.Agent != model.AgentOpenCode {
		return ErrUnsupportedAction
	}
	if !isSafeID(meta.Ref.ID) || strings.HasPrefix(meta.Ref.ID, "-") || meta.Ref.ID == "." || meta.Ref.ID == ".." {
		return errors.New("invalid opencode session id")
	}
	if !om.canTargetRoot() {
		return errors.New("opencode root is non-standard; refusing to execute CLI")
	}
	if _, err := om.ps.resolvedRoot(); err != nil {
		return err
	}
	if !filepath.IsAbs(meta.SourcePath) || filepath.Clean(meta.SourcePath) != om.dbPath() {
		return errors.New("opencode session database does not match configured root")
	}
	if err := om.renameSchema(ctx, meta.Ref.ID); err != nil {
		return err
	}
	exe, err := resolveCLI("opencode")
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp("", "agent-sessions-opencode-rename-*")
	if err != nil {
		return errors.New("cannot prepare private opencode server")
	}
	defer os.RemoveAll(scratch)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return errors.New("cannot create opencode server credential")
	}
	secret := hex.EncodeToString(secretBytes)
	env := openCodeRenameEnv(om.root, scratch, secret, os.Environ())
	env = withPathPrefix(env, filepath.Dir(exe))
	proc, streams, err := codediagnostics.HostSessionLaunch(codediagnostics.Invocation{
		Executable: exe, Args: []string{"serve", "--hostname", "127.0.0.1", "--port", "0"},
		Directory: scratch, Env: env,
	})
	if err != nil {
		return errors.New("cannot start private opencode server (process-tree ownership is required on this platform)")
	}
	done := make(chan struct{})
	go func() { _, _ = proc.Wait(); close(done) }()
	defer func() {
		// Kill the owned process group even if its leader has already exited: a
		// wrapper must not leave a server child behind. Close unblocks all readers.
		_ = proc.Terminate()
		_ = streams.Stdin.Close()
		_ = proc.Close()
		<-done
	}()
	go func() { _, _ = io.Copy(io.Discard, streams.Stderr) }()
	address := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(streams.Stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			for _, prefix := range []string{"server listening on ", "opencode server listening on "} {
				if candidate, ok := strings.CutPrefix(line, prefix); ok {
					select {
					case address <- candidate:
					default:
					}
				}
			}
		}
	}()
	var base string
	select {
	case base = <-address:
	case <-done:
		return errors.New("private opencode server exited before startup")
	case <-ctx.Done():
		return ctx.Err()
	}
	if !openCodeRenameLoopback(base) {
		return errors.New("private opencode server reported an unsafe address")
	}
	transport := &http.Transport{
		Proxy:             nil,
		DialContext:       (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return patchOpenCodeTitle(ctx, client, base, secret, meta.Ref.ID, title)
}

// Do not let serve perform a v1-to-v2 migration as a side effect of rename.
// v1-only/legacy roots must be upgraded by OpenCode itself first. Read-only
// schema checks deliberately fail closed on unknown or incomplete layouts.
func (om *opencodeManager) renameSchema(ctx context.Context, id string) error {
	fi, err := os.Lstat(om.dbPath())
	if err != nil || !fi.Mode().IsRegular() {
		return errors.New("opencode database is unavailable or unsafe")
	}
	db, err := sqliteread.Open(ctx, om.dbPath())
	if err != nil {
		return errors.New("cannot open opencode database read-only")
	}
	defer db.Close()
	for _, table := range []string{"session_v2", "session_message"} {
		exists, err := sqliteread.HasTable(ctx, db, table)
		if err != nil || !exists {
			return errors.New("OpenCode rename requires a recognized, fully migrated v2 database")
		}
	}
	hasV1, err := sqliteread.HasTable(ctx, db, "session")
	if err != nil {
		return errors.New("cannot inspect opencode schema")
	}
	if hasV1 {
		complete, err := v1MigrationCompleted(ctx, db)
		if err != nil || !complete {
			return errors.New("OpenCode migration is unfinished; open OpenCode once before renaming")
		}
	}
	var found string
	if err := db.QueryRowContext(ctx, `SELECT id FROM session_v2 WHERE id=?`, id).Scan(&found); err != nil {
		return errors.New("session not found in recognized opencode v2 schema")
	}
	columns, err := db.QueryContext(ctx, `SELECT id, title FROM session_v2 LIMIT 0`)
	if err != nil {
		return errors.New("unrecognized opencode session schema")
	}
	_ = columns.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, session_id, type, seq, data FROM session_message LIMIT 0`)
	if err != nil {
		return errors.New("unrecognized opencode message schema")
	}
	return rows.Close()
}

func openCodeRenameEnv(root, scratch, secret string, inherited []string) []string {
	env := make([]string, 0, len(inherited)+16)
	for _, kv := range inherited {
		key, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(key)
		// In particular do not inherit OPENCODE_DB, remote/service selectors,
		// configuration content, or a higher-precedence OPENCODE_PASSWORD.
		if strings.HasPrefix(upper, "OPENCODE_") || strings.HasPrefix(upper, "XDG_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"XDG_DATA_HOME="+filepath.Dir(root),
		"XDG_CONFIG_HOME="+filepath.Join(scratch, "config"),
		"XDG_CACHE_HOME="+filepath.Join(scratch, "cache"),
		"XDG_STATE_HOME="+filepath.Join(scratch, "state"),
		"OPENCODE_CONFIG_DIR="+filepath.Join(scratch, "config", "opencode"),
		"OPENCODE_DB="+filepath.Join(root, "opencode.db"),
		"OPENCODE_DISABLE_CHANNEL_DB=1",
		"OPENCODE_PASSWORD="+secret,
		"OPENCODE_SERVER_PASSWORD="+secret,
		"OPENCODE_SERVER_USERNAME=opencode",
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_CONFIG_PROJECT_DISABLE=1",
		"OPENCODE_DISABLE_CLAUDE_CODE=1",
	)
}

func openCodeRenameLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && port > 0 && port <= 65535
}

func patchOpenCodeTitle(ctx context.Context, client *http.Client, base, secret, id, title string) error {
	// Current v2 advertises /api/session/{sessionID}; older releases documented
	// /session/{sessionID}. Trust neither a guessed path nor a redirect.
	request := func(method, path string, body []byte, auth bool) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if auth {
			req.SetBasicAuth("opencode", secret)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return client.Do(req)
	}
	resp, err := request(http.MethodGet, "/openapi.json", nil, true)
	if err != nil {
		return fmt.Errorf("cannot inspect private opencode API: %w", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&spec)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || decodeErr != nil {
		return errors.New("private opencode server has an unsupported API")
	}
	route := ""
	for _, path := range []string{"/api/session/{sessionID}", "/session/{sessionID}"} {
		if spec.Paths[path]["patch"].OperationID == "session.update" {
			route = strings.ReplaceAll(path, "{sessionID}", url.PathEscape(id))
			break
		}
	}
	if route == "" {
		return errors.New("private opencode server does not advertise native session rename")
	}
	// Prove the actual session endpoint is protected before sending a title.
	resp, err = request(http.MethodGet, route, nil, false)
	if err != nil {
		return fmt.Errorf("cannot verify private opencode authentication: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return errors.New("private opencode session API is not authenticated; refusing rename")
	}
	body, _ := json.Marshal(struct {
		Title string `json:"title"`
	}{title})
	resp, err = request(http.MethodPatch, route, body, true)
	if err != nil {
		return fmt.Errorf("opencode rename request failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("opencode rename failed (HTTP %d)", resp.StatusCode)
	}
	return nil
}
