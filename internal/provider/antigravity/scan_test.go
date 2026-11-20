package antigravity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // Register the SQLite driver for fixture DBs.

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/testutil/platform"
)

const (
	parentID = "2b9a4c81-215f-47af-9395-99d2b08be416"
	childID  = "0ce5ddc2-779f-452d-9c81-281712caf621"
)

// schema is agy 1.5.12's conversation_summaries table, as found on disk.
const schema = "CREATE TABLE `conversation_summaries` (`conversation_id` text,`title` text NOT NULL DEFAULT \"\",`preview` text NOT NULL DEFAULT \"\",`step_count` integer NOT NULL DEFAULT 0,`last_modified_time` datetime NOT NULL,`workspace_uris` text NOT NULL,`status` text NOT NULL DEFAULT \"\",`source` text NOT NULL DEFAULT \"\",`project_id` text NOT NULL DEFAULT \"\",`agent_name` text NOT NULL DEFAULT \"\",`parent_conversation_id` text NOT NULL DEFAULT \"\",`nesting_depth` integer NOT NULL DEFAULT 0,`battle_id` text NOT NULL DEFAULT \"\",`winning_conversation_id` text NOT NULL DEFAULT \"\",`not_fully_idle` numeric NOT NULL DEFAULT false,`killed` numeric NOT NULL DEFAULT false,`last_user_input_time` datetime NOT NULL,`last_user_input_step_index` integer NOT NULL DEFAULT -1,`app_data_dir` text NOT NULL DEFAULT \"\",`raw_summary` blob,`group_id` text NOT NULL DEFAULT \"\",PRIMARY KEY (`conversation_id`));" +
	"CREATE TABLE conversation_parents (conversation_id TEXT PRIMARY KEY, parent_id TEXT NOT NULL) WITHOUT ROWID;"

type fixtureRow struct {
	id, title, preview, status, uris, agent, parent string
	modified                                        time.Time
	notIdle, killed                                 bool
}

