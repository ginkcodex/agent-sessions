package manage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
)

func TestOpenCodeRenameSubprocess(t *testing.T) {
	if os.Getenv("AS_OPENCODE_HELPER") != "1" {
		return
	}
	mode := os.Getenv("AS_OPENCODE_MODE")
	if mode == "exit" {
		os.Exit(12)
	}
	if mode == "hang" {
		for {
			time.Sleep(time.Hour)
		}
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 || !reflect.DeepEqual(args[1:], []string{"serve", "--hostname", "127.0.0.1", "--port", "0"}) {
		os.Exit(13)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		os.Exit(14)
	}
	base := "http://" + listener.Addr().String()
	record := struct {
		Base      string
		Env       []string
		Directory string
	}{Base: base, Env: os.Environ()}
	record.Directory, _ = os.Getwd()
	data, _ := json.Marshal(record)
	// Write the trace before announcing the address: the caller may kill the
	// process group as soon as it reads (and rejects) the announcement.
	_ = os.WriteFile(os.Getenv("AS_OPENCODE_TRACE"), data, 0600)
	if mode == "unsafe" {
		fmt.Println("server listening on http://0.0.0.0:1234")
	} else {
		fmt.Println("server listening on " + base)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi.json" {
			if mode == "schema" {
				fmt.Fprint(w, `{"paths":{}}`)
				return
			}
			fmt.Fprint(w, `{"paths":{"/api/session/{sessionID}":{"patch":{"operationId":"session.update"}}}}`)
			return
		}
		user, password, ok := r.BasicAuth()
		if mode != "unauth" && (!ok || user != "opencode" || password != os.Getenv("OPENCODE_SERVER_PASSWORD")) {
			w.WriteHeader(401)
			return
		}
		if r.Method != "PATCH" {
			w.WriteHeader(200)
			return
		}
		if mode == "timeout" {
			<-r.Context().Done()
			return
		}
		if mode == "failure" {
			w.WriteHeader(500)
			return
		}
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			w.WriteHeader(400)
			return
		}
		result := struct {
			Path    string
			Payload map[string]any
		}{r.URL.Path, payload}
		data, _ := json.Marshal(result)
		_ = os.WriteFile(os.Getenv("AS_OPENCODE_TRACE")+".patch", data, 0600)
		w.WriteHeader(204)
	})
	_ = http.Serve(listener, handler)
	os.Exit(0)
}

func openCodeRenameFixture(t *testing.T) (*Manager, model.SessionMeta) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "opencode")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE session_v2 (id TEXT PRIMARY KEY,title TEXT); INSERT INTO session_v2 VALUES ('ses_fixture','old'); CREATE TABLE session_message(id TEXT,session_id TEXT,type TEXT,seq INTEGER,data TEXT)`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	om, err := newOpenCodeManager(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &Manager{opencode: om}, model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentOpenCode, ID: "ses_fixture"}, SourcePath: filepath.Join(root, "opencode.db")}
}

func openCodeRenameFakeCLI(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("owned process launcher fails closed on this platform")
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "opencode")
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(bin, "'", "'\\''") + "' -test.run=^TestOpenCodeRenameSubprocess$ -- \"$@\"\n"
	if err := os.WriteFile(shim, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "trace.json")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AS_OPENCODE_HELPER", "1")
	t.Setenv("AS_OPENCODE_MODE", mode)
	t.Setenv("AS_OPENCODE_TRACE", trace)
	return trace
}

func TestRenameOpenCodeNativePayloadRootAndCleanup(t *testing.T) {
	m, meta := openCodeRenameFixture(t)
	trace := openCodeRenameFakeCLI(t, "")
	t.Setenv("OPENCODE_DB", "/must/not/use.db")
	t.Setenv("OPENCODE_PASSWORD", "inherited-secret")
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{"server":{"hostname":"0.0.0.0"}}`)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	title := "Renamed \"native\" session 日本語"
	if err := m.renameOpenCode(context.Background(), meta, title); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(trace + ".patch")
	if err != nil {
		t.Fatal(err)
	}
	var patch struct {
		Path    string
		Payload map[string]any
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatal(err)
	}
	if patch.Path != "/api/session/ses_fixture" || !reflect.DeepEqual(patch.Payload, map[string]any{"title": title}) {
		t.Fatalf("unexpected mutation: %s", data)
	}
	var record struct {
		Base      string
		Env       []string
		Directory string
	}
	data, err = os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, kv := range record.Env {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	if env["XDG_DATA_HOME"] != filepath.Dir(m.opencode.root) || env["OPENCODE_DB"] != meta.SourcePath {
		t.Fatalf("incorrect root scoping: %#v", env)
	}
	if len(env["OPENCODE_PASSWORD"]) != 64 || env["OPENCODE_PASSWORD"] != env["OPENCODE_SERVER_PASSWORD"] || env["OPENCODE_PASSWORD"] == "inherited-secret" {
		t.Fatal("server credential not isolated")
	}
	if _, ok := env["OPENCODE_CONFIG_CONTENT"]; ok {
		t.Fatal("inherited configuration leaked")
	}
	if env["OPENCODE_DISABLE_AUTOUPDATE"] != "1" || env["OPENCODE_DISABLE_MODELS_FETCH"] != "1" {
		t.Fatal("network side effects not disabled")
	}
	if record.Directory == m.opencode.root || record.Directory == meta.CWD {
		t.Fatal("server executed against user workspace")
	}
	if _, err := os.Stat(record.Directory); !os.IsNotExist(err) {
		t.Fatal("private server working directory leaked")
	}
	assertOpenCodeRenameStopped(t, record.Base)
}

