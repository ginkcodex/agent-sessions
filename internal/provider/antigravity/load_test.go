package antigravity

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
	"github.com/ginkcode/agent-sessions/internal/testutil/golden"
)

var loadNow = time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC)

func ref(id string) model.SessionRef {
	return model.SessionRef{Agent: model.AgentAntigravity, ID: id}
}

func load(t *testing.T, p *Provider, prev provider.TranscriptState) provider.IncrementalResult {
	t.Helper()
	res, err := p.LoadIncremental(context.Background(), ref(parentID), prev)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func tool(t *testing.T, tx *model.Transcript, msgID string, k int) *model.ToolCall {
	t.Helper()
	for _, m := range tx.Messages {
		if m.ID != msgID {
			continue
		}
		n := 0
		for _, part := range m.Parts {
			if part.Kind == model.PartTool {
				if n == k {
					return part.Tool
				}
				n++
			}
		}
	}
	t.Fatalf("no tool %d in %s", k, msgID)
	return nil
}

func TestLoadGolden(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, parentID, "session")
	writeDB(t, root, fixtureRow{id: parentID, title: "Read the guide", status: "CASCADE_RUN_STATUS_IDLE", uris: `["file:///work"]`, agent: "self", modified: loadNow})
	p := newTestProvider(t, root, loadNow)

	// The subagent link needs the child's transcript.
	if child := tool(t, load(t, p, nil).Transcript, "step-8", 2).Child; child != nil {
		t.Fatalf("child linked before its transcript exists: %+v", child)
	}
	writeTranscript(t, root, childID, systemStep(0, parentID, "Check the docs"))
	tx := load(t, p, nil).Transcript

	// Load's meta is the scan's.
	if scanned := byID(scan(t, p, provider.ScanState{}))[parentID]; !reflect.DeepEqual(scanned, tx.Meta) {
		t.Fatalf("load meta differs from scan:\n%+v\n%+v", tx.Meta, scanned)
	}
	tx.Meta.SourcePath, tx.Meta.CWD, tx.Meta.CWDMissing, tx.Meta.RepoRoot = "<fixture>", "<cwd>", false, ""
	golden.JSON(t, "load_session", tx)
}

func copyFixture(t *testing.T, root, id, name string) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".jsonl")
	if err != nil {
		t.Fatal(err)
	}
	path := writeTranscript(t, root, id)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Background tasks, subagent tasks and reports, notices, a cancelled turn
// and a compaction, in a parent and its subagent.
func TestLoadGoldenSystemSteps(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, parentID, "parent")
	copyFixture(t, root, childID, "subagent")
	writeDB(t, root,
		fixtureRow{id: parentID, status: "CASCADE_RUN_STATUS_IDLE", uris: `["file:///work"]`, agent: "self", modified: loadNow},
		fixtureRow{id: childID, status: "CASCADE_RUN_STATUS_IDLE", uris: `["file:///work"]`, agent: "research", parent: parentID, modified: loadNow},
	)
	p := newTestProvider(t, root, loadNow)
	scanned := byID(scan(t, p, provider.ScanState{}))
	for name, id := range map[string]string{"parent": parentID, "subagent": childID} {
		tx, err := p.Load(context.Background(), ref(id))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(scanned[id], tx.Meta) {
			t.Fatalf("%s: load meta differs from scan:\n%+v\n%+v", name, tx.Meta, scanned[id])
		}
		tx.Meta.SourcePath, tx.Meta.CWD, tx.Meta.CWDMissing, tx.Meta.RepoRoot = "<fixture>", "<cwd>", false, ""
		golden.JSON(t, "load_"+name, tx)
	}
}

// A task result appended later completes the earlier call.
func TestLoadIncrementalTaskResult(t *testing.T) {
	root := t.TempDir()
	path := writeTranscript(t, root, parentID, userStep(0, "go"), plannerStep(1, "", "run_command"),
		mustJSON(map[string]any{"step_index": 2, "type": stepGeneric, "status": "RUNNING", "created_at": at(2), "content": "Created At: x\nTool is running as a background task"}),
		plannerStep(3, "waiting"))
	p := newTestProvider(t, root, loadNow)
	res := load(t, p, nil)
	if c := tool(t, res.Transcript, "step-1", 0); c.Status != model.ToolPending {
		t.Fatalf("running call = %s", c.Status)
	}
	appendSteps(t, path, systemStep(4, parentID+"/task-2", `Task id "`+parentID+`/task-2" finished with result:`+"\nCreated At: x\nall passed"))
	res = load(t, p, res.State)
	if !res.Resumed || res.FirstChanged != 1 || len(res.Transcript.Messages) != 3 {
		t.Fatalf("resumed=%v first=%d messages=%d", res.Resumed, res.FirstChanged, len(res.Transcript.Messages))
	}
	if c := tool(t, res.Transcript, "step-1", 0); c.Status != model.ToolCompleted || c.Output != "all passed" {
		t.Fatalf("call = %+v", c)
	}
}

