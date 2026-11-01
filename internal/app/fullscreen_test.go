package app

import (
	"context"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFullscreenToggle(t *testing.T) {
	tests := []struct {
		name             string
		goos             string
		fullscreen       bool
		maximised        bool
		restoreMaximised bool
		want             []string
		wantRestore      bool
	}{
		{
			name: "windows maximised entry", goos: "windows", maximised: true,
			want: []string{"isFullscreen", "isMaximised", "fullscreen"}, wantRestore: true,
		},
		{
			name: "windows normal entry clears stale state", goos: "windows", restoreMaximised: true,
			want: []string{"isFullscreen", "isMaximised", "fullscreen"},
		},
		{
			name: "windows maximised exit", goos: "windows", fullscreen: true, restoreMaximised: true,
			want: []string{"isFullscreen", "unfullscreen", "unmaximise", "maximise"},
		},
		{
			name: "windows normal exit", goos: "windows", fullscreen: true,
			want: []string{"isFullscreen", "unfullscreen"},
		},
		{
			name: "linux entry", goos: "linux", maximised: true, restoreMaximised: true,
			want: []string{"isFullscreen", "fullscreen"},
		},
		{
			name: "linux exit", goos: "linux", fullscreen: true, restoreMaximised: true,
			want: []string{"isFullscreen", "unfullscreen"},
		},
		{
			name: "macos entry", goos: "darwin", maximised: true,
			want: []string{"isFullscreen", "fullscreen"},
		},
		{
			name: "macos exit", goos: "darwin", fullscreen: true, restoreMaximised: true,
			want: []string{"isFullscreen", "unfullscreen"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			record := func(name string) func(context.Context) {
				return func(got context.Context) {
					if got != ctx {
						t.Fatal("operation did not receive the startup context")
					}
					calls = append(calls, name)
				}
			}
			ops := fullscreenOperations{
				isFullscreen: func(ctx context.Context) bool {
					record("isFullscreen")(ctx)
					return tt.fullscreen
				},
				isMaximised: func(ctx context.Context) bool {
					record("isMaximised")(ctx)
					return tt.maximised
				},
				fullscreen: record("fullscreen"), unfullscreen: record("unfullscreen"),
				unmaximise: record("unmaximise"), maximise: record("maximise"),
			}
			c := fullscreenController{restoreMaximised: tt.restoreMaximised}
			if err := c.toggle(ctx, tt.goos, ops); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tt.want) {
				t.Fatalf("calls = %v, want %v", calls, tt.want)
			}
			if c.restoreMaximised != tt.wantRestore {
				t.Fatalf("restoreMaximised = %v, want %v", c.restoreMaximised, tt.wantRestore)
			}
		})
	}
}

func TestFullscreenToggleResetsAcrossCycles(t *testing.T) {
	var c fullscreenController
	var fullscreen bool
	maximised := true
	var restores int
	ops := fullscreenOperations{
		isFullscreen: func(context.Context) bool { return fullscreen },
		isMaximised:  func(context.Context) bool { return maximised },
		fullscreen:   func(context.Context) { fullscreen = true },
		unfullscreen: func(context.Context) { fullscreen = false },
		unmaximise:   func(context.Context) {},
		maximise:     func(context.Context) { restores++ },
	}
	for cycle := 0; cycle < 2; cycle++ {
		for toggle := 0; toggle < 2; toggle++ {
			if err := c.toggle(context.Background(), "windows", ops); err != nil {
				t.Fatal(err)
			}
		}
		if c.restoreMaximised {
			t.Fatal("saved maximised flag survived exit")
		}
		maximised = false
	}
	if restores != 1 {
		t.Fatalf("maximised %d times, want only the first cycle", restores)
	}
}

func TestFullscreenToggleSerializesPerApp(t *testing.T) {
	var a, other App
	var active atomic.Int32
	var overlap atomic.Bool
	var fullscreen bool
	var calls int
	ops := fullscreenOperations{
		isFullscreen: func(context.Context) bool {
			if active.Add(1) != 1 {
				overlap.Store(true)
			}
			// Leave the transaction active across the query and mutation, not
			// just within one operation, and give other callers a chance to run.
			runtime.Gosched()
			return fullscreen
		},
		fullscreen: func(context.Context) {
			fullscreen = true
			calls++
			active.Add(-1)
		},
		unfullscreen: func(context.Context) {
			fullscreen = false
			calls++
			active.Add(-1)
		},
	}
	// Holding one App's controller must not block a different App.
	other.fullscreen.mu.Lock()
	defer other.fullscreen.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.fullscreen.toggle(context.Background(), "linux", ops); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if overlap.Load() {
		t.Fatal("fullscreen transactions overlapped")
	}
	if calls != 64 || fullscreen || active.Load() != 0 {
		t.Fatalf("calls = %d, fullscreen = %v, active = %d", calls, fullscreen, active.Load())
	}
}

func TestFullscreenToggleUnready(t *testing.T) {
	var a App
	if err := a.ToggleFullscreen(); err == nil || err.Error() != "fullscreen: the window is not ready" {
		t.Fatalf("ToggleFullscreen() error = %v", err)
	}
	// The guard must precede all operations and preserve any existing state.
	c := fullscreenController{restoreMaximised: true}
	if err := c.toggle(nil, "windows", fullscreenOperations{}); err == nil {
		t.Fatal("toggle with nil context succeeded")
	}
	if !c.restoreMaximised {
		t.Fatal("unready toggle changed saved state")
	}
}
