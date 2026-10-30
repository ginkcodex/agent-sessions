package codediagnostics

import (
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

// Facts are injected: nothing here depends on the developer's host, GOROOT or
// environment.
func testGoFacts(goos, goarch string, minor int) goBuildFacts {
	return goBuildFacts{goos: goos, goarch: goarch, compiler: "gc", releaseMinor: minor, releaseKnown: minor > 0}
}

var darwinFacts = testGoFacts("darwin", "arm64", 26)

func goHeader(expr string) []byte { return []byte("//go:build " + expr + "\n\npackage p\n") }

func TestInferGoBuildTagsExpressions(t *testing.T) {
	type want struct {
		tags            []string
		known, admitted bool
	}
	admit := func(tags ...string) want { return want{tags, true, true} }
	excluded := want{nil, true, false}
	unknown := want{}
	cases := []struct {
		name, expr string
		want       want
	}{
		{"reported expression", "(linux || darwin) && (webkit2_41 || desktop)", admit("desktop")},
		{"host os", "darwin", admit()},
		{"other os", "linux", excluded},
		{"not other os", "!windows", admit()},
		{"unix on darwin", "unix", admit()},
		{"not unix", "!unix", excluded},
		{"arch", "arm64", admit()},
		{"other arch", "amd64", excluded},
		{"compiler gc", "gc", admit()},
		{"compiler gccgo", "gccgo", excluded},
		{"ignore", "ignore", excluded},
		{"single custom", "desktop", admit("desktop")},
		{"negated custom", "!desktop", admit()},
		{"and sorted", "zeta && alpha", admit("alpha", "zeta")},
		{"or picks smallest lexicographic", "zeta || alpha", admit("alpha")},
		{"fewest tags", "(a && b) || c", admit("c")},
		{"lexicographic tie", "(b && c) || (a && d)", admit("a", "d")},
		{"mutually exclusive", "a && !a", excluded},
		{"negation needed off", "!a && b", admit("b")},
		{"tautology", "a || !a", admit()},
		{"release satisfied", "go1.18", admit()},
		{"release too new", "go1.99", excluded},
		{"release combined", "go1.20 && desktop", admit("desktop")},
		{"other arch feature", "amd64.v3", excluded},
		{"cgo alone", "cgo", unknown},
		{"cgo and custom", "cgo && desktop", unknown},
		{"not cgo and custom", "!cgo && desktop", unknown},
		{"cgo or custom", "cgo || desktop", admit("desktop")},
		{"cgo tautology", "cgo || !cgo", admit()},
		{"not cgo", "!cgo", unknown},
		{"cgo contradiction", "cgo && !cgo", excluded},
		{"race", "race", unknown},
		{"race or custom", "race || desktop", admit("desktop")},
		{"experiment", "goexperiment.boringcrypto", unknown},
		{"boringcrypto alias", "boringcrypto", unknown},
		{"msan asan", "msan && asan", unknown},
		{"host arch feature", "arm64.v8.0", unknown},
		{"custom dot tag", "my.tag", admit("my.tag")},
		{"six custom", "a && b && c && d && e && f", admit("a", "b", "c", "d", "e", "f")},
		{"seven custom", "a && b && c && d && e && f && g", unknown},
		{"too many unknown", "cgo && race && msan && asan && goexperiment.x", unknown},
		{"overlong tag", strings.Repeat("t", maxGoBuildTagBytes+1), unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inferGoBuildTags("x.go", goHeader(tc.expr), darwinFacts)
			if got.known != tc.want.known || got.admitted != tc.want.admitted || !slices.Equal(got.tags, tc.want.tags) {
				t.Fatalf("%q = %+v, want %+v", tc.expr, got, tc.want)
			}
			if !got.known && (got.admitted || got.tags != nil) {
				t.Fatalf("unknown selection must carry no tags: %+v", got)
			}
		})
	}
}

