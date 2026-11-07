package all_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/all"
)

// TestMeasureTranscriptLoad times every provider's transcript loads for the
// sessions of one project, read only, on the real agent data of this user:
//
//	AS_MEASURE_CWD=/path/to/project go test -run TestMeasureTranscriptLoad -v ./internal/provider/all/
//
// Per provider: a cold and a warm Scan, then for each session whose working
// directory is the project or below it, the first Load on a fresh provider
// (as after the server starts), a second Load, and an incremental load that
// resumes the first one's state (as a reopened or live transcript does). It
// reports totals and the AS_MEASURE_TOP (default 5) slowest first loads, with
// the size of the viewer's first page uncapped and as the engine caps it
// (docs/plan/transcript-page-cap.md).
func TestMeasureTranscriptLoad(t *testing.T) {
	cwd := os.Getenv("AS_MEASURE_CWD")
	if cwd == "" {
		t.Skip("set AS_MEASURE_CWD to a project folder to measure")
	}
	top := 5
	if n := os.Getenv("AS_MEASURE_TOP"); n != "" {
		var err error
		if top, err = strconv.Atoi(n); err != nil || top < 1 {
			t.Fatalf("AS_MEASURE_TOP=%q: want a positive count", n)
		}
	}
	roots, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, scanner := range all.Providers(roots, nil) {
		began := time.Now()
		cold, err := scanner.Scan(ctx, provider.ScanState{})
		coldScan := time.Since(began)
		if err != nil {
			t.Logf("%s: scan: %v", scanner.ID(), err)
			continue
		}
		began = time.Now()
		if _, err := scanner.Scan(ctx, cold.State); err != nil {
			t.Fatalf("%s: warm scan: %v", scanner.ID(), err)
		}
		warmScan := time.Since(began)
		t.Logf("%s: %d sessions; cold scan %v, warm scan %v", scanner.ID(), len(cold.Changed),
			coldScan.Round(time.Millisecond), warmScan.Round(time.Millisecond))

		type sample struct {
			meta                  model.SessionMeta
			size                  int64
			messages              int
			page                  int    // JSON bytes of the newest page the viewer asks for first
			parts                 string // the page's bytes by part kind, largest first
			largest               int    // JSON bytes of the session's largest message
			capped                string // the engine's capped first page
			first, second, resume time.Duration
		}
		var samples []sample
		for _, meta := range cold.Changed {
			if meta.CWD != cwd && !strings.HasPrefix(meta.CWD, cwd+string(filepath.Separator)) {
				continue
			}
			s := sample{meta: meta}
			if info, err := os.Stat(meta.SourcePath); err == nil && info.Mode().IsRegular() {
				s.size = info.Size()
			}
			// A fresh provider per session: the first load pays what a first
			// open after the server starts pays.
			fresh, _ := all.Providers(roots, nil).Get(scanner.ID())
			began := time.Now()
			first, err := provider.LoadIncremental(ctx, fresh, meta.Ref, nil)
			s.first = time.Since(began)
			if err != nil {
				t.Logf("%s %s: load: %v", scanner.ID(), meta.Ref.ID, err)
				continue
			}
			s.messages = len(first.Transcript.Messages)
			s.page, s.parts = pageWeight(first.Transcript.Messages)
			s.largest = largestMessage(first.Transcript.Messages)
			if s.capped, err = cappedPage(ctx, fresh, meta.Ref); err != nil {
				t.Fatalf("%s %s: capped page: %v", scanner.ID(), meta.Ref.ID, err)
			}
			began = time.Now()
			if _, err := provider.LoadIncremental(ctx, fresh, meta.Ref, nil); err != nil {
				t.Fatalf("%s %s: second load: %v", scanner.ID(), meta.Ref.ID, err)
			}
			s.second = time.Since(began)
			began = time.Now()
			if _, err := provider.LoadIncremental(ctx, fresh, meta.Ref, first.State); err != nil {
				t.Fatalf("%s %s: resumed load: %v", scanner.ID(), meta.Ref.ID, err)
			}
			s.resume = time.Since(began)
			samples = append(samples, s)
		}
		if len(samples) == 0 {
			t.Logf("%s: no sessions in %s", scanner.ID(), cwd)
			continue
		}
		var total, totalSecond, totalResume time.Duration
		var bytes int64
		var biggest sample
		for _, s := range samples {
			if s.page > biggest.page {
				biggest = s
			}
			total, totalSecond, totalResume, bytes = total+s.first, totalSecond+s.second, totalResume+s.resume, bytes+s.size
		}
		slices.SortFunc(samples, func(a, b sample) int { return int(b.first - a.first) })
		t.Logf("%s: %d sessions in project, %.1f MB; first loads %v (median %v), second %v, resumed %v",
			scanner.ID(), len(samples), float64(bytes)/(1<<20), total.Round(time.Millisecond),
			samples[len(samples)/2].first.Round(time.Millisecond), totalSecond.Round(time.Millisecond), totalResume.Round(time.Millisecond))
		for _, s := range samples[:min(top, len(samples))] {
			t.Logf("  %s %.1f MB, %d messages: first %v, second %v, resumed %v; first page %.2f MB; largest message %.2f MB; capped page %s",
				s.meta.Ref.ID, float64(s.size)/(1<<20), s.messages, s.first.Round(time.Millisecond),
				s.second.Round(time.Millisecond), s.resume.Round(time.Millisecond), float64(s.page)/(1<<20),
				float64(s.largest)/(1<<20), s.capped)
		}
		t.Logf("  largest first page: %s, %.2f MB: %s", biggest.meta.Ref.ID, float64(biggest.page)/(1<<20), biggest.parts)
	}
}

