package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// capTranscript has messages m0…m(n-1); message i holds sizes[i] bytes of text.
func capTranscript(sizes ...int) *model.Transcript {
	t := &model.Transcript{}
	for i, n := range sizes {
		t.Messages = append(t.Messages, model.Message{
			ID:    fmt.Sprintf("m%d", i),
			Role:  "assistant",
			Parts: []model.Part{{Kind: "text", Text: strings.Repeat("x", n)}},
		})
	}
	return t
}

// size is the cap's weight of messages [from, to) of all.
func size(all *model.Transcript, from, to int) int64 {
	var n int64
	for i := from; i < to; i++ {
		n += model.MessageBytes(&all.Messages[i])
	}
	return n
}

func ids(p MessagesPage) string {
	var out []string
	for _, m := range p.Messages {
		out = append(out, m.ID)
	}
	return strings.Join(out, ",")
}

func TestCapPage(t *testing.T) {
	const k = 1000
	all := capTranscript(k, k, k, 10*k, k, k, k, k)
	for _, tc := range []struct {
		name          string
		offset, limit int
		cap           PageCap
		want          string
		wantOffset    int
		hasMore       bool
		capped        bool
	}{
		{"no cap", -1, 50, PageCap{}, "m0,m1,m2,m3,m4,m5,m6,m7", 0, false, false},
		{"budget fits all", -1, 50, PageCap{MaxBytes: size(all, 0, 8), Anchor: -1}, "m0,m1,m2,m3,m4,m5,m6,m7", 0, false, false},
		{"newest within budget", -1, 50, PageCap{MaxBytes: size(all, 4, 8), Anchor: -1}, "m4,m5,m6,m7", 4, false, true},
		{"newest stops at a large message", -1, 50, PageCap{MaxBytes: size(all, 3, 8) - 1, Anchor: -1}, "m4,m5,m6,m7", 4, false, true},
		{"earlier page keeps its end", 0, 3, PageCap{MaxBytes: size(all, 1, 3), Anchor: 2}, "m1,m2", 1, true, true},
		{"later page keeps its start", 4, 4, PageCap{MaxBytes: size(all, 4, 6), Anchor: 4}, "m4,m5", 4, true, true},
		{"jump grows both ways", 0, 8, PageCap{MaxBytes: size(all, 4, 7), Anchor: 5}, "m4,m5,m6", 4, true, true},
		{"one way blocked, the other keeps growing", 0, 8, PageCap{MaxBytes: size(all, 4, 8), Anchor: 4}, "m4,m5,m6,m7", 4, false, true},
		{"anchor alone over budget", -1, 50, PageCap{MaxBytes: 10, Anchor: 3}, "m3", 3, true, true},
		{"anchor before the page is clamped", 4, 4, PageCap{MaxBytes: size(all, 4, 5), Anchor: 0}, "m4", 4, true, true},
		{"anchor after the page is clamped", 0, 3, PageCap{MaxBytes: size(all, 2, 3), Anchor: 99}, "m2", 2, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, _ := pageOf(all, "r1", tc.offset, tc.limit)
			got := capPage(page, tc.cap)
			if ids(got) != tc.want || got.Offset != tc.wantOffset || got.HasMore != tc.hasMore || got.Capped != tc.capped {
				t.Fatalf("got %s offset %d hasMore %v capped %v; want %s offset %d hasMore %v capped %v",
					ids(got), got.Offset, got.HasMore, got.Capped, tc.want, tc.wantOffset, tc.hasMore, tc.capped)
			}
			if got.TotalCount != 8 || got.Revision != "r1" {
				t.Fatalf("total %d revision %q", got.TotalCount, got.Revision)
			}
			if tc.capped && got.Limit != len(got.Messages) {
				t.Fatalf("capped limit %d, %d messages", got.Limit, len(got.Messages))
			}
			if size(all, got.Offset, got.Offset+len(got.Messages)) > tc.cap.MaxBytes && len(got.Messages) > 1 && tc.cap.MaxBytes > 0 {
				t.Fatalf("page over budget")
			}
		})
	}
}

func TestCapPageEmptyAndPastTheEnd(t *testing.T) {
	pc := PageCap{MaxBytes: 1, Anchor: -1}
	page, _ := pageOf(capTranscript(), "", -1, 50)
	if got := capPage(page, pc); len(got.Messages) != 0 || got.Offset != 0 || got.Capped || got.HasMore {
		t.Fatalf("empty transcript = %+v", got)
	}
	page, _ = pageOf(capTranscript(5, 5), "", 7, 50)
	if got := capPage(page, pc); len(got.Messages) != 0 || got.Offset != 7 || got.Capped || got.HasMore {
		t.Fatalf("past the end = %+v", got)
	}
}

func TestGetMessagesCapped(t *testing.T) {
	svc, fake := setupTestService(t)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "s1"}
	all := fake.Transcripts["s1"]
	last := len(all.Messages) - 1
	page, err := svc.GetMessagesCapped(t.Context(), ref, -1, 50, PageCap{MaxBytes: 1, Anchor: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].ID != all.Messages[last].ID || page.Offset != last || !page.Capped || page.HasMore {
		t.Fatalf("capped latest page = %+v", page)
	}
	whole, err := svc.GetMessages(t.Context(), ref, -1, 50)
	if err != nil || whole.Capped || len(whole.Messages) != len(all.Messages) {
		t.Fatalf("uncapped page = %+v, %v", whole, err)
	}
	for _, pc := range []PageCap{{MaxBytes: -1}, {MaxBytes: 1, Anchor: -2}} {
		if _, err := svc.GetMessagesCapped(t.Context(), ref, -1, 50, pc); err == nil {
			t.Errorf("cap %+v: want an error", pc)
		}
	}
}
