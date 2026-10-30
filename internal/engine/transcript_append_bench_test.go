package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func BenchmarkTranscriptAppend(b *testing.B) {
	for _, agent := range []string{"claude", "codex"} {
		for _, mb := range []int{38, 100} {
			for _, records := range []int{1, 10, 100} {
				b.Run(fmt.Sprintf("%s/%dMB/%drecords", agent, mb, records), func(b *testing.B) {
					root := b.TempDir()
					path := providertest.WriteTranscript(b, agent, root, providertest.TranscriptFixtureID, mb*1000000)
					p := providertest.TranscriptProvider(agent, root)
					ref := providertest.TranscriptRef(agent, providertest.TranscriptFixtureID)
					st, err := os.Stat(path)
					if err != nil {
						b.Fatal(err)
					}
					work, scans := new(provider.LoadWork), new(provider.ScanWork)
					ctx := provider.WithScanWork(provider.WithLoadWork(context.Background(), work), scans)
					var appended, calls, sourceBytes, resolveBytes int64
					var page MessagesPage
					b.ReportAllocs()
					b.ResetTimer()
					b.StopTimer()
					for range b.N {
						// Reset both file and cache so each observation starts at the same size.
						if err := os.Truncate(path, st.Size()); err != nil {
							b.Fatal(err)
						}
						svc := NewService(nil, provider.Set{p})
						svc.transcripts.start(ctx)
						before, err := svc.GetMessages(context.Background(), ref, -1, 50)
						if err != nil {
							b.Fatal(err)
						}
						// Exclude the initial load from work counts as well as timing.
						work.Calls.Store(0)
						work.SourceBytes.Store(0)
						scans.RecordBytes.Store(0)
						appended = providertest.AppendTranscriptRecords(b, agent, path, providertest.TranscriptTurns, records)
						b.StartTimer()
						svc.EvictTranscripts([]model.SessionRef{ref}, nil) // as onChanged does
						page, err = svc.GetMessages(ctx, ref, -1, 50)
						b.StopTimer()
						if err != nil {
							b.Fatal(err)
						}
						calls += work.Calls.Load()
						sourceBytes += work.SourceBytes.Load()
						resolveBytes += scans.RecordBytes.Load()
						perTurn := providertest.TranscriptTurnRecords(agent)
						want := 2*providertest.TranscriptTurns + 2*(records/perTurn) + min(records%perTurn, 2)
						if page.TotalCount != want {
							b.Fatalf("messages=%d want=%d", page.TotalCount, want)
						}
						if records == 1 {
							full, err := json.Marshal(page)
							if err != nil {
								b.Fatal(err)
							}
							// Locate the true changed suffix in absolute message coordinates.
							first := page.Offset
							for first < before.Offset+len(before.Messages) && first < page.Offset+len(page.Messages) && reflect.DeepEqual(before.Messages[first-before.Offset], page.Messages[first-page.Offset]) {
								first++
							}
							delta, err := json.Marshal(page.Messages[first-page.Offset:])
							if err != nil {
								b.Fatal(err)
							}
							b.ReportMetric(float64(len(full)), "window-JSON-B")
							b.ReportMetric(float64(len(delta)), "changed-JSON-B")
						}
					}
					b.ReportMetric(float64(appended), "append-B")
					b.ReportMetric(float64(calls)/float64(b.N), "loads/op")
					b.ReportMetric(float64(sourceBytes)/float64(b.N), "source-B/op")
					b.ReportMetric(float64(resolveBytes)/float64(b.N), "resolve-record-B/op")
				})
			}
		}
	}
}

