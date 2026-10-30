package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// BenchmarkMessagesDeltaAppend times a live refresh of the latest 50-message
// window after appended records, as invalidation plus GetMessagesSince, and
// reports its JSON bytes next to the full page's.
func BenchmarkMessagesDeltaAppend(b *testing.B) {
	for _, agent := range []string{"claude", "codex"} {
		for _, mb := range []int{38, 100} {
			for _, records := range []int{1, 10} {
				b.Run(fmt.Sprintf("%s/%dMB/%drecords", agent, mb, records), func(b *testing.B) {
					root := b.TempDir()
					path := providertest.WriteTranscript(b, agent, root, providertest.TranscriptFixtureID, mb*1000000)
					p := providertest.TranscriptProvider(agent, root)
					ref := providertest.TranscriptRef(agent, providertest.TranscriptFixtureID)
					st, err := os.Stat(path)
					if err != nil {
						b.Fatal(err)
					}
					ctx := context.Background()
					var deltaJSON, pageJSON, reused int
					b.ReportAllocs()
					b.ResetTimer()
					b.StopTimer()
					for range b.N {
						if err := os.Truncate(path, st.Size()); err != nil {
							b.Fatal(err)
						}
						svc := NewService(nil, provider.Set{p})
						svc.transcripts.start(ctx)
						before, err := svc.GetMessages(ctx, ref, -1, 50)
						if err != nil {
							b.Fatal(err)
						}
						providertest.AppendTranscriptRecords(b, agent, path, providertest.TranscriptTurns, records)
						req := MessagesSinceRequest{Ref: ref, BaseRevision: before.Revision, HaveOffset: before.Offset, HaveCount: len(before.Messages), Offset: -1, Limit: 50}
						b.StartTimer()
						svc.EvictTranscripts([]model.SessionRef{ref}, nil) // as onChanged does
						delta, err := svc.GetMessagesSince(ctx, req)
						b.StopTimer()
						if err != nil {
							b.Fatal(err)
						}
						page, err := svc.GetMessages(ctx, ref, -1, 50)
						if err != nil {
							b.Fatal(err)
						}
						window := append(before.Messages[delta.Offset-before.Offset:delta.Offset-before.Offset+delta.Reused:delta.Offset-before.Offset+delta.Reused], delta.Messages...)
						if delta.Reused == 0 || !reflect.DeepEqual(window, page.Messages) {
							b.Fatalf("reused %d; window differs from the page", delta.Reused)
						}
						d, err := json.Marshal(delta)
						if err != nil {
							b.Fatal(err)
						}
						f, err := json.Marshal(page)
						if err != nil {
							b.Fatal(err)
						}
						deltaJSON, pageJSON, reused = len(d), len(f), delta.Reused
						svc.transcripts.stop()
					}
					b.ReportMetric(float64(deltaJSON), "delta-JSON-B")
					b.ReportMetric(float64(pageJSON), "page-JSON-B")
					b.ReportMetric(float64(reused), "reused")
				})
			}
		}
	}
}