func writeDB(t *testing.T, root string, rows ...fixtureRow) {
	t.Helper()
	path := filepath.Join(root, summariesDB)
	_ = os.Remove(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("PRAGMA journal_mode=WAL;" + schema); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		stamp := r.modified.UTC().Format("2006-01-02 15:04:05.999999999-07:00")
		_, err := db.Exec(`INSERT INTO conversation_summaries (conversation_id, title, preview, last_modified_time,
			workspace_uris, status, agent_name, parent_conversation_id, not_fully_idle, killed, last_user_input_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '0001-01-01 00:00:00+00:00')`,
			r.id, r.title, r.preview, stamp, r.uris, r.status, r.agent, r.parent, r.notIdle, r.killed)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func transcriptPath(root, id string) string {
	return filepath.Join(root, "brain", id, ".system_generated", "logs", "transcript_full.jsonl")
}

func writeTranscript(t *testing.T, root, id string, steps ...string) string {
	t.Helper()
	path := transcriptPath(root, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(steps, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendSteps(t *testing.T, path string, steps ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strings.Join(steps, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func at(step int) string {
	return time.Date(2026, 11, 19, 4, 0, step, 0, time.UTC).Format(time.RFC3339)
}

func userStep(step int, text string) string {
	return mustJSON(map[string]any{
		"step_index": step, "source": "USER_EXPLICIT", "type": stepUserInput, "status": "DONE", "created_at": at(step),
		"content": "<USER_REQUEST>\n" + text + "\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: 2026-11-19T11:00:00+07:00.\n</ADDITIONAL_METADATA>",
	})
}

func plannerStep(step int, reply string, calls ...string) string {
	r := map[string]any{
		"step_index": step, "source": "MODEL", "type": stepPlanner, "status": "DONE", "created_at": at(step),
		"model": "gemini-4-neon-high", "input_tokens": 100, "cache_read_tokens": 10, "output_tokens": 5, "thinking": "hmm",
	}
	if reply != "" {
		r["content"] = reply
	}
	var tc []map[string]any
	for _, name := range calls {
		tc = append(tc, map[string]any{"name": name, "args": map[string]string{"Path": "/x"}})
	}
	if tc != nil {
		r["tool_calls"] = tc
	}
	return mustJSON(r)
}

func genericStep(step int) string {
	return mustJSON(map[string]any{
		"step_index": step, "source": "MODEL", "type": stepGeneric, "status": "DONE", "created_at": at(step),
		"content": "Created At: 2026-11-19T11:00:00+07:00\nok",
	})
}

func systemStep(step int, sender, text string) string {
	return mustJSON(map[string]any{
		"step_index": step, "source": "SYSTEM", "type": stepSystemMessage, "status": "DONE", "created_at": at(step),
		"content": "The following is a <SYSTEM_MESSAGE> not actually sent by the user. It is provided by the system as important information to pay attention to.\n\n<SYSTEM_MESSAGE>\n[Message] timestamp=" + at(step) + " sender=" + sender + " priority=MESSAGE_PRIORITY_HIGH content=" + text + "\n</SYSTEM_MESSAGE>",
	})
}

func newTestProvider(t *testing.T, root string, now time.Time) *Provider {
	t.Helper()
	p := New(root, nil)
	p.now = func() time.Time { return now }
	return p
}

func scan(t *testing.T, p *Provider, prev provider.ScanState) provider.ScanResult {
	t.Helper()
	res, err := p.Scan(context.Background(), prev)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func byID(res provider.ScanResult) map[string]model.SessionMeta {
	out := map[string]model.SessionMeta{}
	for _, m := range res.Changed {
		out[m.Ref.ID] = m
	}
	return out
}

func TestScanParentAndSubagent(t *testing.T) {
	root := platform.TempDir(t)
	ws := filepath.Join(root, "my ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := writeTranscript(t, root, parentID,
		userStep(0, "Good morning"),
		plannerStep(1, "", "read_file", "invoke_subagent"),
		genericStep(2), genericStep(3),
		systemStep(4, childID, "report"),
		plannerStep(5, "Done."),
	)
	writeTranscript(t, root, childID,
		systemStep(0, parentID, "Analyze the\narchitecture"),
		plannerStep(1, "Report."),
	)
	uri := "file://" + filepath.ToSlash(strings.ReplaceAll(ws, " ", "%20"))
	if !strings.HasPrefix(uri, "file:///") {
		uri = "file:///" + strings.TrimPrefix(uri, "file://")
	}
	now := time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC)
	writeDB(t, root,
		fixtureRow{id: parentID, title: "Casual Morning Greeting", status: "CASCADE_RUN_STATUS_IDLE", uris: `["` + uri + `","file:///tmp"]`, modified: now},
		fixtureRow{id: childID, status: "CASCADE_RUN_STATUS_IDLE", uris: `["` + uri + `"]`, agent: "research", parent: parentID, modified: now},
	)
	res := scan(t, newTestProvider(t, root, now), provider.ScanState{})
	if len(res.Diag.Warnings) != 0 || res.Diag.ParseErrors != 0 || len(res.Diag.UnknownTypes) != 0 {
		t.Fatalf("diagnostics: %+v", res.Diag)
	}
	got := byID(res)
	parent, child := got[parentID], got[childID]
	if parent.Title != "Casual Morning Greeting" || parent.FirstPrompt != "Good morning" || parent.ParentID != "" {
		t.Errorf("parent title/prompt/parent = %q %q %q", parent.Title, parent.FirstPrompt, parent.ParentID)
	}
	if parent.CWD != ws || parent.SourcePath != parentPath {
		t.Errorf("parent cwd/source = %q %q, want %q %q", parent.CWD, parent.SourcePath, ws, parentPath)
	}
	if want := (model.MessageCounts{User: 1, Assistant: 2, ToolCalls: 2}); parent.Counts != want {
		t.Errorf("parent counts = %+v, want %+v", parent.Counts, want)
	}
	if want := (model.TokenUsage{Input: 200, Output: 10, CacheRead: 20}); parent.Tokens != want {
		t.Errorf("parent tokens = %+v, want %+v", parent.Tokens, want)
	}
	if parent.Model != "gemini-4-neon-high" || !parent.CreatedAt.Equal(time.Date(2026, 11, 19, 4, 0, 0, 0, time.UTC)) ||
		!parent.UpdatedAt.Equal(time.Date(2026, 11, 19, 4, 0, 5, 0, time.UTC)) || parent.Turn != model.TurnDone {
		t.Errorf("parent model/created/updated/turn = %q %v %v %q", parent.Model, parent.CreatedAt, parent.UpdatedAt, parent.Turn)
	}
	if child.ParentID != parentID || child.AgentName != "research" {
		t.Errorf("child parent/agent = %q %q", child.ParentID, child.AgentName)
	}
	if child.Title != "Analyze the architecture" || child.FirstPrompt != "Analyze the architecture" || child.Counts.User != 1 {
		t.Errorf("child title/prompt/users = %q %q %d", child.Title, child.FirstPrompt, child.Counts.User)
	}
}

func TestScanParentFromParentsTable(t *testing.T) {
	root := platform.TempDir(t)
	writeTranscript(t, root, childID, systemStep(0, parentID, "task"))
	writeDB(t, root, fixtureRow{id: childID, status: "CASCADE_RUN_STATUS_IDLE", uris: "[]", modified: time.Now()})
	db, err := sql.Open("sqlite", filepath.Join(root, summariesDB))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO conversation_parents VALUES (?, ?)", childID, parentID); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if got := byID(scan(t, New(root, nil), provider.ScanState{}))[childID].ParentID; got != parentID {
		t.Fatalf("ParentID = %q, want %q", got, parentID)
	}
}

func TestWorkspacePath(t *testing.T) {
	for uri, want := range map[string]string{
		"file:///home/a%20b/ws":          "/home/a b/ws",
		"file:///tmp/":                   "/tmp",
		"file:///C:/Users/A%20B/my%20ws": `C:\Users\A B\my ws`,
		"file:///c:/x":                   `C:\x`,
		"file:///d%3A/Workspaces/ws":     `D:\Workspaces\ws`,
		"file://server/share/dir":        `\\server\share\dir`,
		"file://localhost/srv/x":         "/srv/x",
		"https://example.com/x":          "",
		"not a uri %zz":                  "",
	} {
		if got := workspacePath(uri); got != want {
			t.Errorf("workspacePath(%q) = %q, want %q", uri, got, want)
		}
	}
	if got := firstWorkspace(`["file:///C:/a%20b","file:///tmp"]`); got != `C:\a b` {
		t.Errorf("firstWorkspace = %q", got)
	}
	if got := firstWorkspace("not json"); got != "" {
		t.Errorf("firstWorkspace(invalid) = %q", got)
	}
}

func TestScanWindowsWorkspace(t *testing.T) {
	root := platform.TempDir(t)
	writeTranscript(t, root, parentID, userStep(0, "hi"))
	writeDB(t, root, fixtureRow{id: parentID, status: "CASCADE_RUN_STATUS_IDLE", uris: `["file:///D:/My%20Projects/app"]`, modified: time.Now()})
	if got := byID(scan(t, New(root, nil), provider.ScanState{}))[parentID].CWD; got != `D:\My Projects\app` {
		t.Fatalf("CWD = %q", got)
	}
}

func TestScanWithoutIndex(t *testing.T) {
	root := platform.TempDir(t)
	writeTranscript(t, root, parentID, userStep(0, "Reply with just: hi"), plannerStep(1, "hi"))
	res := scan(t, New(root, nil), provider.ScanState{})
	if len(res.Diag.Warnings) != 0 {
		t.Errorf("a missing index is not a warning: %+v", res.Diag.Warnings)
	}
	m := byID(res)[parentID]
	if m.Title != "Reply with just: hi" || m.CWD != "" || m.Turn != model.TurnDone {
		t.Fatalf("meta = %+v", m)
	}
}

func TestScanCorruptIndexKeepsLastRows(t *testing.T) {
	root := platform.TempDir(t)
	writeTranscript(t, root, parentID, userStep(0, "hello"), plannerStep(1, "hi"))
	writeDB(t, root, fixtureRow{id: parentID, title: "Named", status: "CASCADE_RUN_STATUS_IDLE", uris: `["file:///tmp"]`, modified: time.Now()})
	p := New(root, nil)
	first := scan(t, p, provider.ScanState{})
	if byID(first)[parentID].Title != "Named" {
		t.Fatalf("first scan: %+v", first.Changed)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(filepath.Join(root, summariesDB+suffix))
	}
	if err := os.WriteFile(filepath.Join(root, summariesDB), []byte("this is not a database, just text padding it out"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := scan(t, p, first.State)
	if len(second.Diag.Warnings) == 0 {
		t.Error("an unreadable index should warn")
	}
	if len(second.Changed) != 0 {
		t.Errorf("unchanged session republished with the last row: %+v", second.Changed)
	}

	// A fresh scan has no last row: the transcript alone names the session.
	fresh := byID(scan(t, New(root, nil), provider.ScanState{}))[parentID]
	if fresh.Title != "hello" {
		t.Errorf("fresh title = %q", fresh.Title)
	}
}

func TestScanReadOnlyWALWithoutShm(t *testing.T) {
	platform.SkipWithoutModeBits(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := platform.TempDir(t)
	writeTranscript(t, root, parentID, userStep(0, "hello"))
	writeDB(t, root, fixtureRow{id: parentID, title: "Named", status: "CASCADE_RUN_STATUS_IDLE", uris: `["file:///tmp"]`, modified: time.Now()})
	_ = os.Remove(filepath.Join(root, summariesDB+"-shm"))
	_ = os.Remove(filepath.Join(root, summariesDB+"-wal"))
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	res := scan(t, New(root, nil), provider.ScanState{})
	if len(res.Diag.Warnings) == 0 {
		t.Error("expected an index warning")
	}
	if m := byID(res)[parentID]; m.Title != "hello" {
		t.Fatalf("meta = %+v", m)
	}
	if _, err := os.Stat(filepath.Join(root, summariesDB+"-shm")); !os.IsNotExist(err) {
		t.Errorf("-shm created in a read-only store: %v", err)
	}
}

func TestScanIncrementalAppend(t *testing.T) {
	root := platform.TempDir(t)
	// The long reply puts the middle of the file outside the windows the
	// checkpoint's prefix digest covers.
	path := writeTranscript(t, root, parentID, userStep(0, "first"), plannerStep(1, strings.Repeat("one ", 4<<10)))
	p := New(root, nil)
	first := scan(t, p, provider.ScanState{})
	if len(first.Changed) != 1 {
		t.Fatalf("first scan changed %d", len(first.Changed))
	}
	if again := scan(t, p, first.State); len(again.Changed) != 0 {
		t.Fatalf("unchanged rescan republished %d", len(again.Changed))
	}

	// Garble consumed bytes in place: a resumed scan never reads them.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	copy(data[len(data)/2:], strings.Repeat("x", 10))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	appendSteps(t, path, userStep(2, "second"), plannerStep(3, "two"))
	res := scan(t, p, first.State)
	if res.Diag.ParseErrors != 0 {
		t.Fatalf("resumed scan re-read consumed bytes: %+v", res.Diag)
	}
	m := byID(res)[parentID]
	if want := (model.MessageCounts{User: 2, Assistant: 2}); m.Counts != want || m.FirstPrompt != "first" {
		t.Fatalf("counts/prompt = %+v %q", m.Counts, m.FirstPrompt)
	}
	info, _ := os.Stat(path)
	if st := res.State.Sources[path]; st.Offset != info.Size() || st.Size != info.Size() {
		t.Fatalf("state = %+v, size %d", st, info.Size())
	}
}

// A larger transcript whose head changed (a replacement, not an append) is
// read from the start.
func TestScanRebuildsReplacedTranscript(t *testing.T) {
	root := platform.TempDir(t)
	path := writeTranscript(t, root, parentID, userStep(0, "first"), plannerStep(1, "one"))
	p := New(root, nil)
	first := scan(t, p, provider.ScanState{})
	writeTranscript(t, root, parentID, userStep(0, "other"), plannerStep(1, "two"), userStep(2, "more"))
	res := scan(t, p, first.State)
	m := byID(res)[parentID]
	if want := (model.MessageCounts{User: 2, Assistant: 1}); m.Counts != want || m.FirstPrompt != "other" {
		t.Fatalf("counts/prompt = %+v %q", m.Counts, m.FirstPrompt)
	}
	if info, _ := os.Stat(path); res.State.Sources[path].Offset != info.Size() {
		t.Fatalf("state = %+v", res.State.Sources[path])
	}
}

func TestScanPrunesDeletedSession(t *testing.T) {
	root := platform.TempDir(t)
	writeTranscript(t, root, parentID, userStep(0, "a"))
	writeTranscript(t, root, childID, userStep(0, "b"))
	p := New(root, nil)
	first := scan(t, p, provider.ScanState{})
	if err := os.RemoveAll(filepath.Join(root, "brain", childID)); err != nil {
		t.Fatal(err)
	}
	res := scan(t, p, first.State)
	if len(res.Removed) != 1 || res.Removed[0] != (model.SessionRef{Agent: model.AgentAntigravity, ID: childID}) {
		t.Fatalf("removed = %+v", res.Removed)
	}
	if _, ok := res.State.Sources[transcriptPath(root, childID)]; ok {
		t.Error("removed session kept its state")
	}
}

// A session read from transcript.jsonl that gains transcript_full.jsonl is
// tracked under the new name: the old file's state goes, with no warning.
func TestScanTranscriptRenamed(t *testing.T) {
	root := platform.TempDir(t)
	short := filepath.Join(root, "brain", parentID, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(short), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(short, []byte(userStep(0, "short")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(root, nil)
	first := scan(t, p, provider.ScanState{})
	full := writeTranscript(t, root, parentID, userStep(0, "full"))
	for range 2 {
		res := scan(t, p, first.State)
		if len(res.Diag.Warnings) != 0 || len(res.Removed) != 0 {
			t.Fatalf("warnings %+v, removed %+v", res.Diag.Warnings, res.Removed)
		}
		if _, ok := res.State.Sources[short]; ok || len(res.State.Sources) != 1 {
			t.Fatalf("sources = %v", res.State.Sources)
		}
		if _, ok := res.State.Sources[full]; !ok {
			t.Fatalf("sources = %v", res.State.Sources)
		}
		first = res
	}
}

func TestDiscoverRejectsEscapingSymlink(t *testing.T) {
	root := platform.TempDir(t)
	outside := platform.TempDir(t)
	writeTranscript(t, outside, childID, userStep(0, "outside"))
	if err := os.MkdirAll(filepath.Join(root, "brain"), 0o755); err != nil {
		t.Fatal(err)
	}
	platform.Symlink(t, filepath.Join(outside, "brain", childID), filepath.Join(root, "brain", childID))
	writeTranscript(t, root, parentID, userStep(0, "inside"))
	res := scan(t, New(root, nil), provider.ScanState{})
	if len(res.Changed) != 1 || res.Changed[0].Ref.ID != parentID {
		t.Fatalf("changed = %+v", res.Changed)
	}
	if len(res.Diag.Warnings) != 1 || !strings.Contains(res.Diag.Warnings[0].Msg, "escapes") {
		t.Fatalf("warnings = %+v", res.Diag.Warnings)
	}
}

func TestDiscoverSkipsChunksAndFallsBack(t *testing.T) {
	root := platform.TempDir(t)
	writeTranscript(t, root, parentID, userStep(0, "full"))
	chunks := filepath.Join(root, "brain", parentID, ".system_generated", "logs", "chunks")
	if err := os.MkdirAll(chunks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunks, "chunk-0.jsonl"), []byte(userStep(0, "full")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only chunks: not a session.
	onlyChunks := filepath.Join(root, "brain", "11111111-1111-1111-1111-111111111111", ".system_generated", "logs", "chunks")
	if err := os.MkdirAll(onlyChunks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(onlyChunks, "chunk-0.jsonl"), []byte(userStep(0, "x")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// transcript.jsonl is used when there is no transcript_full.jsonl.
	short := filepath.Join(root, "brain", childID, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(short), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(short, []byte(userStep(0, "short")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var diag provider.Diagnostics
	sources, err := New(root, nil).discover(context.Background(), &diag)
	if err != nil {
		t.Fatal(err)
	}
	want := []source{{ID: childID, Path: short}, {ID: parentID, Path: transcriptPath(root, parentID)}}
	if fmt.Sprint(sources) != fmt.Sprint(want) {
		t.Fatalf("sources = %v, want %v", sources, want)
	}
}

func TestScanTurn(t *testing.T) {
	now := time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC)
	recent, stale := now.Add(-time.Minute), now.Add(-time.Hour)
	running := fixtureRow{status: "CASCADE_RUN_STATUS_RUNNING", uris: "[]", notIdle: true, modified: recent}
	with := func(r fixtureRow, f func(*fixtureRow)) *fixtureRow { f(&r); return &r }
	cases := []struct {
		name    string
		steps   []string
		row     *fixtureRow
		modTime time.Time
		want    string
	}{
		{"prompt with recent RUNNING row", []string{userStep(0, "go")}, &running, recent, model.TurnWorking},
		{"pending tool call", []string{userStep(0, "go"), plannerStep(1, "", "run_command")}, &running, recent, model.TurnTool},
		{"tool result in", []string{userStep(0, "go"), plannerStep(1, "", "run_command"), genericStep(2)}, &running, recent, model.TurnWorking},
		{"one of two results in", []string{userStep(0, "go"), plannerStep(1, "", "a", "b"), genericStep(2)}, &running, recent, model.TurnTool},
		{"recent RUNNING row after a final reply", []string{userStep(0, "go"), plannerStep(1, "done")}, &running, recent, model.TurnDone},
		{
			"stale CANCELING row",
			[]string{userStep(0, "go"), plannerStep(1, "", "run_command")},
			with(running, func(r *fixtureRow) { r.status, r.modified = "CASCADE_RUN_STATUS_CANCELING", stale }), stale, model.TurnDone,
		},
		{
			"recent CANCELING row",
			[]string{userStep(0, "go")},
			with(running, func(r *fixtureRow) { r.status = "CASCADE_RUN_STATUS_CANCELING" }), recent, model.TurnWorking,
		},
		{"empty status", []string{userStep(0, "go")}, with(running, func(r *fixtureRow) { r.status = "" }), recent, model.TurnDone},
		{"idle", []string{userStep(0, "go")}, with(running, func(r *fixtureRow) { r.status = "CASCADE_RUN_STATUS_IDLE" }), recent, model.TurnDone},
		{"fully idle", []string{userStep(0, "go")}, with(running, func(r *fixtureRow) { r.notIdle = false }), recent, model.TurnDone},
		{"killed", []string{userStep(0, "go")}, with(running, func(r *fixtureRow) { r.killed = true }), recent, model.TurnDone},
		{"stale row, fresh transcript", []string{userStep(0, "go")}, with(running, func(r *fixtureRow) { r.modified = stale }), recent, model.TurnWorking},
		{"no row, fresh transcript", []string{userStep(0, "go")}, nil, recent, model.TurnWorking},
		{"no row, stale transcript", []string{userStep(0, "go")}, nil, stale, model.TurnDone},
		{"cancelled turn", []string{
			userStep(0, "go"), plannerStep(1, "part"),
			mustJSON(map[string]any{"step_index": 2, "type": stepErrorMessage, "created_at": at(2), "content": "Error: The stream was interrupted"}),
		}, &running, recent, model.TurnDone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := platform.TempDir(t)
			path := writeTranscript(t, root, parentID, tc.steps...)
			if err := os.Chtimes(path, tc.modTime, tc.modTime); err != nil {
				t.Fatal(err)
			}
			if tc.row != nil {
				r := *tc.row
				r.id = parentID
				writeDB(t, root, r)
			}
			if got := byID(scan(t, newTestProvider(t, root, now), provider.ScanState{}))[parentID].Turn; got != tc.want {
				t.Fatalf("Turn = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScanRepublishesStaleTurn(t *testing.T) {
	now := time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC)
	root := platform.TempDir(t)
	path := writeTranscript(t, root, parentID, userStep(0, "go"))
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	p := newTestProvider(t, root, now)
	first := scan(t, p, provider.ScanState{})
	if got := byID(first)[parentID].Turn; got != model.TurnWorking {
		t.Fatalf("first Turn = %q", got)
	}
	p.now = func() time.Time { return now.Add(liveWindow + time.Second) }
	second := scan(t, p, first.State)
	if got := byID(second)[parentID].Turn; got != model.TurnDone {
		t.Fatalf("an unchanged transcript gone quiet should republish as done, got %+v", second.Changed)
	}
	if third := scan(t, p, second.State); len(third.Changed) != 0 {
		t.Fatalf("third scan republished %+v", third.Changed)
	}
}

func TestDetect(t *testing.T) {
	root := platform.TempDir(t)
	p := New(root, nil)
	if det, err := p.Detect(context.Background()); err != nil || det.Present {
		t.Fatalf("empty store: %+v %v", det, err)
	}
	if err := os.Mkdir(filepath.Join(root, "brain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if det, err := p.Detect(context.Background()); err != nil || !det.Present || det.Roots[0] != root {
		t.Fatalf("store with brain/: %+v %v", det, err)
	}
}