// A long report is clipped, and Blob joins the result and the report.
func TestLoadReportBlob(t *testing.T) {
	root := t.TempDir()
	writeTranscript(t, root, childID, systemStep(0, parentID, "task"))
	report := strings.Repeat("r", maxOutputBytes)
	writeTranscript(t, root, parentID, userStep(0, "go"), plannerStep(1, "", "invoke_subagent"),
		mustJSON(map[string]any{"step_index": 2, "type": stepGeneric, "status": "DONE", "created_at": at(2), "content": `{"conversationId": "` + childID + `"}`}),
		systemStep(3, childID, report))
	p := newTestProvider(t, root, loadNow)
	c := tool(t, load(t, p, nil).Transcript, "step-1", 0)
	if c.Child == nil || !c.OutputTruncated || len(c.Output) != maxOutputBytes || !strings.Contains(c.OutputRef, ",") {
		t.Fatalf("call = child %v truncated %v len %d ref %q", c.Child, c.OutputTruncated, len(c.Output), c.OutputRef)
	}
	full, err := p.Blob(context.Background(), ref(parentID), c.OutputRef)
	if want := `{"conversationId": "` + childID + `"}` + "\n\n" + report; err != nil || string(full) != want {
		t.Fatalf("Blob = %d bytes, %v", len(full), err)
	}
}

// A child whose transcript appears after the call's result is linked on the
// next incremental load, with nothing appended to the parent.
func TestLoadIncrementalLinksLateChild(t *testing.T) {
	root := t.TempDir()
	writeTranscript(t, root, parentID, userStep(0, "go"), plannerStep(1, "", "invoke_subagent"),
		mustJSON(map[string]any{"step_index": 2, "type": stepGeneric, "status": "DONE", "created_at": at(2), "content": `{"conversationId": "` + childID + `"}`}))
	p := newTestProvider(t, root, loadNow)
	res := load(t, p, nil)
	if c := tool(t, res.Transcript, "step-1", 0); c.Child != nil {
		t.Fatalf("linked before the child exists: %+v", c.Child)
	}
	writeTranscript(t, root, childID, systemStep(0, parentID, "task"))
	res2 := load(t, p, res.State)
	if !res2.Resumed || res2.FirstChanged != 1 {
		t.Fatalf("resumed=%v first=%d", res2.Resumed, res2.FirstChanged)
	}
	if c := tool(t, res2.Transcript, "step-1", 0); c.Child == nil || c.Child.ID != childID {
		t.Fatalf("child = %+v", c.Child)
	}
	if tool(t, res.Transcript, "step-1", 0).Child != nil {
		t.Fatal("an earlier snapshot was mutated")
	}
}

// Past maxSegments reports the Blob key stops growing and ends in ",more";
// Blob still serves it, saying the later reports are left out.
func TestLoadCapsReportSegments(t *testing.T) {
	root := t.TempDir()
	writeTranscript(t, root, childID, systemStep(0, parentID, "task"))
	steps := []string{
		userStep(0, "go"), plannerStep(1, "", "invoke_subagent"),
		mustJSON(map[string]any{"step_index": 2, "type": stepGeneric, "status": "DONE", "created_at": at(2), "content": `{"conversationId": "` + childID + `"}`}),
	}
	report := strings.Repeat("r", 2<<10)
	for i := range maxSegments + 5 {
		steps = append(steps, systemStep(3+i, childID, report))
	}
	writeTranscript(t, root, parentID, steps...)
	p := newTestProvider(t, root, loadNow)
	c := tool(t, load(t, p, nil).Transcript, "step-1", 0)
	if !c.OutputTruncated || !strings.HasSuffix(c.OutputRef, ",more") || strings.Count(c.OutputRef, ",") != maxSegments {
		t.Fatalf("truncated=%v ref has %d commas, ends %q", c.OutputTruncated, strings.Count(c.OutputRef, ","), c.OutputRef[max(0, len(c.OutputRef)-12):])
	}
	full, err := p.Blob(context.Background(), ref(parentID), c.OutputRef)
	if err != nil || strings.Count(string(full), report) != maxSegments-1 || !strings.HasSuffix(string(full), "Open the subagent's session to read them.]") {
		t.Fatalf("Blob = %d bytes, %d reports, %v", len(full), strings.Count(string(full), report), err)
	}
	if _, err := p.Blob(context.Background(), ref(parentID), "rec:more"); err == nil {
		t.Error("Blob accepted a key of only the marker")
	}
}

