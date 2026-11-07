package files

import (
	"context"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestMeasureList times List on a real folder; it runs only when asked:
//
//	AS_MEASURE_ROOT=/path/to/workspace go test -run TestMeasureList -v ./internal/files/
//
// One warm-up, then AS_MEASURE_RUNS (default 7) timed runs; it reports the
// median and the range, the Git commands of one run and what was listed.
// docs/perf.md (NG-04) records its results.
func TestMeasureList(t *testing.T) {
	root := os.Getenv("AS_MEASURE_ROOT")
	if root == "" {
		t.Skip("set AS_MEASURE_ROOT to a folder to measure")
	}
	runs := 7
	if n := os.Getenv("AS_MEASURE_RUNS"); n != "" {
		var err error
		if runs, err = strconv.Atoi(n); err != nil || runs < 1 {
			t.Fatalf("AS_MEASURE_RUNS=%q: want a positive count", n)
		}
	}
	var listing Listing
	var calls map[string]int
	var times []time.Duration
	for i := range runs + 1 {
		var mu sync.Mutex
		counted := map[string]int{}
		ctx := context.WithValue(context.Background(), gitInvocationKey{}, func(c string) { mu.Lock(); counted[c]++; mu.Unlock() })
		began := time.Now()
		got, err := List(ctx, root)
		elapsed := time.Since(began)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			times = append(times, elapsed)
		}
		listing, calls = got, counted
	}
	slices.Sort(times)
	t.Logf("List %s: median %v, min %v, max %v over %d runs", root, times[len(times)/2].Round(time.Millisecond),
		times[0].Round(time.Millisecond), times[len(times)-1].Round(time.Millisecond), len(times))
	t.Logf("git per run %v; files %d, directories %d, ignored %d, ignored folders %d, repositories %d, status %d, truncated %v",
		calls, len(listing.Files), len(listing.Directories), len(listing.Ignored), len(listing.IgnoredDirectories),
		len(listing.Repositories), len(listing.Status), listing.Truncated)
}