// pageSize is the viewer's first request: the newest 50 messages
// (TranscriptView.svelte PAGE_SIZE).
const pageSize = 50

// pageWeight is the JSON size of the newest page and its bytes by part kind
// (tool parts split into input and output).
func pageWeight(messages []model.Message) (int, string) {
	page := messages[max(0, len(messages)-pageSize):]
	raw, _ := json.Marshal(page)
	byKind := map[string]int{}
	for _, m := range page {
		for _, p := range m.Parts {
			if p.Tool != nil {
				byKind[string(p.Kind)+".input"] += len(p.Tool.Input)
				byKind[string(p.Kind)+".output"] += len(p.Tool.Output)
				continue
			}
			b, _ := json.Marshal(p)
			byKind[string(p.Kind)] += len(b)
		}
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	slices.SortFunc(kinds, func(a, b string) int { return byKind[b] - byKind[a] })
	var out []string
	for _, k := range kinds[:min(4, len(kinds))] {
		out = append(out, fmt.Sprintf("%s %.2f MB", k, float64(byKind[k])/(1<<20)))
	}
	return len(raw), strings.Join(out, ", ")
}

// pageBudget is the viewer's page cap (PAGE_BYTES in frontend/src/lib/pageCap.ts).
const pageBudget = 256 << 10

// largestMessage is the JSON size of the largest message.
func largestMessage(messages []model.Message) int {
	largest := 0
	for i := range messages {
		raw, _ := json.Marshal(messages[i])
		largest = max(largest, len(raw))
	}
	return largest
}

// cappedPage is the viewer's first page as the engine caps it: its message
// count, its JSON size and the model.MessageBytes size the cap counted.
func cappedPage(ctx context.Context, p provider.Provider, ref model.SessionRef) (string, error) {
	svc := engine.NewService(nil, provider.Set{p})
	page, err := svc.GetMessagesCapped(ctx, ref, -1, pageSize, engine.PageCap{MaxBytes: pageBudget, Anchor: -1})
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(page.Messages)
	var counted int64
	for i := range page.Messages {
		counted += model.MessageBytes(&page.Messages[i])
	}
	return fmt.Sprintf("%d messages, %.2f MB JSON (%.2f MB counted)", len(page.Messages),
		float64(len(raw))/(1<<20), float64(counted)/(1<<20)), nil
}
