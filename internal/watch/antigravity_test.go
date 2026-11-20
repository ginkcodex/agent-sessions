package watch

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func TestAntigravityEvents(t *testing.T) {
	root := filepath.FromSlash("/s/antigravity-cli")
	at := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	logs := "brain/2b9a4c81-215f-47af-9395-99d2b08be416/.system_generated/"
	for rel, want := range map[string][2]bool{
		"conversation_summaries.db":           {true, true},
		"conversation_summaries.db-wal":       {true, true},
		logs + "logs/transcript_full.jsonl":   {true, false},
		logs + "logs/transcript.jsonl":        {true, false},
		logs + "logs/chunks/transcript.jsonl": {},
		logs + "steps/3/output.txt":           {},
		"brain/x/scratch/transcript.jsonl":    {},
		"conversation_summaries.db-shm":       {},
		"conversations/x.db":                  {},
		"conversations/x.db-wal":              {},
		"log/agy.log":                         {},
		"presence/x.lock":                     {},
		"cache/x.jsonl":                       {},
	} {
		matched, full := relevantEvent(model.AgentAntigravity, at(rel), fsnotify.Write)
		if [2]bool{matched, full} != want {
			t.Errorf("%s: matched=%v full=%v, want %v", rel, matched, full, want)
		}
	}

	fake := providertest.NewFake(model.AgentAntigravity, "Antigravity")
	fake.Watch = []string{at("brain"), at("conversation_summaries.db"), at("conversation_summaries.db-wal")}
	roots := rootsFor(fake)
	want := []watchRoot{{path: at("brain"), recursive: true}, {path: root}}
	if len(roots) != len(want) {
		t.Fatalf("roots = %+v, want %+v", roots, want)
	}
	for i := range want {
		if abs, _ := absoluteClean(want[i].path); roots[i].path != abs || roots[i].recursive != want[i].recursive {
			t.Fatalf("roots = %+v, want %+v", roots, want)
		}
	}
}

// A transcript append and an index write notify; noise beside them does not.
func TestAntigravityWatch(t *testing.T) {
	root := t.TempDir()
	logs := filepath.Join(root, "brain", "s1", ".system_generated", "logs")
	mkdir(t, logs)
	mkdir(t, filepath.Join(root, "log"))
	transcript := filepath.Join(logs, "transcript_full.jsonl")
	write := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(transcript)
	fake := providertest.NewFake(model.AgentAntigravity, "Antigravity")
	fake.Watch = []string{filepath.Join(root, "brain"), filepath.Join(root, "conversation_summaries.db"), filepath.Join(root, "conversation_summaries.db-wal")}
	h := startRecorded(t, fake)

	write(filepath.Join(root, "log", "agy.log"))
	write(filepath.Join(root, "presence.lock"))
	write(filepath.Join(logs, "notes.txt"))
	settle()
	if n := h.changeCount(); n != 0 {
		t.Fatalf("noise: %d rescans", n)
	}
	write(filepath.Join(root, "conversation_summaries.db-wal"))
	h.waitChange("index write")

	write(transcript)
	deadline := time.Now().Add(3 * time.Second)
	for h.changeCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.changes) == 0 || !slices.Contains(h.changes[len(h.changes)-1].Paths, transcript) {
		t.Fatalf("transcript append: changes %+v", h.changes)
	}
}