func TestLoadClipsOutputAndServesBlob(t *testing.T) {
	root := t.TempDir()
	big := "Created At: 2026-11-19T11:00:00+07:00\nCompleted At: 2026-11-19T11:00:01+07:00\n" + strings.Repeat("é", 40<<10) + "end"
	path := writeTranscript(t, root, parentID,
		userStep(0, "dump"),
		plannerStep(1, "", "read_file"),
		mustJSON(map[string]any{"step_index": 2, "type": stepGeneric, "status": "DONE", "created_at": at(2), "content": big}),
	)
	p := newTestProvider(t, root, loadNow)
	call := tool(t, load(t, p, nil).Transcript, "step-1", 0)
	if !call.OutputTruncated || len(call.Output) > maxOutputBytes || !strings.HasPrefix(call.Output, "é") || call.OutputRef == "" {
		t.Fatalf("not clipped: truncated=%v len=%d ref=%q", call.OutputTruncated, len(call.Output), call.OutputRef)
	}
	full, err := p.Blob(context.Background(), ref(parentID), call.OutputRef)
	if err != nil || string(full) != strings.Repeat("é", 40<<10)+"end" {
		t.Fatalf("Blob = %d bytes, %v", len(full), err)
	}

	info, _ := os.Stat(path)
	for _, key := range []string{"", "x", "rec:", "rec:abc:2", "rec:0:2", "rec:-1:2", "rec:" + strconv.FormatInt(info.Size()+10, 10) + ":2", strings.Replace(call.OutputRef, ":2", ":3", 1)} {
		if _, err := p.Blob(context.Background(), ref(parentID), key); err == nil {
			t.Errorf("Blob(%q) succeeded", key)
		}
	}
	if _, err := p.Blob(context.Background(), ref("../"+parentID), call.OutputRef); err == nil {
		t.Error("Blob accepted a path in the session ID")
	}
}

func TestLoadIncremental(t *testing.T) {
	root := t.TempDir()
	path := writeTranscript(t, root, parentID, userStep(0, "go"), plannerStep(1, "", "read_file", "read_file"))
	p := newTestProvider(t, root, loadNow)
	cold := func() *model.Transcript { return load(t, p, nil).Transcript }
	check := func(res provider.IncrementalResult, resumed bool, first int) {
		t.Helper()
		if res.Resumed != resumed || (resumed && res.FirstChanged != first) {
			t.Fatalf("resumed=%v first=%d, want %v %d", res.Resumed, res.FirstChanged, resumed, first)
		}
		if want := cold(); !reflect.DeepEqual(res.Transcript, want) {
			t.Fatalf("resumed load differs from a cold one:\n%+v\n%+v", res.Transcript, want)
		}
	}

	res := load(t, p, nil)
	if res.State == nil || tool(t, res.Transcript, "step-1", 0).Status != model.ToolPending {
		t.Fatalf("state=%v", res.State)
	}
	res2 := load(t, p, res.State)
	check(res2, true, 2) // unchanged

	// A result appended later fills the earlier response's call.
	appendSteps(t, path, genericStep(2))
	res3 := load(t, p, res2.State)
	check(res3, true, 1)
	if c := tool(t, res3.Transcript, "step-1", 0); c.Status != model.ToolCompleted || c.Output != "ok" {
		t.Fatalf("call 0 = %+v", c)
	}
	if res2.Transcript.Messages[1].Parts[1].Tool.Status != model.ToolPending {
		t.Fatal("an earlier snapshot was mutated")
	}

	// A partial line is shown, but not committed.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString(genericStep(3))
	_ = f.Close()
	res4 := load(t, p, res3.State)
	check(res4, true, 1)
	if c := tool(t, res4.Transcript, "step-1", 1); c.Status != model.ToolCompleted {
		t.Fatalf("tail result not shown: %+v", c)
	}
	appendSteps(t, path, "", plannerStep(4, "done"))
	res5 := load(t, p, res4.State)
	check(res5, true, 1)
	if len(res5.Transcript.Messages) != 3 || res5.Transcript.Meta.Counts.Assistant != 2 {
		t.Fatalf("messages=%d counts=%+v", len(res5.Transcript.Messages), res5.Transcript.Meta.Counts)
	}

	// A rewrite is not an append: rebuild.
	writeTranscript(t, root, parentID, userStep(0, "other"))
	check(load(t, p, res5.State), false, 0)
}