func TestInferGoBuildTagsUnknownReleaseAndFacts(t *testing.T) {
	noRelease := testGoFacts("darwin", "arm64", 0)
	if got := inferGoBuildTags("x.go", goHeader("go1.18"), noRelease); got.known {
		t.Fatalf("unknown release must not be decided: %+v", got)
	}
	if got := inferGoBuildTags("x.go", goHeader("go1.18 || desktop"), noRelease); !got.known || !got.admitted || !slices.Equal(got.tags, []string{"desktop"}) {
		t.Fatalf("independent solution must survive an unknown release: %+v", got)
	}
	if got := inferGoBuildTags("x.go", goHeader("desktop"), goBuildFacts{}); got.known || got.admitted || got.tags != nil {
		t.Fatalf("zero facts are never trusted: %+v", got)
	}
	if got := inferGoBuildTags("x.go", goHeader("go1.2x"), darwinFacts); got.known != true || got.admitted {
		t.Fatalf("non-release go1.* tag is false: %+v", got)
	}
}

func TestInferGoBuildTagsOtherHosts(t *testing.T) {
	cases := []struct {
		facts goBuildFacts
		expr  string
		ok    bool
	}{
		{testGoFacts("windows", "amd64", 26), "windows && amd64", true},
		{testGoFacts("windows", "amd64", 26), "unix", false},
		{testGoFacts("linux", "amd64", 26), "unix && linux", true},
		{testGoFacts("android", "arm64", 26), "linux", true},
		{testGoFacts("illumos", "amd64", 26), "solaris", true},
		{testGoFacts("ios", "arm64", 26), "darwin", true},
		{testGoFacts("darwin", "arm64", 26), "ios", false},
		{testGoFacts("js", "wasm", 26), "js && wasm", true},
		{testGoFacts("js", "wasm", 26), "unix", false},
	}
	for _, tc := range cases {
		got := inferGoBuildTags("x.go", goHeader(tc.expr), tc.facts)
		if !got.known || got.admitted != tc.ok || got.tags != nil {
			t.Errorf("%s/%s %q = %+v, want admitted=%v", tc.facts.goos, tc.facts.goarch, tc.expr, got, tc.ok)
		}
	}
}

