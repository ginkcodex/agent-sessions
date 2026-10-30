package providertest

import (
	"fmt"
	"os"
	"testing"
)

func TestAppendTranscriptRecords(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, n := range []int{1, 10, 100} {
			t.Run(fmt.Sprintf("%s/%d", agent, n), func(t *testing.T) {
				root := t.TempDir()
				path := WriteTranscript(t, agent, root, TranscriptFixtureID, 0)
				p := TranscriptProvider(agent, root)
				ref := TranscriptRef(agent, TranscriptFixtureID)
				before, err := p.Load(t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				st, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				added := AppendTranscriptRecords(t, agent, path, TranscriptTurns, n)
				after, err := p.Load(t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				perTurn := TranscriptTurnRecords(agent)
				want := len(before.Messages) + 2*(n/perTurn) + min(n%perTurn, 2)
				if len(after.Messages) != want {
					t.Fatalf("messages=%d want=%d", len(after.Messages), want)
				}
				end, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if end.Size()-st.Size() != added {
					t.Fatal("incorrect appended byte count")
				}
			})
		}
	}
}
