package transcriptbuild

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
)

func message(i int) model.Message {
	return model.Message{ID: fmt.Sprint(i), Parts: []model.Part{{Text: "old", Files: []string{"old"}, File: &model.FileRef{Ref: "old"}, Tool: &model.ToolCall{Input: []byte("{}"), Output: "old", Child: &model.SessionRef{ID: "old"}}}}}
}

func mutate(b *Builder, i int) {
	m := b.Touch(i)
	m.ID = "new"
	p := &m.Parts[0]
	p.Text = "new"
	p.Files[0] = "new"
	p.File.Ref = "new"
	p.Tool.Input[0] = '['
	p.Tool.Output = "new"
	p.Tool.Child.ID = "new"
}

func checkBytes(t *testing.T, b *Builder, messages []model.Message) {
	t.Helper()
	var want int64
	for i := range messages {
		want += model.MessageBytes(&messages[i])
	}
	if b.Bytes() != want {
		t.Fatalf("bytes=%d want=%d", b.Bytes(), want)
	}
}

func TestImmutableGenerations(t *testing.T) {
	var b Builder
	for i := range 20 {
		b.Append(message(i))
	}
	old, first := b.Freeze()
	if first != 0 || cap(old) != len(old) {
		t.Fatal("invalid first publication")
	}
	want := make([]model.Message, 20)
	for i := range want {
		want[i] = message(i)
	}
	fork := b.Fork()
	for i := range 20 {
		mutate(&b, i)
	}
	b.Append(message(20))
	fresh, first := b.Freeze()
	if first != 0 || !reflect.DeepEqual(old, want) {
		t.Fatal("old snapshot mutated")
	}
	checkBytes(t, &b, fresh)
	// Fork and parent can append independently even if the original had spare capacity.
	fork.Append(message(99))
	mutate(fork, 2)
	branch, first := fork.Freeze()
	if first != 2 || branch[20].ID != "99" || fresh[20].ID != "20" || old[2].ID != "2" {
		t.Fatal("fork isolation failed")
	}
	checkBytes(t, fork, branch)
	b.Tool(ToolLoc{Message: 1, Part: 0}).Output = "later"
	later, first := b.Freeze()
	if first != 1 || fresh[1].Parts[0].Tool.Output != "new" || later[1].Parts[0].Tool.Output != "later" {
		t.Fatal("refreeze mutated snapshot")
	}
	if _, first = b.Freeze(); first != b.Len() {
		t.Fatal("unchanged freeze")
	}
}

func TestFrozenReaders(t *testing.T) {
	var b Builder
	for i := range 200 {
		b.Append(message(i))
	}
	frozen, _ := b.Freeze()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			<-start
			for range 20 {
				for i, m := range frozen {
					if m.ID != fmt.Sprint(i) || m.Parts[0].Tool.Output != "old" {
						t.Error("frozen reader changed")
					}
				}
			}
		})
	}
	close(start)
	for i := range 200 {
		mutate(&b, i)
	}
	b.Freeze()
	wg.Wait()
}

func TestAppendOnlyAndRepeatedTouch(t *testing.T) {
	var b Builder
	b.Append(message(0))
	old, _ := b.Freeze()
	b.Append(message(1))
	if b.Changed() != 1 || b.Overhead() <= 0 {
		t.Fatal("missing change boundary or storage overhead")
	}
	current, first := b.Freeze()
	if b.Changed() != b.Len() {
		t.Fatal("freeze did not reset change boundary")
	}
	if first != 1 || len(old) != 1 {
		t.Fatal("append-only boundary")
	}
	p := b.Touch(0)
	p.ID = "changed"
	backing := &b.messages[0]
	b.Touch(0).Parts[0].Text = "changed again"
	if backing != &b.messages[0] {
		t.Fatal("repeated touch copied the message array")
	}
	b.Freeze()
	if current[0].ID != "0" {
		t.Fatal("published array changed")
	}
}

func TestRandomWeights(t *testing.T) {
	var b Builder
	rng := rand.New(rand.NewPCG(1, 2))
	for generation := range 100 {
		for range 20 {
			if b.Len() == 0 || rng.IntN(3) == 0 {
				b.Append(message(generation))
			} else {
				i := rng.IntN(b.Len())
				b.Touch(i).Parts[0].Tool.Output = fmt.Sprint(rng.Uint64())
			}
		}
		messages, _ := b.Freeze()
		checkBytes(t, &b, messages)
	}
}

func BenchmarkTouchGeneration(b *testing.B) {
	for _, n := range []int{100, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			var builder Builder
			for range n {
				builder.Append(model.Message{})
			}
			builder.Freeze()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for i := range n {
					builder.Touch(i).IsMeta = true
				}
				builder.Freeze()
			}
		})
	}
}
