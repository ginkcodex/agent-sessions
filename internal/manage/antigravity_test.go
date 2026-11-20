package manage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

const uuidC = "cccccccc-1111-2222-3333-444444444444"

// agyFixture writes one session's files in an agy store, as agy 1.5.12 does
// (docs/plan/antigravity-cli-delete.md), and returns its catalog entry.
func agyFixture(t *testing.T, root, id, parent string, extra ...string) model.SessionMeta {
	t.Helper()
	brain := filepath.Join(root, "brain", id)
	src := filepath.Join(brain, ".system_generated", "logs", "transcript_full.jsonl")
	for _, p := range append([]string{
		src,
		filepath.Join(brain, ".system_generated", "steps", "0.json"),
		filepath.Join(brain, "scratch", "notes.md"),
		filepath.Join(root, "conversations", id+".db"),
	}, extra...) {
		writeGuardFixture(t, p)
	}
	return model.SessionMeta{
		Ref:        model.SessionRef{Agent: model.AgentAntigravity, ID: id},
		ParentID:   parent,
		SourcePath: src,
		UpdatedAt:  time.Now().Add(-48 * time.Hour),
		Turn:       model.TurnDone,
		Title:      "agy " + id,
	}
}

// agyStore is a store with a parent (uuidA), its subagent (uuidB) and an
// unrelated session (uuidC), plus agy's own shared files.
func agyStore(t *testing.T) (string, []model.SessionMeta) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "antigravity-cli")
	catalog := []model.SessionMeta{
		agyFixture(t, root, uuidA, "",
			filepath.Join(root, "annotations", uuidA+".pbtxt"),
			filepath.Join(root, "conversations", uuidA+".db-wal")),
		agyFixture(t, root, uuidB, uuidA),
		agyFixture(t, root, uuidC, "", filepath.Join(root, "annotations", uuidC+".pbtxt")),
	}
	for _, shared := range []string{"conversation_summaries.db", "jetbox_summaries_proto.pb", "history.jsonl", "antigravity-oauth-token", filepath.Join("presence", uuidA+".lock")} {
		writeGuardFixture(t, filepath.Join(root, shared))
	}
	backdate(t, root)
	return root, catalog
}

func agyManager(t *testing.T, root string, opts ...Option) (*Manager, *fakeTrash) {
	t.Helper()
	ft := &fakeTrash{fail: map[string]error{}}
	m, _ := newTestManager(t, paths.Roots{Antigravity: root}, append([]Option{WithTrash(ft)}, opts...)...)
	enableAll(t, m)
	return m, ft
}

func agyRef(id string) []model.SessionRef {
	return []model.SessionRef{{Agent: model.AgentAntigravity, ID: id}}
}

func TestDelete_Antigravity_TrashesSessionAndSubagent(t *testing.T) {
	root, catalog := agyStore(t)
	m, ft := agyManager(t, root)

	p, err := m.Preview(context.Background(), agyRef(uuidA), catalog)
	if err != nil {
		t.Fatal(err)
	}
	item := p.Items[0]
	if item.Blocked != "" || item.Action != ActionTrash || !item.Reversible || item.Warning != AntigravityWarning || item.Bytes != 20 {
		t.Fatalf("item = %+v", item)
	}
	want := []string{
		filepath.Join(root, "brain", uuidB),
		filepath.Join(root, "conversations", uuidB+".db"),
		filepath.Join(root, "brain", uuidA),
		filepath.Join(root, "conversations", uuidA+".db"),
		filepath.Join(root, "conversations", uuidA+".db-wal"),
		filepath.Join(root, "annotations", uuidA+".pbtxt"),
	}
	if !reflect.DeepEqual(item.Paths, want) {
		t.Fatalf("paths = %v\nwant %v", item.Paths, want)
	}

	rep, err := m.Delete(context.Background(), agyRef(uuidA), catalog, p.Token)
	if err != nil || rep.Deleted != 1 || rep.FreedBytes != 20 {
		t.Fatalf("Delete = %+v, %v", rep, err)
	}
	if !reflect.DeepEqual(ft.calls, want) {
		t.Fatalf("trashed %v", ft.calls)
	}
	if !reflect.DeepEqual(rep.Forgotten, []model.SessionRef{agyRef(uuidA)[0], agyRef(uuidB)[0]}) {
		t.Fatalf("forgotten = %v", rep.Forgotten)
	}
}

