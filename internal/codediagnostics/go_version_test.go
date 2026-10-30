package codediagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGoVersion(t *testing.T, goroot, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(goroot, "VERSION"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGoTooOld(t *testing.T) {
	goroot := t.TempDir()
	writeGoVersion(t, goroot, "go1.26.8\ntime 2026-09-01T00:00:00Z\n")
	for _, tc := range []struct {
		name     string
		reqs     []goRequirement
		affected int
		notice   string
	}{
		{"older patch", []goRequirement{{"", "1.26.2"}}, 0, ""},
		{"same", []goRequirement{{"", "1.26.8"}}, 0, ""},
		{"language only", []goRequirement{{"", "1.26"}}, 0, ""},
		{"newer patch", []goRequirement{{"", "1.26.10"}}, 1, "needs Go 1.26.10 (go.mod), but the Go found is 1.26.8 (" + goroot + ")"},
		{"newer minor", []goRequirement{{"tools", "1.27"}}, 1, "needs Go 1.27 (tools/go.mod)"},
		{"release candidate", []goRequirement{{"", "1.27rc1"}}, 1, "Install Go 1.27rc1 or newer"},
		{"newest named", []goRequirement{{"", "1.26.9"}, {"a", "1.26"}, {"b", "1.27.1"}, {"c", "1.26.10"}}, 3, "needs Go 1.27.1 (b/go.mod and 2 more modules)"},
		{"invalid directive", []goRequirement{{"", "banana"}}, 0, ""},
	} {
		notice, affected := goTooOld(goroot, tc.reqs, "for go vet")
		if affected != tc.affected || !strings.Contains(notice, tc.notice) || (tc.notice == "") != (notice == "") {
			t.Errorf("%s: %d %q", tc.name, affected, notice)
		}
	}
	// A toolchain that is not a release is never reported.
	for _, content := range []string{"", "devel go1.27-abcdef\n", "synthetic"} {
		writeGoVersion(t, goroot, content)
		if notice, affected := goTooOld(goroot, []goRequirement{{"", "1.99"}}, "for go vet"); affected != 0 || notice != "" {
			t.Errorf("%q: %d %q", content, affected, notice)
		}
	}
	if _, affected := goTooOld(filepath.Join(goroot, "missing"), []goRequirement{{"", "1.99"}}, "for go vet"); affected != 0 {
		t.Error("missing VERSION reported")
	}
}

// Go refuses a module needing a newer Go offline, so gopls says why instead
// of starting without packages.
func TestDiscoverGoplsNeedsNewerGo(t *testing.T) {
	skipUnlessProduction(t)
	bin, env := goplsHost(t, "v0.24.0", goplsModulePath)
	writeGoVersion(t, env.GoRoot, "go1.26.8\n")

	root := goplsRoot(t, map[string]string{"go.mod": "module example.test/a\n\ngo 1.26.10\n", "a.go": "package a"})
	p, err := DiscoverGopls(root, []string{bin}, env)
	if err != nil || p.Availability() != "unsupported" {
		t.Fatalf("%v %+v", err, p)
	}
	if _, ok := p.Server(); ok {
		t.Fatal("server offered for a module Go refuses")
	}
	if st := p.Provider(); !strings.Contains(st.Notice, "needs Go 1.26.10 (go.mod), but the Go found is 1.26.8") || !strings.Contains(st.Notice, "Go to definition") {
		t.Fatalf("%+v", st)
	}

	// Only some modules: the rest keep their server, and coverage is partial.
	mixed := goplsRoot(t, map[string]string{"go.mod": "module example.test/a\n\ngo 1.26\n", "tools/go.mod": "module example.test/tools\n\ngo 1.27\n"})
	p, err = DiscoverGopls(mixed, []string{bin}, env)
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%v %+v", err, p)
	}
	if st := p.Provider(); !st.Coverage.Partial || !strings.Contains(st.Notice, "needs Go 1.27 (tools/go.mod)") {
		t.Fatalf("%+v", st)
	}

	// A new enough toolchain is unaffected.
	writeGoVersion(t, env.GoRoot, "go1.27.0\n")
	if p, err = DiscoverGopls(root, []string{bin}, env); err != nil || p.Availability() != "available" || p.Provider().Coverage.Partial {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestDiscoverGoVetNeedsNewerGo(t *testing.T) {
	skipUnlessProduction(t)
	bin := fakeGoInstallation(t)
	writeGoVersion(t, filepath.Dir(bin), "go1.26.8\n")
	root := vetRoot(t, map[string]string{"go.mod": "module synthetic\n\ngo 1.26.10\n"})
	cache := t.TempDir()
	_, err := DiscoverGoVet(root, []string{bin}, GoVetEnvironment{cache, cache, cache})
	code(t, err, "toolchainTooOld")
	if !strings.Contains(err.Error(), "needs Go 1.26.10 (go.mod)") {
		t.Fatal(err)
	}
	writeGoVersion(t, filepath.Dir(bin), "go1.26.10\n")
	if _, err := DiscoverGoVet(root, []string{bin}, GoVetEnvironment{cache, cache, cache}); err != nil {
		t.Fatal(err)
	}
}