func TestInferGoBuildTagsHeaderForms(t *testing.T) {
	cases := []struct {
		name, src       string
		tags            []string
		known, admitted bool
	}{
		{"unconstrained", "package p\n", nil, true, true},
		{"no trailing newline", "package p", nil, true, true},
		{"crlf", "//go:build desktop\r\n\r\npackage p\r\n", []string{"desktop"}, true, true},
		{"doc comment before package", "// Package p docs.\npackage p\n", nil, true, true},
		{"go:build after license", "// Copyright\n\n//go:build desktop\n\npackage p\n", []string{"desktop"}, true, true},
		{"go:build must precede blank-separated package", "//go:build desktop\npackage p\n", []string{"desktop"}, true, true},
		{"go:build after package ignored", "package p\n\n//go:build linux\n", nil, true, true},
		{"block comment before go:build", "/* c */\n//go:build desktop\n\npackage p\n", []string{"desktop"}, true, true},
		{"go:build inside block comment ignored", "/*\n//go:build linux\n*/\n\npackage p\n", nil, true, true},
		{"package after block comment", "//go:build desktop\n\n/* c */ package p\n", []string{"desktop"}, true, true},
		{"semicolon", "//go:build desktop\n\npackage p; import \"os\"\n", []string{"desktop"}, true, true},
		{"unicode package", "package pä\n", nil, true, true},
		{"legacy excluded", "// +build linux\n\npackage p\n", nil, true, false},
		{"legacy or", "// +build linux darwin\n\npackage p\n", nil, true, true},
		{"legacy and", "// +build darwin,desktop\n\npackage p\n", []string{"desktop"}, true, true},
		{"legacy not", "// +build !windows,!desktop\n\npackage p\n", nil, true, true},
		{"legacy lines are anded", "// +build darwin\n// +build desktop\n\npackage p\n", []string{"desktop"}, true, true},
		{"legacy lines anded excluded", "// +build darwin\n// +build linux\n\npackage p\n", nil, true, false},
		{"legacy ignore", "// +build ignore\n\npackage p\n", nil, true, false},
		{"legacy needs blank line", "// +build linux\npackage p\n", nil, true, true},
		{"legacy after blank line before package", "// +build linux\n\n// doc\npackage p\n", nil, true, false},
		{"go:build wins over legacy", "//go:build desktop\n// +build linux\n\npackage p\n", []string{"desktop"}, true, true},
		{"mismatched go:build and legacy", "//go:build darwin\n// +build windows\n\npackage p\n", nil, true, true},
		{"cgo unknown", "//go:build cgo && desktop\n\npackage p\n", nil, false, false},
		{"malformed operator", "//go:build desktop &&\n\npackage p\n", nil, false, false},
		{"malformed parens", "//go:build (desktop\n\npackage p\n", nil, false, false},
		{"malformed single and", "//go:build a & b\n\npackage p\n", nil, false, false},
		{"empty go:build", "//go:build\n\npackage p\n", nil, false, false},
		{"multiple go:build", "//go:build a\n//go:build b\n\npackage p\n", nil, false, false},
		{"not a package clause", "//go:build desktop\n\nimport \"os\"\n", nil, false, false},
		{"missing package name", "//go:build desktop\n\npackage\n", nil, false, false},
		{"blank package name", "package _\n", nil, false, false},
		{"keyword package name", "package func\n", nil, false, false},
		{"trailing text after package", "package p q\n", nil, false, false},
		{"truncated before package", "//go:build desktop\n", nil, false, false},
		{"truncated mid keyword", "//go:build desktop\n\npac", nil, false, false},
		{"truncated after keyword", "//go:build desktop\n\npackage", nil, false, false},
		{"unterminated block comment", "//go:build desktop\n\n/* open\npackage p\n", nil, false, false},
		{"empty file", "", nil, false, false},
		{"binary garbage", "\x00\x01\x02", nil, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inferGoBuildTags("x.go", []byte(tc.src), darwinFacts)
			if got.known != tc.known || got.admitted != tc.admitted || !slices.Equal(got.tags, tc.tags) {
				t.Fatalf("%q = %+v, want tags=%v known=%v admitted=%v", tc.src, got, tc.tags, tc.known, tc.admitted)
			}
		})
	}
}

func TestParseGoBuildHeaderDistinguishesUnconstrainedFromErrors(t *testing.T) {
	expr, err := parseGoBuildHeader([]byte("package p\n"))
	if err != nil || expr != nil {
		t.Fatalf("unconstrained = (%v, %v), want (nil, nil)", expr, err)
	}
	expr, err = parseGoBuildHeader(goHeader("a && b"))
	if err != nil || expr == nil || expr.String() != "a && b" {
		t.Fatalf("modern = (%v, %v)", expr, err)
	}
	if _, err = parseGoBuildHeader([]byte("//go:build a\n")); !errors.Is(err, errGoBuildTruncated) {
		t.Fatalf("truncated error = %v", err)
	}
	if _, err = parseGoBuildHeader([]byte("//go:build a &&\n\npackage p\n")); !errors.Is(err, errGoBuildMalformed) {
		t.Fatalf("malformed error = %v", err)
	}
	if _, err = parseGoBuildHeader([]byte("//go:build a\n//go:build b\n\npackage p\n")); !errors.Is(err, errGoBuildMalformed) {
		t.Fatalf("duplicate error = %v", err)
	}
	// Legacy lines are ANDed; precedence is exactly go/build's (modern wins).
	legacy, err := parseGoBuildHeader([]byte("// +build darwin\n// +build desktop\n\npackage p\n"))
	if err != nil || legacy.String() != "darwin && desktop" {
		t.Fatalf("legacy = (%v, %v)", legacy, err)
	}
	modern, err := parseGoBuildHeader([]byte("//go:build desktop\n// +build linux\n\npackage p\n"))
	if err != nil || modern.String() != "desktop" {
		t.Fatalf("modern precedence = (%v, %v)", modern, err)
	}
}

