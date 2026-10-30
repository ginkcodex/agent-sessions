package providertest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/claude"
	"github.com/ginkcode/agent-sessions/internal/provider/codex"
)

// TranscriptTurns is the number of user/assistant turns in a generated transcript.
const TranscriptTurns = 256

// TranscriptFixtureID is the session ID of the generated transcript.
const TranscriptFixtureID = "00000000-0000-0000-0000-000000000002"

// TranscriptFixture writes one valid Claude or Codex transcript below tb.TempDir
// and returns a provider rooted there. targetBytes (decimal MB × 1,000,000) sets
// only the user prompt length, so the file is slightly larger than the target;
// targetBytes 0 gives the small equivalent with identical IDs and structure.
// Each turn has a split assistant message (two records), a tool call and its
// completed result. The file ends with a valid unterminated record that adds an
// unanswered tool call to the final assistant message.
func TranscriptFixture(tb testing.TB, agent string, targetBytes int) (provider.Provider, model.SessionRef, int64) {
	tb.Helper()
	root := tb.TempDir()
	path := WriteTranscript(tb, agent, root, TranscriptFixtureID, targetBytes)
	st, err := os.Stat(path)
	if err != nil {
		tb.Fatal(err)
	}
	return TranscriptProvider(agent, root), TranscriptRef(agent, TranscriptFixtureID), st.Size()
}

// TranscriptProvider returns a Claude or Codex provider rooted at root.
func TranscriptProvider(agent, root string) provider.Provider {
	if agent == "codex" {
		return codex.New(root, nil)
	}
	return claude.New(root, nil)
}

// TranscriptRef is the reference of a generated transcript with session ID id.
func TranscriptRef(agent, id string) model.SessionRef {
	if agent == "codex" {
		return model.SessionRef{Agent: model.AgentCodex, ID: id}
	}
	return model.SessionRef{Agent: model.AgentClaude, ID: id}
}

// TranscriptPath is where WriteTranscript puts session id below root.
func TranscriptPath(agent, root, id string) string {
	if agent == "codex" {
		return filepath.Join(root, "sessions", "2026", "10", "24", "rollout-2026-10-24T00-00-00-"+id+".jsonl")
	}
	return filepath.Join(root, "projects", "synthetic", id+".jsonl")
}