func assertOpenCodeRenameStopped(t *testing.T, base string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(base, "http://"), 200*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Fatal("private opencode server leaked")
	}
}

func TestRenameOpenCodeServerFailuresAndTimeout(t *testing.T) {
	for _, mode := range []string{"exit", "hang", "unsafe", "schema", "unauth", "failure", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			m, meta := openCodeRenameFixture(t)
			trace := openCodeRenameFakeCLI(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := m.renameOpenCode(ctx, meta, "new")
			if err == nil {
				t.Fatal("expected failure")
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("server lifecycle exceeded deadline")
			}
			if mode == "hang" || mode == "timeout" {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("lost context error: %v", err)
				}
			}
			if data, readErr := os.ReadFile(trace); readErr == nil {
				var record struct {
					Base      string
					Directory string
				}
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				assertOpenCodeRenameStopped(t, record.Base)
				if _, err := os.Stat(record.Directory); !os.IsNotExist(err) {
					t.Fatal("server scratch directory leaked")
				}
			}
			if _, err := os.Stat(trace + ".patch"); !os.IsNotExist(err) {
				t.Fatal("unexpected title mutation")
			}
		})
	}
}

func TestRenameOpenCodeRefusesUnsafeRootsAndMigration(t *testing.T) {
	for _, mode := range []string{"source", "id", "missing", "unknown", "v1", "migration", "symlink", "nonstandard", "db-symlink"} {
		t.Run(mode, func(t *testing.T) {
			m, meta := openCodeRenameFixture(t)
			trace := openCodeRenameFakeCLI(t, "")
			switch mode {
			case "source":
				meta.SourcePath = filepath.Join(t.TempDir(), "opencode.db")
			case "id":
				meta.Ref.ID = "../escape"
			case "missing":
				meta.Ref.ID = "ses_missing"
			case "symlink":
				original := m.opencode.root
				target := filepath.Join(t.TempDir(), "opencode")
				if err := os.Rename(original, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, original); err != nil {
					t.Fatal(err)
				}
			case "db-symlink":
				target := filepath.Join(t.TempDir(), "db")
				if err := os.Rename(meta.SourcePath, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, meta.SourcePath); err != nil {
					t.Fatal(err)
				}
			case "nonstandard":
				m.opencode.root = filepath.Join(t.TempDir(), "custom")
			default:
				db, err := sql.Open("sqlite", meta.SourcePath)
				if err != nil {
					t.Fatal(err)
				}
				query := ""
				switch mode {
				case "unknown":
					query = `DROP TABLE session_message`
				case "v1":
					query = `DROP TABLE session_v2; CREATE TABLE session (id TEXT)`
				case "migration":
					query = `CREATE TABLE session(id TEXT); CREATE TABLE kv(key TEXT,value TEXT); INSERT INTO kv VALUES ('migration.v1-v2','{"phase":"copying"}')`
				}
				_, err = db.Exec(query)
				_ = db.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := m.renameOpenCode(context.Background(), meta, "new"); err == nil {
				t.Fatal("unsafe rename accepted")
			}
			if _, err := os.Stat(trace); !os.IsNotExist(err) {
				t.Fatal("CLI started before safety checks")
			}
		})
	}
}

func TestOpenCodeRenameHTTPRefusesRedirectAndUnknownAPI(t *testing.T) {
	var mutations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			mutations++
		}
		http.Redirect(w, r, "http://127.0.0.1:1", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err := patchOpenCodeTitle(context.Background(), client, server.URL, "secret", "ses_fixture", "new"); err == nil {
		t.Fatal("redirect accepted")
	}
	if mutations != 0 {
		t.Fatal("mutation attempted on unknown API")
	}
}

func TestOpenCodeRenameLoopbackValidation(t *testing.T) {
	for _, bad := range []string{"http://0.0.0.0:1", "http://localhost:1", "http://127.0.0.1:0", "http://127.0.0.1:65536", "https://127.0.0.1:1", "http://user@127.0.0.1:1", "http://127.0.0.1:1/path", "http://127.0.0.1:1?x=y", "http://127.0.0.1:1#x"} {
		if openCodeRenameLoopback(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
	if !openCodeRenameLoopback("http://127.0.0.1:12345") {
		t.Fatal("valid loopback rejected")
	}
}