func TestPreview_Antigravity_SubagentAlone(t *testing.T) {
	root, catalog := agyStore(t)
	m, _ := agyManager(t, root)
	p, err := m.Preview(context.Background(), agyRef(uuidB), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items[0].Paths) != 2 || p.Items[0].Blocked != "" {
		t.Fatalf("item = %+v", p.Items[0])
	}
}

func TestPreview_Antigravity_Blocked(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(t *testing.T, root string, catalog []model.SessionMeta)
		opts   []Option
		reason string
	}{
		{name: "agy TUI running", opts: []Option{WithProcFS(fakeProc{argv: map[int][]string{7: {"/home/u/.local/bin/agy"}}})}, reason: "agent process is running: agy"},
		{name: "print mode running", opts: []Option{WithProcFS(fakeProc{argv: map[int][]string{7: {"agy", "-p", "hi"}}})}, reason: "agent process is running: agy"},
		{name: "process table unknown", opts: []Option{WithProcFS(failingCmdlineProc{})}, reason: "cannot"},
		{name: "open in a chat", opts: []Option{WithOwned(func(ref model.SessionRef) bool { return ref.ID == uuidA })}, reason: "open in a chat"},
		{name: "subagent open in a chat", opts: []Option{WithOwned(func(ref model.SessionRef) bool { return ref.ID == uuidB })}, reason: "open in a chat"},
		{name: "transcript just written", edit: func(t *testing.T, _ string, c []model.SessionMeta) {
			now := time.Now()
			if err := os.Chtimes(c[0].SourcePath, now, now); err != nil {
				t.Fatal(err)
			}
		}, reason: "active within 10 minutes"},
		{name: "subagent just written", edit: func(_ *testing.T, _ string, c []model.SessionMeta) { c[1].UpdatedAt = time.Now() }, reason: "subagent cannot be safely deleted"},
		{name: "turn running", edit: func(_ *testing.T, _ string, c []model.SessionMeta) { c[0].Turn = model.TurnWorking }, reason: "currently active"},
		{name: "path of another session", edit: func(_ *testing.T, _ string, c []model.SessionMeta) { c[0].SourcePath = c[2].SourcePath }, reason: "does not match"},
		{name: "path outside the store", edit: func(_ *testing.T, _ string, c []model.SessionMeta) {
			c[0].SourcePath = filepath.Join(filepath.Dir(filepath.Dir(c[0].SourcePath)), "..", "..", "..", "..", "x", "logs", "t.jsonl")
		}, reason: "does not match"},
		{name: "symlink in brain", edit: func(t *testing.T, root string, _ []model.SessionMeta) {
			if err := os.Symlink(filepath.Join(root, "history.jsonl"), filepath.Join(root, "brain", uuidA, "scratch", "link")); err != nil {
				t.Fatal(err)
			}
		}, reason: "symlink"},
		{name: "credential in brain", edit: func(t *testing.T, root string, _ []model.SessionMeta) {
			writeGuardFixture(t, filepath.Join(root, "brain", uuidA, ".user_uploaded", "antigravity-oauth-token"))
			backdate(t, root)
		}, reason: "protected"},
		{name: "symlinked conversation", edit: func(t *testing.T, root string, _ []model.SessionMeta) {
			db := filepath.Join(root, "conversations", uuidA+".db")
			if err := os.Remove(db); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "conversation_summaries.db"), db); err != nil {
				t.Fatal(err)
			}
		}, reason: "symlink"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, catalog := agyStore(t)
			if tc.edit != nil {
				tc.edit(t, root, catalog)
			}
			m, _ := agyManager(t, root, tc.opts...)
			p, err := m.Preview(context.Background(), agyRef(uuidA), catalog)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(p.Items[0].Blocked, tc.reason) {
				t.Fatalf("blocked = %q, want %q", p.Items[0].Blocked, tc.reason)
			}
		})
	}
}