// WriteTranscript writes the TranscriptFixture content for session id below
// root and returns its path. Several sessions may share one root.
func WriteTranscript(tb testing.TB, agent, root, id string, targetBytes int) string {
	tb.Helper()
	path := TranscriptPath(agent, root, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		tb.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	w := bufio.NewWriter(f)
	write := func(s string) {
		if _, err := w.WriteString(s); err != nil {
			tb.Fatal(err)
		}
	}
	if agent == "codex" {
		write(fmt.Sprintf(`{"type":"session_meta",%s,"payload":{"id":%q}}`+"\n", fixtureStamp, id))
	}
	// 100 MB / 256 turns keeps every record well below jsonl.MaxLineLen.
	prompt := strings.Repeat("x", max(1, targetBytes/TranscriptTurns))
	for i := range TranscriptTurns {
		write(transcriptTurn(agent, i, prompt))
	}
	// A valid unterminated live-write tail continues the final split assistant; its unanswered
	// call is unknown because the generated root has no live-session evidence.
	if agent == "claude" {
		write(fmt.Sprintf(`{"type":"assistant","uuid":"tail",%s,"message":{"id":"a%d","role":"assistant","content":[{"type":"tool_use","id":"tail","name":"Read","input":{}}]}}`, fixtureStamp, TranscriptTurns-1))
	} else {
		write(fmt.Sprintf(`{"type":"response_item",%s,"payload":{"type":"function_call","call_id":"tail","name":"Read","arguments":{}}}`, fixtureStamp))
	}
	if err := w.Flush(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

const fixtureStamp = `"timestamp":"2026-10-24T00:00:00Z"`

// TranscriptTurnRecords is the number of JSONL records in one generated turn.
func TranscriptTurnRecords(agent string) int {
	if agent == "codex" {
		return 5
	}
	return 4
}

// transcriptTurn renders turn i: a user prompt, a split assistant message with
// a tool call, and the call's completed result.
func transcriptTurn(agent string, i int, prompt string) string {
	if agent == "claude" {
		return fmt.Sprintf(`{"type":"user","uuid":"u%d",%s,"message":{"role":"user","content":%q}}`+"\n", i, fixtureStamp, prompt) +
			fmt.Sprintf(`{"type":"assistant","uuid":"a%d",%s,"message":{"id":"a%d","role":"assistant","content":[{"type":"text","text":"first"}]}}`+"\n", i, fixtureStamp, i) +
			fmt.Sprintf(`{"type":"assistant",%s,"message":{"id":"a%d","role":"assistant","content":[{"type":"text","text":"second"},{"type":"tool_use","id":"t%d","name":"Read","input":{}}]}}`+"\n", fixtureStamp, i, i) +
			fmt.Sprintf(`{"type":"user",%s,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t%d","content":"done"}]}}`+"\n", fixtureStamp, i)
	}
	record := func(payload string) string {
		return fmt.Sprintf(`{"type":"response_item",%s,"payload":%s}`+"\n", fixtureStamp, payload)
	}
	return record(fmt.Sprintf(`{"type":"message","id":"u%d","role":"user","content":[{"type":"input_text","text":%q}]}`, i, prompt)) +
		record(fmt.Sprintf(`{"type":"message","id":"a%d","role":"assistant","content":[{"type":"output_text","text":"first"}]}`, i)) +
		record(fmt.Sprintf(`{"type":"message","id":"a%d","role":"assistant","content":[{"type":"output_text","text":"second"}]}`, i)) +
		record(fmt.Sprintf(`{"type":"function_call","call_id":"t%d","name":"Read","arguments":{}}`, i)) +
		record(fmt.Sprintf(`{"type":"function_call_output","call_id":"t%d","output":"done"}`, i))
}

// AppendTranscriptRecords appends exactly n complete records, starting at turn
// first. A partial final turn is intentional: live updates arrive record by record.
func AppendTranscriptRecords(tb testing.TB, agent, path string, first, n int) int64 {
	tb.Helper()
	var records []string
	for i := first; len(records) < n; i++ {
		records = append(records, strings.Split(strings.TrimSuffix(transcriptTurn(agent, i, "appended"), "\n"), "\n")...)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		tb.Fatal(err)
	}
	prefix := ""
	if st.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], st.Size()-1); err != nil {
			tb.Fatal(err)
		}
		if last[0] != '\n' {
			prefix = "\n"
		}
	}
	data := prefix + strings.Join(records[:n], "\n") + "\n"
	written, err := f.WriteString(data)
	if err != nil {
		tb.Fatal(err)
	}
	return int64(written)
}

// TranscriptEstimate is an intentionally approximate shallow-layout + content
// byte estimate, NOT a heap measurement. It excludes allocator rounding, metadata
// strings, sharing, parser temporaries, maps and GC/runtime overhead.
func TranscriptEstimate(tr *model.Transcript) int64 {
	n := int64(unsafe.Sizeof(*tr)) + int64(cap(tr.Messages))*int64(unsafe.Sizeof(model.Message{}))
	for _, m := range tr.Messages {
		n += int64(len(m.ID)+len(m.Model)) + int64(cap(m.Parts))*int64(unsafe.Sizeof(model.Part{}))
		for _, p := range m.Parts {
			n += int64(len(p.Text))
			if p.Tool != nil {
				n += int64(unsafe.Sizeof(*p.Tool)) + int64(len(p.Tool.ID)+len(p.Tool.Name)+len(p.Tool.Input)+len(p.Tool.Output)+len(p.Tool.OutputRef))
			}
		}
	}
	return n
}