func BenchmarkCodexResolution(b *testing.B) {
	for _, extras := range []int{0, 64} {
		b.Run(fmt.Sprintf("%dextra", extras), func(b *testing.B) {
			root := b.TempDir()
			path := providertest.WriteTranscript(b, "codex", root, providertest.TranscriptFixtureID, 38000000)
			for i := range extras {
				providertest.WriteTranscript(b, "codex", root, fmt.Sprintf("00000000-0000-0000-0001-%012d", i), 1<<20)
			}
			ref := providertest.TranscriptRef("codex", providertest.TranscriptFixtureID)
			// cold-load resolves with a new provider, as every load did before
			// IT-06; warm-load repeats a load of unchanged sources; warm-append
			// appends one record, then continues both resolution and transcript.
			for _, mode := range []string{"resolution-scan", "cold-load", "warm-load", "warm-append"} {
				b.Run(mode, func(b *testing.B) {
					p := providertest.TranscriptProvider("codex", root)
					initial, err := provider.LoadIncremental(context.Background(), p, ref, nil)
					if err != nil {
						b.Fatal(err)
					}
					state := initial.State
					scans, work := new(provider.ScanWork), new(provider.LoadWork)
					ctx := provider.WithScanWork(provider.WithLoadWork(context.Background(), work), scans)
					b.ReportAllocs()
					b.ResetTimer()
					for i := range b.N {
						switch mode {
						case "resolution-scan":
							result, err := p.Scan(ctx, provider.ScanState{})
							if err != nil {
								b.Fatal(err)
							}
							if len(result.Changed) != extras+1 {
								b.Fatalf("sessions=%d", len(result.Changed))
							}
						case "cold-load":
							b.StopTimer()
							p = providertest.TranscriptProvider("codex", root)
							b.StartTimer()
							if _, err := p.Load(ctx, ref); err != nil {
								b.Fatal(err)
							}
						case "warm-load":
							if _, err := p.Load(ctx, ref); err != nil {
								b.Fatal(err)
							}
						case "warm-append":
							b.StopTimer()
							providertest.AppendTranscriptRecords(b, "codex", path, 100000+i, 1)
							b.StartTimer()
							next, err := provider.LoadIncremental(ctx, p, ref, state)
							if err != nil {
								b.Fatal(err)
							}
							if !next.Resumed {
								b.Fatal("did not resume")
							}
							state = next.State
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(scans.RecordBytes.Load())/float64(b.N), "resolve-record-B/op")
					b.ReportMetric(float64(scans.SourceOpens.Load())/float64(b.N), "resolve-opens/op")
					b.ReportMetric(float64(work.SourceBytes.Load())/float64(b.N), "source-B/op")
				})
			}
		})
	}
}

// BenchmarkTranscriptCacheBudget opens four 100 MB sessions in turn through one
// service, then reloads the last after an append, and reports what the cache
// keeps resident: entries, estimated bytes against the budget, and the live heap
// delta with the service reachable. Only the reload is timed.
func BenchmarkTranscriptCacheBudget(b *testing.B) {
	for _, agent := range []string{"claude", "codex"} {
		b.Run(agent, func(b *testing.B) {
			root := b.TempDir()
			refs := make([]model.SessionRef, 4)
			paths := make([]string, len(refs))
			sizes := make([]int64, len(refs))
			for i := range refs {
				id := fmt.Sprintf("00000000-0000-0000-0002-%012d", i)
				paths[i] = providertest.WriteTranscript(b, agent, root, id, 100000000)
				refs[i] = providertest.TranscriptRef(agent, id)
				st, err := os.Stat(paths[i])
				if err != nil {
					b.Fatal(err)
				}
				sizes[i] = st.Size()
			}
			p := providertest.TranscriptProvider(agent, root)
			var heap, estimate, resident, loads int64
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for range b.N {
				for i, path := range paths {
					if err := os.Truncate(path, sizes[i]); err != nil {
						b.Fatal(err)
					}
				}
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				svc := NewService(nil, provider.Set{p})
				work := new(provider.LoadWork)
				ctx := provider.WithLoadWork(context.Background(), work)
				svc.transcripts.start(ctx)
				for _, ref := range refs {
					if _, err := svc.GetMessages(ctx, ref, -1, 50); err != nil {
						b.Fatal(err)
					}
				}
				last := refs[len(refs)-1]
				providertest.AppendTranscriptRecords(b, agent, paths[len(paths)-1], providertest.TranscriptTurns, 1)
				work.Calls.Store(0)
				b.StartTimer()
				svc.EvictTranscripts([]model.SessionRef{last}, nil) // as onChanged does
				_, err := svc.GetMessages(ctx, last, -1, 50)
				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}
				loads += work.Calls.Load()
				runtime.GC()
				runtime.ReadMemStats(&after)
				heap += int64(after.HeapAlloc) - int64(before.HeapAlloc)
				c := svc.transcripts
				c.mu.Lock()
				c.lru.Range(func(_ string, e *transcriptEntry) bool {
					estimate += e.bytes()
					resident++
					return true
				})
				c.mu.Unlock()
				runtime.KeepAlive(svc)
				svc.transcripts.stop()
			}
			b.ReportMetric(float64(resident)/float64(b.N), "entries")
			b.ReportMetric(float64(estimate)/float64(b.N), "estimated-B")
			b.ReportMetric(float64(transcriptByteBudget), "budget-B")
			b.ReportMetric(float64(heap)/float64(b.N), "heap-live-B")
			b.ReportMetric(float64(loads)/float64(b.N), "loads/op")
			if estimate/int64(b.N) > transcriptByteBudget {
				b.Fatalf("estimated %d B over the %d B budget", estimate/int64(b.N), transcriptByteBudget)
			}
		})
	}
}

func BenchmarkTranscriptRetention(b *testing.B) {
	for _, agent := range []string{"claude", "codex"} {
		for _, mb := range []int{38, 100} {
			b.Run(fmt.Sprintf("%s/%dMB", agent, mb), func(b *testing.B) {
				p, ref, _ := providertest.TranscriptFixture(b, agent, mb*1000000)
				var heap, estimate int64
				b.ResetTimer()
				for range b.N {
					b.StopTimer()
					svc := NewService(nil, provider.Set{p})
					runtime.GC()
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					b.StartTimer()
					tr, err := svc.loadTranscript(context.Background(), ref)
					b.StopTimer()
					if err != nil {
						b.Fatal(err)
					}
					runtime.GC()
					runtime.ReadMemStats(&after)
					heap += int64(after.HeapAlloc) - int64(before.HeapAlloc)
					estimate += model.TranscriptBytes(tr)
					runtime.KeepAlive(svc)
					runtime.KeepAlive(tr)
				}
				b.ReportMetric(float64(heap)/float64(b.N), "heap-live-B")
				b.ReportMetric(float64(estimate)/float64(b.N), "estimated-B")
				if heap > 0 {
					b.ReportMetric(float64(estimate)/float64(heap), "estimate/heap")
				}
			})
		}
	}
}