func TestPreview_Antigravity_ACPServerDoesNotBlock(t *testing.T) {
	root, catalog := agyStore(t)
	m, _ := agyManager(t, root, WithProcFS(fakeProc{argv: map[int][]string{7: {"/usr/local/bin/agy", "acp"}, 8: {"opencode", "run"}}}))
	p, err := m.Preview(context.Background(), agyRef(uuidC), catalog)
	if err != nil || p.Items[0].Blocked != "" {
		t.Fatalf("Preview = %+v, %v", p, err)
	}
}

func TestDelete_Antigravity_RechecksProcessesBeforeTrash(t *testing.T) {
	root, catalog := agyStore(t)
	procs := &switchProc{}
	m, ft := agyManager(t, root, WithProcFS(procs))
	p, err := m.Preview(context.Background(), agyRef(uuidC), catalog)
	if err != nil {
		t.Fatal(err)
	}
	// Delete's plan and execute's re-plan see no agy; a TUI starts before
	// the first file goes.
	procs.agyFrom = procs.calls + 3
	rep, err := m.Delete(context.Background(), agyRef(uuidC), catalog, p.Token)
	if err != nil || rep.Failed != 1 || len(ft.calls) != 0 || len(rep.Items[0].Remaining) != 3 {
		t.Fatalf("Delete = %+v, %v, trashed %v", rep, err, ft.calls)
	}
}

// switchProc shows an agy TUI from its agyFrom-th scan on.
type switchProc struct{ calls, agyFrom int }

func (s *switchProc) Cmdlines() (map[int][]string, error) {
	s.calls++
	if s.agyFrom != 0 && s.calls >= s.agyFrom {
		return map[int][]string{9: {"agy"}}, nil
	}
	return map[int][]string{}, nil
}

func TestCapture_Antigravity_SessionAndSubagent(t *testing.T) {
	root, catalog := agyStore(t)
	catalog[0].UpdatedAt = time.Now() // export ignores recency
	m, _ := agyManager(t, root)
	got, err := m.CaptureSession(context.Background(), catalog[0], catalog)
	if err != nil {
		t.Fatal(err)
	}
	rels := func(c Capture) (out []string) {
		for _, f := range c.Files {
			out = append(out, f.Rel)
		}
		return out
	}
	brain := func(id string) []string {
		return []string{
			"brain/" + id + "/.system_generated/logs/transcript_full.jsonl",
			"brain/" + id + "/.system_generated/steps/0.json",
			"brain/" + id + "/scratch/notes.md",
		}
	}
	wantA := append(brain(uuidA), "conversations/"+uuidA+".db", "conversations/"+uuidA+".db-wal", "annotations/"+uuidA+".pbtxt")
	wantB := append(brain(uuidB), "conversations/"+uuidB+".db")
	if len(got) != 2 || got[0].Ref.ID != uuidA || got[1].Ref.ID != uuidB || got[1].ParentID != uuidA ||
		!reflect.DeepEqual(rels(got[0]), wantA) || !reflect.DeepEqual(rels(got[1]), wantB) {
		t.Fatalf("captured %+v", got)
	}
}

func TestCapture_Antigravity_Refused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(t *testing.T, root string, c []model.SessionMeta)
		reason string
	}{
		{name: "running turn", edit: func(_ *testing.T, _ string, c []model.SessionMeta) { c[0].Turn = model.TurnWorking }, reason: "running"},
		{name: "running subagent", edit: func(_ *testing.T, _ string, c []model.SessionMeta) { c[1].Turn = model.TurnTool }, reason: "running"},
		{name: "credential in brain", edit: func(t *testing.T, root string, _ []model.SessionMeta) {
			writeGuardFixture(t, filepath.Join(root, "brain", uuidB, ".user_uploaded", "antigravity-oauth-token"))
		}, reason: "protected"},
		{name: "path of another session", edit: func(_ *testing.T, _ string, c []model.SessionMeta) { c[0].SourcePath = c[2].SourcePath }, reason: "does not match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, catalog := agyStore(t)
			tc.edit(t, root, catalog)
			m, _ := agyManager(t, root)
			if _, err := m.CaptureSession(context.Background(), catalog[0], catalog); err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err = %v, want %q", err, tc.reason)
			}
		})
	}
}