func TestInferGoBuildTagsFilenames(t *testing.T) {
	cases := []struct {
		facts    goBuildFacts
		name     string
		admitted bool
	}{
		{darwinFacts, "x.go", true},
		{darwinFacts, "x_darwin.go", true},
		{darwinFacts, "x_linux.go", false},
		{darwinFacts, "x_arm64.go", true},
		{darwinFacts, "x_amd64.go", false},
		{darwinFacts, "x_darwin_arm64.go", true},
		{darwinFacts, "x_darwin_amd64.go", false},
		{darwinFacts, "x_linux_arm64.go", false},
		{darwinFacts, "x_linux_test.go", false},
		{darwinFacts, "x_darwin_test.go", true},
		{darwinFacts, "x_darwin_arm64_test.go", true},
		{darwinFacts, "x_ios.go", false},
		{darwinFacts, "linux.go", true},
		{darwinFacts, "x_linux_extra.go", true},
		{darwinFacts, "x_unix.go", true},
		{darwinFacts, "x_cgo.go", true},
		{darwinFacts, "a_b_windows.go", false},
		{darwinFacts, "x_darwin.pb.go", true},
		{darwinFacts, "x_windows.pb.go", false},
		{testGoFacts("ios", "arm64", 26), "x_darwin.go", true},
		{testGoFacts("ios", "arm64", 26), "x_ios.go", true},
		{testGoFacts("android", "arm64", 26), "x_linux.go", true},
		{testGoFacts("android", "arm64", 26), "x_linux_arm64.go", true},
		{testGoFacts("illumos", "amd64", 26), "x_solaris.go", true},
		{testGoFacts("linux", "amd64", 26), "x_linux_amd64_test.go", true},
		{darwinFacts, "sub/dir/x_linux.go", false},
		{darwinFacts, "_x.go", false},
		{darwinFacts, ".x.go", false},
		{darwinFacts, "_test.go", false},
		{darwinFacts, "x.txt", false},
	}
	for _, tc := range cases {
		got := inferGoBuildTags(tc.name, []byte("package p\n"), tc.facts)
		if !got.known || got.admitted != tc.admitted || got.tags != nil {
			t.Errorf("%s on %s/%s = %+v, want admitted=%v", tc.name, tc.facts.goos, tc.facts.goarch, got, tc.admitted)
		}
	}
	// The filename decides before the header, even when the header is invalid.
	if got := inferGoBuildTags("x_linux.go", []byte("garbage"), darwinFacts); !got.known || got.admitted {
		t.Fatalf("filename exclusion must not need a valid header: %+v", got)
	}
	// A matching filename leaves the header in charge.
	if got := inferGoBuildTags("x_darwin.go", goHeader("desktop"), darwinFacts); !got.known || !got.admitted || !slices.Equal(got.tags, []string{"desktop"}) {
		t.Fatalf("matching filename plus header = %+v", got)
	}
	if got := inferGoBuildTags("x_linux.go", goHeader("desktop"), darwinFacts); !got.known || got.admitted || got.tags != nil {
		t.Fatalf("custom tags cannot rescue a filename exclusion: %+v", got)
	}
}

func TestInferGoBuildTagsDeterministicAndCanonical(t *testing.T) {
	for _, expr := range []string{"zeta && alpha && mid", "mid && zeta && alpha", "(alpha && mid) && zeta && alpha"} {
		first := inferGoBuildTags("x.go", goHeader(expr), darwinFacts)
		if !slices.IsSorted(first.tags) || len(slices.Compact(slices.Clone(first.tags))) != len(first.tags) {
			t.Fatalf("%q tags not canonical: %v", expr, first.tags)
		}
		for range 50 {
			if got := inferGoBuildTags("x.go", goHeader(expr), darwinFacts); !slices.Equal(got.tags, first.tags) || got.known != first.known || got.admitted != first.admitted {
				t.Fatalf("%q varied between runs: %v vs %v", expr, got.tags, first.tags)
			}
		}
		if !slices.Equal(first.tags, []string{"alpha", "mid", "zeta"}) {
			t.Fatalf("%q = %v", expr, first.tags)
		}
	}
}