func TestLoadMissingResultFailsOnlyOnceSkipped(t *testing.T) {
	root := t.TempDir()
	path := writeTranscript(t, root, parentID, userStep(0, "go"), plannerStep(1, "", "a", "b"), genericStep(3))
	p := newTestProvider(t, root, loadNow)
	tx := load(t, p, nil).Transcript
	if a, b := tool(t, tx, "step-1", 0), tool(t, tx, "step-1", 1); a.Status != model.ToolError || b.Status != model.ToolCompleted {
		t.Fatalf("a=%s b=%s", a.Status, b.Status)
	}
	appendSteps(t, path, plannerStep(4, "", "c"))
	if c := tool(t, load(t, p, nil).Transcript, "step-4", 0); c.Status != model.ToolPending {
		t.Fatalf("last call = %s, want pending", c.Status)
	}
}

func TestLoadRejectsBadRefs(t *testing.T) {
	root := t.TempDir()
	writeTranscript(t, root, parentID, userStep(0, "go"))
	p := newTestProvider(t, root, loadNow)
	for _, r := range []model.SessionRef{
		{Agent: model.AgentAntigravity},
		{Agent: model.AgentAntigravity, ID: ".."},
		{Agent: model.AgentAntigravity, ID: "../brain/" + parentID},
		{Agent: model.AgentAntigravity, ID: `x\..`},
		{Agent: model.AgentAntigravity, ID: "missing"},
		{Agent: model.AgentCodex, ID: parentID},
	} {
		if _, err := p.Load(context.Background(), r); err == nil {
			t.Errorf("Load(%+v) succeeded", r)
		}
	}
}

func TestResultText(t *testing.T) {
	for _, tc := range []struct {
		s    step
		want string
	}{
		{step{Content: "Created At: a\nCompleted At: b\nCreated At: a\nCompleted At: b\nout\nCreated At: kept"}, "out\nCreated At: kept"},
		{step{Content: "Created At: a\r\nline\r\n"}, "line\n"},
		{step{Content: "Created At: a\nCompleted At: b\n", Error: "boom"}, "boom"},
		{step{Content: "Created At: a", Error: ""}, ""},
		{step{Content: "no header"}, "no header"},
	} {
		if got := resultText(tc.s); got != tc.want {
			t.Errorf("resultText(%q) = %q, want %q", tc.s.Content, got, tc.want)
		}
	}
}

// Every line boundary of the fixture, unterminated and complete, loads
// incrementally the same as cold, with an accurate FirstChanged.
func TestLoadIncrementalEveryBoundary(t *testing.T) {
	for _, name := range []string{"session", "parent", "subagent"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			data, err := os.ReadFile("testdata/" + name + ".jsonl")
			if err != nil {
				t.Fatal(err)
			}
			if name != "subagent" {
				writeTranscript(t, root, childID, systemStep(0, parentID, "Check the docs"))
			}
			path := writeTranscript(t, root, parentID)
			p, cold := newTestProvider(t, root, loadNow), newTestProvider(t, root, loadNow)
			var h providertest.IncrementalHarness
			for i := range data {
				if data[i] != '\n' {
					continue
				}
				for _, end := range []int{i, i + 1} {
					if err := os.WriteFile(path, data[:end], 0o644); err != nil {
						t.Fatal(err)
					}
					if err := h.Check(t.Context(), p, cold, ref(parentID)); err != nil {
						t.Fatalf("byte %d: %v", end, err)
					}
				}
			}
		})
	}
}

func TestCommands(t *testing.T) {
	p := New(t.TempDir(), nil)
	if got := p.NewSessionCommand("/work"); !reflect.DeepEqual(got, provider.Command{Argv: []string{"agy"}, Dir: "/work"}) {
		t.Errorf("NewSessionCommand = %+v", got)
	}
	if got := p.ResumeCommand(model.SessionMeta{Ref: ref(parentID), CWD: "/work"}); !reflect.DeepEqual(got, provider.Command{Argv: []string{"agy", "--conversation", parentID}, Dir: "/work"}) {
		t.Errorf("ResumeCommand = %+v", got)
	}
}
