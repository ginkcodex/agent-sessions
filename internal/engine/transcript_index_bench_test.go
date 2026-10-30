package engine

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/index"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// BenchmarkFTSLoadAppend times an FTS job's transcript load after an append to
// a session that is not open: the provider loader parses it in full, the
// service resumes from the background slot.
func BenchmarkFTSLoadAppend(b *testing.B) {
	for _, agent := range []string{"claude", "codex"} {
		for _, mb := range []int{38, 100} {
			for _, loader := range []string{"provider", "service"} {
				b.Run(fmt.Sprintf("%s/%dMB/%s", agent, mb, loader), func(b *testing.B) {
					root := b.TempDir()
					path := providertest.WriteTranscript(b, agent, root, providertest.TranscriptFixtureID, mb*1000000)
					p := providertest.TranscriptProvider(agent, root)
					ref := providertest.TranscriptRef(agent, providertest.TranscriptFixtureID)
					st, err := os.Stat(path)
					if err != nil {
						b.Fatal(err)
					}
					work := new(provider.LoadWork)
					ctx := provider.WithLoadWork(context.Background(), work)
					records := providertest.TranscriptTurnRecords(agent)
					var calls, sourceBytes int64
					b.ReportAllocs()
					b.ResetTimer()
					b.StopTimer()
					for range b.N {
						if err := os.Truncate(path, st.Size()); err != nil {
							b.Fatal(err)
						}
						var l index.TranscriptLoader = index.ProviderLoader(provider.Set{p})
						if loader == "service" {
							svc := NewService(nil, provider.Set{p})
							svc.transcripts.start(ctx)
							l = svc
						}
						if _, err := l.LoadForIndex(ctx, ref); err != nil {
							b.Fatal(err)
						}
						work.Calls.Store(0)
						work.SourceBytes.Store(0)
						providertest.AppendTranscriptRecords(b, agent, path, providertest.TranscriptTurns, records)
						b.StartTimer()
						tr, err := l.LoadForIndex(ctx, ref)
						b.StopTimer()
						if err != nil {
							b.Fatal(err)
						}
						if want := 2*providertest.TranscriptTurns + 2; len(tr.Messages) != want {
							b.Fatalf("messages=%d want=%d", len(tr.Messages), want)
						}
						calls += work.Calls.Load()
						sourceBytes += work.SourceBytes.Load()
						if svc, ok := l.(*Service); ok {
							svc.transcripts.stop()
						}
					}
					b.ReportMetric(float64(calls)/float64(b.N), "loads/op")
					b.ReportMetric(float64(sourceBytes)/float64(b.N), "source-B/op")
				})
			}
		}
	}
}