func TestInferGoBuildTagsNeverEmitsReservedNames(t *testing.T) {
	reserved := []string{"windows", "linux", "arm64", "amd64", "cgo", "race", "msan", "asan", "gc", "gccgo", "ignore", "unix", "go1.1", "go1.99", "goexperiment.x", "boringcrypto", "amd64.v1", "arm64.v8.0"}
	for _, tag := range reserved {
		for _, expr := range []string{tag, "!" + tag, tag + " || desktop", "!" + tag + " && desktop"} {
			got := inferGoBuildTags("x.go", goHeader(expr), darwinFacts)
			for _, emitted := range got.tags {
				if emitted != "desktop" {
					t.Errorf("%q emitted reserved tag %q", expr, emitted)
				}
			}
		}
	}
}

func TestInferGoBuildTagsBounds(t *testing.T) {
	huge := strings.Repeat("a || ", 300) + "a"
	if got := inferGoBuildTags("x.go", goHeader(huge), darwinFacts); got.known {
		t.Fatalf("node budget not enforced: %+v", got)
	}
	if _, err := parseGoBuildHeader(goHeader(huge)); !errors.Is(err, errGoBuildTooComplex) {
		t.Fatalf("node budget error = %v", err)
	}
	nest := func(n int) string { return strings.Repeat("a || (", n) + "a" + strings.Repeat(")", n) }
	deep := nest(maxGoBuildExprDepth + 1)
	if _, err := parseGoBuildHeader(goHeader(deep)); !errors.Is(err, errGoBuildTooComplex) {
		t.Fatalf("depth budget error = %v", err)
	}
	if _, err := parseGoBuildHeader(goHeader(nest(maxGoBuildExprDepth - 2))); err != nil {
		t.Fatalf("expression within the depth budget rejected: %v", err)
	}
	long := "//go:build a" + strings.Repeat(" ", maxGoBuildLineBytes) + "|| b\n\npackage p\n"
	if got := inferGoBuildTags("x.go", []byte(long), darwinFacts); got.known {
		t.Fatalf("line budget not enforced: %+v", got)
	}
	oldSyntax := "// +build " + strings.Repeat("a ", maxGoBuildLineBytes) + "\n\npackage p\n"
	if got := inferGoBuildTags("x.go", []byte(oldSyntax), darwinFacts); got.known {
		t.Fatalf("legacy line budget not enforced: %+v", got)
	}
	// Six custom atoms are the maximum search space: 2^6 subsets, never more.
	var atoms []string
	for i := range maxGoBuildCustomAtoms + 1 {
		atoms = append(atoms, fmt.Sprintf("t%d", i))
	}
	if got := inferGoBuildTags("x.go", goHeader(strings.Join(atoms, " || ")), darwinFacts); got.known {
		t.Fatalf("custom atom budget not enforced: %+v", got)
	}
}

