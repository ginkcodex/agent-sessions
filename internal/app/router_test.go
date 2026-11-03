package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

// testWindows is a registry whose Apps have no providers and keep their
// caches and window claims in temporary directories.
func testWindows(t *testing.T) *Windows {
	t.Helper()
	w := NewWindows(nil, nil)
	w.newApp = func() *App {
		a := NewAppWithService(NewService(scan.NewCatalog(), provider.Set{}))
		a.cacheDirOverride = t.TempDir()
		return a
	}
	t.Cleanup(w.Shutdown)
	return w
}

type windowNameKey struct{}

// testRouter routes calls by the window name in the call's context.
func testRouter(w *Windows) *Router {
	return NewRouter(w, func(ctx context.Context) string {
		name, _ := ctx.Value(windowNameKey{}).(string)
		return name
	})
}

func fromWindow(name string) context.Context {
	return context.WithValue(context.Background(), windowNameKey{}, name)
}

func rawArgs(t *testing.T, args ...any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(args))
	for i, arg := range args {
		b, err := json.Marshal(arg)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = b
	}
	return out
}

func TestRouterCallsTheCallingWindowsApp(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	var mu sync.Mutex
	printed := map[string]int{}
	for range 2 {
		name := w.Reserve()
		w.Start(name, t.Context(), Desktop{Print: func(context.Context) error {
			mu.Lock()
			printed[name]++
			mu.Unlock()
			return nil
		}})
	}

	for _, call := range []string{"window-1", "window-2", "window-2"} {
		if _, err := r.Call(fromWindow(call), "PrintWindow", nil); err != nil {
			t.Fatalf("PrintWindow from %s: %v", call, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if printed["window-1"] != 1 || printed["window-2"] != 2 {
		t.Fatalf("prints by window = %v, want each window to print itself", printed)
	}
}

func TestRouterMatchesBoundAppResults(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	name := w.Reserve()
	w.Start(name, t.Context(), Desktop{})
	ctx := fromWindow(name)

	// A string result stays a string: Wails sends it as text, as before.
	got, err := r.Call(ctx, "AppVersion", nil)
	if s, ok := got.(string); err != nil || !ok || s == "" {
		t.Fatalf("AppVersion = %#v, %v", got, err)
	}
	// An App error comes back unchanged.
	_, err = r.Call(ctx, "PrintWindow", rawArgs(t))
	if !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("PrintWindow without a desktop: %v", err)
	}

	for _, tc := range []struct {
		method string
		args   []json.RawMessage
		want   string
	}{
		{"ListFiles", nil, "App.ListFiles expects 1 arguments, got 0"},
		{"AppVersion", rawArgs(t, "x"), "App.AppVersion expects 0 arguments, got 1"},
		{"ListFiles", rawArgs(t, 12), "could not parse argument #0: "},
		{"NoSuchMethod", nil, `unknown App method "NoSuchMethod"`},
		{"ServiceStartup", nil, `unknown App method "ServiceStartup"`},
	} {
		_, err := r.Call(ctx, tc.method, tc.args)
		if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("%s(%s) error = %v, want %q", tc.method, tc.args, err, tc.want)
		}
	}
}

func TestRouterRefusesCallsWithoutTheirWindow(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	name := w.Reserve()
	w.Start(name, t.Context(), Desktop{})

	if _, err := r.Call(context.Background(), "AppVersion", nil); err == nil || !strings.Contains(err.Error(), "no window") {
		t.Fatalf("call from no window: %v", err)
	}
	if _, err := r.Call(fromWindow("window-9"), "AppVersion", nil); !errors.Is(err, ErrWindowClosed) {
		t.Fatalf("call from an unknown window: %v", err)
	}
	w.Closed(name)
	if _, err := r.Call(fromWindow(name), "AppVersion", nil); !errors.Is(err, ErrWindowClosed) {
		t.Fatalf("call from a closed window: %v", err)
	}
	// A new window never takes a closed window's name.
	if next := w.Reserve(); next == name {
		t.Fatalf("name %s reused", next)
	}
}

func TestRouterWaitsForTheWindowToStart(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	name := w.Reserve()

	ctx, cancel := context.WithCancel(fromWindow(name))
	cancel()
	if _, err := r.Call(ctx, "AppVersion", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled call before start: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := r.Call(fromWindow(name), "PrintWindow", nil)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("call returned before the window started: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	printed := make(chan struct{}, 1)
	w.Start(name, t.Context(), Desktop{Print: func(context.Context) error {
		printed <- struct{}{}
		return nil
	}})
	if err := <-done; err != nil {
		t.Fatalf("call after start: %v", err)
	}
	select {
	case <-printed:
	default:
		t.Fatal("the call did not reach the started App")
	}
}

func TestRouterReleasesCallsOfAWindowClosedBeforeStart(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	name := w.Reserve()
	done := make(chan error, 1)
	go func() {
		_, err := r.Call(fromWindow(name), "AppVersion", nil)
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	w.Closed(name)
	select {
	case err := <-done:
		if !errors.Is(err, ErrWindowClosed) {
			t.Fatalf("call of a window closed before start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call still waiting for a window that closed")
	}
	// Starting it late does nothing.
	w.Start(name, t.Context(), Desktop{})
}

// routed is a stand-in for App with the method shapes Wails handles.
type routed struct{ ctx context.Context }

func (*routed) None()                             {}
func (*routed) Two(a int, b string) (int, string) { return a + 1, b + "!" }
func (*routed) Fails() (string, error)            { return "ignored", errors.New("it failed") }
func (*routed) FailsTwice() (error, error)        { return errors.New("one"), errors.New("two") }
func (*routed) Panics()                           { panic("boom") }
func (r *routed) WithContext(ctx context.Context, n int) int {
	r.ctx = ctx
	return n
}
func (*routed) Sum(base int, more ...int) int {
	for _, n := range more {
		base += n
	}
	return base
}
func (*routed) ServiceName() string { return "hidden" }

func TestRoutedMethodsBehaveLikeWailsBindings(t *testing.T) {
	methods := routedMethods(reflect.TypeFor[*routed]())
	if _, ok := methods["ServiceName"]; ok {
		t.Fatal("ServiceName is routed; Wails never binds it")
	}
	recv := &routed{}
	call := func(name string, args ...any) (any, error) {
		return methods[name].call(fromWindow("w"), reflect.ValueOf(recv), rawArgs(t, args...))
	}

	if got, err := call("None"); got != nil || err != nil {
		t.Fatalf("None = %#v, %v", got, err)
	}
	if got, err := call("Two", 1, "a"); err != nil || !reflect.DeepEqual(got, []any{2, "a!"}) {
		t.Fatalf("Two = %#v, %v", got, err)
	}
	if got, err := call("Fails"); got != nil || err == nil || err.Error() != "it failed" {
		t.Fatalf("Fails = %#v, %v", got, err)
	}
	if _, err := call("FailsTwice"); err == nil || err.Error() != "one\ntwo" {
		t.Fatalf("FailsTwice error = %v", err)
	}
	if _, err := call("Panics"); err == nil || err.Error() != "App.Panics: panic: boom" {
		t.Fatalf("Panics error = %v", err)
	}
	// The context is not one of the frontend's arguments.
	if got, err := call("WithContext", 7); err != nil || got != 7 || recv.ctx.Value(windowNameKey{}) != "w" {
		t.Fatalf("WithContext = %#v, %v (ctx %v)", got, err, recv.ctx)
	}
	if _, err := call("WithContext"); err == nil || err.Error() != "App.WithContext expects 2 arguments, got 1" {
		t.Fatalf("WithContext without its argument: %v", err)
	}
	// A variadic method takes its extra arguments as one list.
	if got, err := call("Sum", 1, []int{2, 3}); err != nil || got != 6 {
		t.Fatalf("Sum = %#v, %v", got, err)
	}
}
