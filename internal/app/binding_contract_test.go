package app

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestFrontendBindingMatchesApp keeps the frontend's explicit App facade
// (APP_METHODS in frontend/src/lib/appBinding.ts) in step with the methods
// Wails binds: every exported method of *App.
func TestFrontendBindingMatchesApp(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "appBinding.ts"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)export const APP_METHODS = \[(.*?)\]`).FindSubmatch(src)
	if block == nil {
		t.Fatal("APP_METHODS not found in appBinding.ts")
	}
	var frontend []string
	for _, m := range regexp.MustCompile(`'([A-Za-z0-9_]+)'`).FindAllSubmatch(block[1], -1) {
		frontend = append(frontend, string(m[1]))
	}

	typ := reflect.TypeFor[*App]()
	var bound []string
	for i := range typ.NumMethod() {
		bound = append(bound, typ.Method(i).Name)
	}
	slices.Sort(frontend)
	slices.Sort(bound)

	var missing, extra []string
	for _, name := range bound {
		if _, found := slices.BinarySearch(frontend, name); !found {
			missing = append(missing, name)
		}
	}
	for _, name := range frontend {
		if _, found := slices.BinarySearch(bound, name); !found {
			extra = append(extra, name)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("APP_METHODS out of step with *App:\n bound but not in frontend: %s\n in frontend but not bound: %s",
			strings.Join(missing, ", "), strings.Join(extra, ", "))
	}
}

// TestRouterIsTheOnlyBinding keeps the bound Router down to its one Call
// method, and the frontend's ROUTER_CALL naming it.
func TestRouterIsTheOnlyBinding(t *testing.T) {
	typ := reflect.TypeFor[*Router]()
	var methods []string
	for i := range typ.NumMethod() {
		methods = append(methods, typ.Method(i).Name)
	}
	if !slices.Equal(methods, []string{"Call"}) {
		t.Fatalf("*Router methods = %v, want only Call: Wails binds every one", methods)
	}

	src, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "desktopRuntime.ts"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`export const ROUTER_CALL = '([^']+)'`).FindSubmatch(src)
	elem := typ.Elem()
	want := elem.PkgPath() + "." + elem.Name() + ".Call"
	if m == nil || string(m[1]) != want {
		t.Fatalf("ROUTER_CALL in desktopRuntime.ts = %q, want %q", m, want)
	}
}