func TestParseGoBuildHeaderBudget(t *testing.T) {
	filler := bytes.Repeat([]byte("// padding line\n"), (maxGoBuildHeaderBytes/16)+10)
	src := append(append([]byte("//go:build desktop\n\n"), filler...), []byte("\npackage p\n")...)
	if _, err := parseGoBuildHeader(src); !errors.Is(err, errGoBuildTruncated) {
		t.Fatalf("package clause past the budget = %v, want truncated", err)
	}
	// Exactly inside the budget is fine.
	room := maxGoBuildHeaderBytes - len("//go:build desktop\n\n") - len("package p\n")
	pad := bytes.Repeat([]byte("//"+strings.Repeat("x", 62)+"\n"), room/65)
	ok := append(append([]byte("//go:build desktop\n\n"), pad...), []byte("package p\n")...)
	if len(ok) > maxGoBuildHeaderBytes {
		t.Fatalf("fixture %d too large", len(ok))
	}
	if expr, err := parseGoBuildHeader(ok); err != nil || expr == nil {
		t.Fatalf("header inside budget = (%v, %v)", expr, err)
	}
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestReadGoBuildPrefix(t *testing.T) {
	body := strings.Repeat("var x = 1\n", 100000)
	src := "//go:build desktop\n\npackage p\n" + body
	prefix, err := readGoBuildPrefix(strings.NewReader(src))
	if err != nil || string(prefix) != "//go:build desktop\n\npackage p\n" {
		t.Fatalf("prefix = %q, %v", prefix, err)
	}
	if expr, err := parseGoBuildHeader(prefix); err != nil || expr == nil || expr.String() != "desktop" {
		t.Fatalf("prefix does not parse: %v %v", expr, err)
	}
	expected := inferGoBuildTags("x.go", prefix, darwinFacts)
	if full := inferGoBuildTags("x.go", []byte(src), darwinFacts); !slices.Equal(full.tags, expected.tags) {
		t.Fatalf("prefix and full source disagree: %+v vs %+v", expected, full)
	}

	// Read cap with a never-ending comment header.
	cr := &countingReader{r: io.MultiReader(strings.NewReader("//go:build desktop\n\n"), neverEnding("// c\n"))}
	if _, err = readGoBuildPrefix(cr); !errors.Is(err, errGoBuildTruncated) {
		t.Fatalf("endless header = %v", err)
	}
	if cr.n > maxGoBuildHeaderBytes {
		t.Fatalf("read %d bytes, budget %d", cr.n, maxGoBuildHeaderBytes)
	}

	for name, in := range map[string]string{
		"no package":   "//go:build desktop\n\n// only comments\n",
		"empty":        "",
		"open comment": "/* never closed\n",
	} {
		if _, err := readGoBuildPrefix(strings.NewReader(in)); !errors.Is(err, errGoBuildTruncated) {
			t.Errorf("%s = %v, want truncated", name, err)
		}
	}
	for name, in := range map[string]string{
		"import first": "import \"os\"\n",
		"bad go:build": "//go:build a &&\n\npackage p\n",
		"bad package":  "package _\n",
		"duplicate":    "//go:build a\n//go:build b\n\npackage p\n",
	} {
		if _, err := readGoBuildPrefix(strings.NewReader(in)); err == nil || errors.Is(err, errGoBuildTruncated) {
			t.Errorf("%s = %v, want a malformed (non-truncated) error", name, err)
		}
	}
	// A package line with no newline at EOF is complete; the same line mid-stream is not.
	if prefix, err = readGoBuildPrefix(strings.NewReader("package p")); err != nil || string(prefix) != "package p" {
		t.Fatalf("EOF package line = %q, %v", prefix, err)
	}

	boom := errors.New("boom")
	if _, err = readGoBuildPrefix(io.MultiReader(strings.NewReader("//go:build a\n"), iotest.ErrReader(boom))); !errors.Is(err, boom) {
		t.Fatalf("reader error = %v", err)
	}
}

type neverEnding string

func (n neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = n[i%len(n)]
	}
	return len(p), nil
}

func TestReadGoBuildPrefixIsChunkIndependent(t *testing.T) {
	// Put the package keyword across the internal read-chunk boundary at every offset.
	for pad := goBuildReadChunk - 12; pad <= goBuildReadChunk+12; pad++ {
		head := "//go:build desktop\n\n" + strings.Repeat("/", pad-len("//go:build desktop\n\n")-1) + "\n"
		src := head + "package pa\nvar x = 1\n"
		want, werr := readGoBuildPrefix(strings.NewReader(src))
		got, gerr := readGoBuildPrefix(iotest.OneByteReader(strings.NewReader(src)))
		half, herr := readGoBuildPrefix(iotest.HalfReader(strings.NewReader(src)))
		if werr != nil || gerr != nil || herr != nil || !bytes.Equal(want, got) || !bytes.Equal(want, half) || !strings.HasSuffix(string(want), "package pa\n") {
			t.Fatalf("pad %d: %q/%v %q/%v %q/%v", pad, want, werr, got, gerr, half, herr)
		}
	}
}

