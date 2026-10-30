package engine

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// All timings exclude generated fixture construction. Sequential requests always
// reconcile; Snapshot is path-only, unverified, and NOT an authoritative refresh.
// Same-key metrics are per four-client batch with natural (not forced) overlap.
func BenchmarkProjectIndex(b *testing.B) {
	projectFixtureEnvironment(b)
	for _, git := range []bool{false, true} {
		for _, count := range []int{1000, files.MaxFiles + 1} {
			b.Run(fmt.Sprintf("git=%v/files=%d", git, count), func(b *testing.B) {
				root := projectFixture(b, git, count)
				for _, mode := range []string{"List", "Manager", "Snapshot", "ListSameKey4", "ManagerSameKey4", "ListDifferentKey4", "ManagerDifferentKey4"} {
					b.Run(mode, func(b *testing.B) {
						roots := []string{root, root, root, root}
						if mode == "ListDifferentKey4" || mode == "ManagerDifferentKey4" {
							for i := 1; i < len(roots); i++ {
								roots[i] = projectFixture(b, git, count)
							}
						}
						p := newProjectIndex()
						defer p.stop()
						var loads atomic.Int64
						p.load = func(ctx context.Context, root string) (files.Listing, error) {
							loads.Add(1)
							return files.List(ctx, root)
						}
						if _, err := p.listing(context.Background(), root); err != nil {
							b.Fatal(err)
						}
						loads.Store(0)
						var statuses, paths atomic.Int64
						call := func(root string) error {
							var out files.Listing
							var err error
							switch mode {
							case "Snapshot":
								var ok bool
								out, _, ok = p.snapshot(root)
								if !ok {
									return fmt.Errorf("snapshot missing")
								}
							case "Manager", "ManagerSameKey4", "ManagerDifferentKey4":
								out, err = p.listing(context.Background(), root)
							default:
								loads.Add(1)
								out, err = files.List(context.Background(), root)
							}
							paths.Add(int64(len(out.Files)))
							statuses.Add(int64(len(out.Status)))
							return err
						}
						b.ReportAllocs()
						b.ResetTimer()
						for b.Loop() {
							if mode == "ListSameKey4" || mode == "ManagerSameKey4" || mode == "ListDifferentKey4" || mode == "ManagerDifferentKey4" {
								var wg sync.WaitGroup
								start := make(chan struct{})
								errs := make(chan error, 4)
								for i := range 4 {
									wg.Add(1)
									go func() { defer wg.Done(); <-start; errs <- call(roots[i]) }()
								}
								close(start)
								wg.Wait()
								close(errs)
								for err := range errs {
									if err != nil {
										b.Fatal(err)
									}
								}
							} else if err := call(root); err != nil {
								b.Fatal(err)
							}
						}
						b.StopTimer()
						b.ReportMetric(float64(loads.Load())/float64(b.N), "loads/op")
						b.ReportMetric(float64(paths.Load())/float64(b.N), "paths/op")
						b.ReportMetric(float64(statuses.Load())/float64(b.N), "status-entries/op")
						b.ReportMetric(float64(p.retained), "retained-est-B")
					})
				}
			})
		}
	}
}

// Heap deltas are whole-process post-GC samples, not precise object accounting.
// The manager is explicitly reachable across the live sample. No Listing result
// is kept alive: retained estimates cover snapshots, live bytes also include
// manager/maps, runtime bookkeeping, allocator rounding and measurement noise.
func BenchmarkProjectIndexRetained(b *testing.B) {
	projectFixtureEnvironment(b)
	for _, git := range []bool{false, true} {
		for _, count := range []int{1000, files.MaxFiles + 1} {
			b.Run(fmt.Sprintf("git=%v/files=%d", git, count), func(b *testing.B) {
				roots := make([]string, projectSnapshotLimit+1)
				for i := range roots {
					roots[i] = projectFixture(b, git, count)
				}
				var totalEstimate, totalLive int64
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					b.StopTimer()
					runtime.GC()
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					b.StartTimer()
					p := newProjectIndex()
					for _, root := range roots {
						if _, err := p.listing(context.Background(), root); err != nil {
							p.stop()
							b.Fatal(err)
						}
					}
					b.StopTimer()
					runtime.GC()
					runtime.ReadMemStats(&after)
					runtime.KeepAlive(p)
					totalEstimate += p.retained
					totalLive += int64(after.HeapAlloc) - int64(before.HeapAlloc)
					if len(p.snapshots) != projectSnapshotLimit || p.retained > projectSnapshotBudget {
						p.stop()
						b.Fatal("retention bound exceeded")
					}
					if _, _, ok := p.snapshot(roots[0]); ok {
						p.stop()
						b.Fatal("oldest snapshot not evicted")
					}
					p.stop()
					b.StartTimer()
				}
				b.StopTimer()
				b.ReportMetric(float64(totalEstimate)/float64(b.N), "retained-est-B/manager")
				b.ReportMetric(float64(totalLive)/float64(b.N), "heap-live-delta-B/manager")
			})
		}
	}
}