func TestReviewedGoBuildFacts(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		if content != "" {
			if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	cases := []struct {
		name, version string
		minor         int
		known         bool
	}{
		{"release", "go1.26.8\ntime 2026-08-28T16:20:06Z\n", 26, true},
		{"minor only", "go1.9", 9, true},
		{"rc", "go1.27rc1\n", 27, true},
		{"beta", "go1.27beta2\n", 27, true},
		{"devel", "devel go1.28-abcdef Mon Jan 1\n", 0, false},
		{"experiment suffix", "go1.26.8 X:boringcrypto\n", 26, true},
		{"leading zero", "go1.026\n", 0, false},
		{"not a number", "go1.x\n", 0, false},
		{"go2", "go2.0\n", 0, false},
		{"missing", "", 0, false},
	}
	for _, tc := range cases {
		f := reviewedGoBuildFacts(write(t, tc.version))
		if f.releaseMinor != tc.minor || f.releaseKnown != tc.known || f.compiler != "gc" || f.goos == "" || f.goarch == "" {
			t.Errorf("%s = %+v", tc.name, f)
		}
	}
	if f := reviewedGoBuildFacts(""); f.releaseKnown || f.compiler != "gc" {
		t.Fatalf("empty goroot = %+v", f)
	}
	if f := reviewedGoBuildFacts(filepath.Join(t.TempDir(), "missing")); f.releaseKnown {
		t.Fatalf("missing goroot = %+v", f)
	}
	// An oversized VERSION is only ever read up to its bound and still cannot invent a release.
	if f := reviewedGoBuildFacts(write(t, strings.Repeat("x", 1<<20))); f.releaseKnown {
		t.Fatalf("garbage VERSION = %+v", f)
	}
	// A symlinked VERSION is not reviewed metadata.
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "VERSION")
	if err := os.WriteFile(target, []byte("go1.26.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "VERSION")); err == nil {
		if f := reviewedGoBuildFacts(dir); f.releaseKnown {
			t.Fatalf("symlinked VERSION trusted: %+v", f)
		}
	}
}

// ---- cross-check against go/build's own matching ------------------------------

type matchFileCase struct {
	name, src string
}

func matchFileContext(facts goBuildFacts, files map[string]string, tags []string, cgo bool) *build.Context {
	var release []string
	for i := 1; i <= facts.releaseMinor; i++ {
		release = append(release, fmt.Sprintf("go1.%d", i))
	}
	return &build.Context{
		GOOS: facts.goos, GOARCH: facts.goarch, Compiler: facts.compiler, CgoEnabled: cgo,
		BuildTags: tags, ReleaseTags: release,
		JoinPath: func(elem ...string) string { return strings.Join(elem, "/") },
		OpenFile: func(path string) (io.ReadCloser, error) {
			if src, ok := files[path[strings.LastIndex(path, "/")+1:]]; ok {
				return io.NopCloser(strings.NewReader(src)), nil
			}
			return nil, os.ErrNotExist
		},
	}
}

func TestInferGoBuildTagsAgreesWithGoBuildMatchFile(t *testing.T) {
	cases := []matchFileCase{
		{"plain.go", "package p\n"},
		{"reported.go", string(goHeader("(linux || darwin) && (webkit2_41 || desktop)"))},
		{"linux_only.go", string(goHeader("linux"))},
		{"not_windows.go", string(goHeader("!windows && !plan9"))},
		{"and.go", string(goHeader("alpha && beta"))},
		{"or.go", string(goHeader("alpha || beta"))},
		{"nested.go", string(goHeader("(alpha && !beta) || (gamma && darwin && !linux)"))},
		{"negated.go", string(goHeader("!alpha && beta"))},
		{"contradiction.go", string(goHeader("alpha && !alpha"))},
		{"release_old.go", string(goHeader("go1.10 && alpha"))},
		{"release_new.go", string(goHeader("go1.99 || alpha"))},
		{"unix.go", string(goHeader("unix && alpha"))},
		{"gc.go", string(goHeader("gc && !gccgo && alpha"))},
		{"cgo_or.go", string(goHeader("cgo || alpha"))},
		{"cgo_and.go", string(goHeader("cgo && alpha"))},
		{"not_cgo.go", string(goHeader("!cgo"))},
		{"cgo_taut.go", string(goHeader("cgo || !cgo"))},
		{"legacy_or.go", "// +build linux darwin\n\npackage p\n"},
		{"legacy_and.go", "// +build darwin,alpha\n\npackage p\n"},
		{"legacy_two.go", "// +build darwin\n// +build alpha beta\n\npackage p\n"},
		{"legacy_ignore.go", "// +build ignore\n\npackage p\n"},
		{"legacy_noblank.go", "// +build linux\npackage p\n"},
		{"both.go", "//go:build alpha\n// +build linux\n\npackage p\n"},
		{"file_darwin.go", string(goHeader("alpha"))},
		{"file_linux.go", string(goHeader("alpha"))},
		{"file_darwin_arm64.go", "package p\n"},
		{"file_darwin_amd64.go", "package p\n"},
		{"file_linux_test.go", "package p\n"},
		{"file_ios.go", "package p\n"},
		{"file_windows_arm64_test.go", "package p\n"},
		{"file_unix.go", "package p\n"},
		{"_hidden.go", "package p\n"},
		{".hidden.go", "package p\n"},
	}
	files := map[string]string{}
	for _, c := range cases {
		files[c.name] = c.src
	}
	hosts := []goBuildFacts{darwinFacts, testGoFacts("linux", "amd64", 26), testGoFacts("ios", "arm64", 22), testGoFacts("android", "arm64", 26), testGoFacts("windows", "amd64", 26)}
	for _, facts := range hosts {
		for _, c := range cases {
			sel := inferGoBuildTags(c.name, []byte(c.src), facts)
			matches := func(tags []string, cgo bool) bool {
				ok, err := matchFileContext(facts, files, tags, cgo).MatchFile("dir", c.name)
				if err != nil {
					t.Fatalf("%s: MatchFile: %v", c.name, err)
				}
				return ok
			}
			label := fmt.Sprintf("%s/%s %s", facts.goos, facts.goarch, c.name)
			switch {
			case sel.known && sel.admitted:
				// With the chosen tags the file builds whichever way the unknown facts fall.
				for _, cgo := range []bool{false, true} {
					if !matches(sel.tags, cgo) {
						t.Errorf("%s: tags %v do not admit the file (cgo=%v)", label, sel.tags, cgo)
					}
				}
				// ...and they are needed: the default context does not already admit it.
				if len(sel.tags) > 0 && matches(nil, false) && matches(nil, true) {
					t.Errorf("%s: tags %v chosen although the default context admits the file", label, sel.tags)
				}
				// Minimality: dropping any tag breaks it for some cgo value.
				for i := range sel.tags {
					fewer := slices.Delete(slices.Clone(sel.tags), i, i+1)
					if matches(fewer, false) && matches(fewer, true) {
						t.Errorf("%s: tag %q is redundant in %v", label, sel.tags[i], sel.tags)
					}
				}
				if len(sel.tags) == 0 && !(matches(nil, false) && matches(nil, true)) {
					t.Errorf("%s: default-admitted but go/build disagrees", label)
				}
			case sel.known:
				// Provably excluded: even every custom tag cannot admit it.
				all := []string{"alpha", "beta", "gamma", "desktop", "webkit2_41"}
				for _, cgo := range []bool{false, true} {
					if matches(nil, cgo) || matches(all, cgo) {
						t.Errorf("%s: judged excluded but go/build builds it (cgo=%v)", label, cgo)
					}
				}
			default:
				// Dependent on an unknown fact (or unparsable): the default must not be both
				// trusted-admitted and trusted-excluded, i.e. go/build differs across cgo.
				if c.name[0] != '_' && c.name[0] != '.' && !strings.HasPrefix(c.name, "cgo") && !strings.HasPrefix(c.name, "not_cgo") {
					t.Errorf("%s: unexpectedly undecided %+v", label, sel)
				}
				differs := false
				for _, tags := range [][]string{nil, {"alpha"}} {
					if matches(tags, false) != matches(tags, true) {
						differs = true
					}
				}
				if !differs {
					t.Errorf("%s: undecided but go/build is independent of cgo", label)
				}
			}
		}
	}
}
