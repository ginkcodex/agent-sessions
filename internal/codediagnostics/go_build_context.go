package codediagnostics

import (
	"bytes"
	"errors"
	"fmt"
	"go/build/constraint"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Pure, bounded derivation of Go build-tag contexts from source headers. Nothing
// here executes a tool, touches the network or reads anything but the bytes it
// is handed (reviewedGoBuildFacts reads only the reviewed GOROOT's VERSION).

const (
	// maxGoBuildHeaderBytes is the per-file header budget. A header whose
	// package clause is not reached within it is reported as truncated.
	maxGoBuildHeaderBytes = 16 << 10
	// maxGoBuildLineBytes bounds a single constraint line before it is parsed.
	maxGoBuildLineBytes = 2048
	// maxGoBuildExprNodes and maxGoBuildExprDepth bound the parsed expression.
	maxGoBuildExprNodes = 256
	maxGoBuildExprDepth = 32
	// maxGoBuildCustomAtoms bounds distinct custom tags the solver searches (2^6
	// subsets); maxGoBuildUnknownAtoms bounds the reserved facts it quantifies
	// over (2^4 assignments).
	maxGoBuildCustomAtoms  = 6
	maxGoBuildUnknownAtoms = 4
	maxGoBuildTagBytes     = 64
	goBuildReadChunk       = 2048
	maxGoVersionFileBytes  = 1024
)

var (
	// errGoBuildTruncated: the package clause was not reached (header budget,
	// end of data or an unterminated comment). Never a valid unconstrained header.
	errGoBuildTruncated = errors.New("go build header: package clause not reached")
	// errGoBuildMalformed: invalid, ambiguous or non-Go header.
	errGoBuildMalformed = errors.New("go build header: malformed")
	// errGoBuildTooComplex: expression exceeds the size or depth budget.
	errGoBuildTooComplex = errors.New("go build header: expression too complex")
)

// goBuildFacts are the fixed facts the host context is analysed under. Callers
// obtain them from reviewedGoBuildFacts; the zero value is never trusted.
// cgo, race, msan, asan, GOEXPERIMENT and architecture-feature tags are not
// represented: they are always unknown and never invented.
type goBuildFacts struct {
	goos, goarch, compiler string
	releaseMinor           int  // N of the reviewed toolchain's go1.N
	releaseKnown           bool // false when VERSION was unreadable or not a release
}

// goBuildSelection is the inference result for one file.
//
//	admitted: with tags enabled the constraint is true under every assignment
//	          of the unknown reserved facts. tags is nil when the default
//	          context already admits the file.
//	known:    the decision is trustworthy. known && !admitted means provably
//	          excluded; !known means malformed, over budget or dependent on an
//	          unknown fact (tags is then nil and the file stays on the default
//	          context).
type goBuildSelection struct {
	tags            []string
	known, admitted bool
}

// reviewedGoBuildFacts derives release facts from the reviewed GOROOT's
// VERSION file (metadata only) and fixes OS, architecture and compiler.
func reviewedGoBuildFacts(goroot string) goBuildFacts {
	f := goBuildFacts{goos: runtime.GOOS, goarch: runtime.GOARCH, compiler: "gc"}
	f.releaseMinor, f.releaseKnown = parseGoVersionMinor(goVersionFileName(goroot))
	return f
}

// goVersionFileName is the first word of the reviewed GOROOT's VERSION file,
// such as "go1.26.8", or "" when the file is missing or unreadable.
func goVersionFileName(goroot string) string {
	if goroot == "" {
		return ""
	}
	p := filepath.Join(goroot, "VERSION")
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	file, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxGoVersionFileBytes))
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// parseGoVersionMinor returns N for a "go1.N[.P|rcX|betaX]" first line.
func parseGoVersionMinor(version string) (int, bool) {
	line, _, _ := strings.Cut(version, "\n")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return 0, false
	}
	rest, ok := strings.CutPrefix(fields[0], "go1.")
	if !ok {
		return 0, false
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	if n == 0 || n > 6 || (n > 1 && rest[0] == '0') {
		return 0, false
	}
	if n < len(rest) {
		if c := rest[n]; c != '.' && c != 'r' && c != 'b' {
			return 0, false
		}
	}
	minor, err := strconv.Atoi(rest[:n])
	return minor, err == nil && minor >= 1
}

// ---- header reading and parsing ---------------------------------------------

var (
	goBuildSlashSlash = []byte("//")
	goBuildSlashStar  = []byte("/*")
	goBuildStarSlash  = []byte("*/")
	goBuildPrefix     = []byte("//go:build")
	goBuildPackage    = "package"
)

// readGoBuildPrefix reads r only until the file's package clause line is
// complete, never more than maxGoBuildHeaderBytes. It returns that prefix,
// which parseGoBuildHeader accepts. errGoBuildTruncated means the package clause
// was not reached (budget, EOF or unterminated comment); errGoBuildMalformed an
// invalid header; any other error is the reader's own.
func readGoBuildPrefix(r io.Reader) ([]byte, error) {
	buf := make([]byte, 0, goBuildReadChunk)
	var chunk [goBuildReadChunk]byte
	for {
		room := min(goBuildReadChunk, maxGoBuildHeaderBytes-len(buf))
		n, rerr := io.ReadFull(r, chunk[:room])
		buf = append(buf, chunk[:n]...)
		atEOF := errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF)
		h, serr := scanGoBuildHeader(buf, atEOF)
		switch {
		case serr == nil:
			prefix := buf[:h.end]
			if _, err := parseGoBuildHeader(prefix); err != nil {
				return nil, err
			}
			return prefix, nil
		case !errors.Is(serr, errGoBuildTruncated):
			return nil, serr
		}
		switch {
		case atEOF:
			return nil, errGoBuildTruncated
		case rerr != nil:
			return nil, rerr
		case len(buf) >= maxGoBuildHeaderBytes:
			return nil, errGoBuildTruncated
		}
	}
}

type goBuildHeader struct {
	goBuild   string   // the single //go:build line, when present
	plusBuild []string // // +build lines of the legacy header block
	end       int      // offset just past the package clause line
}

// scanGoBuildHeader mirrors go/build's header rules (parseFileHeader) and
// additionally requires the first non-comment text to be a package clause.
//
// final says data ends at the end of the file: only then is an unterminated last
// line (e.g. "package p" with no newline) a complete package clause.
func scanGoBuildHeader(data []byte, final bool) (goBuildHeader, error) {
	var h goBuildHeader
	var haveGoBuild bool
	blockEnd := 0
	p := data
	ended, inSlashStar := false, false
	for len(p) > 0 {
		line := p
		terminated := false
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p, terminated = line[:i], p[i+1:], true
		} else {
			p = p[len(p):]
		}
		consumed := len(data) - len(p)
		line = bytes.TrimSpace(line)
		if len(line) == 0 && !ended {
			blockEnd = consumed
			continue
		}
		if !bytes.HasPrefix(line, goBuildSlashSlash) {
			ended = true
		}
		if !inSlashStar && isGoBuildComment(line) {
			if haveGoBuild {
				return h, fmt.Errorf("%w: multiple //go:build lines", errGoBuildMalformed)
			}
			haveGoBuild = true
			if len(line) > maxGoBuildLineBytes {
				return h, errGoBuildTooComplex
			}
			h.goBuild = string(line)
		}
		for len(line) > 0 {
			if inSlashStar {
				if i := bytes.Index(line, goBuildStarSlash); i >= 0 {
					inSlashStar = false
					line = bytes.TrimSpace(line[i+len(goBuildStarSlash):])
					continue
				}
				break
			}
			if bytes.HasPrefix(line, goBuildSlashSlash) {
				break
			}
			if bytes.HasPrefix(line, goBuildSlashStar) {
				inSlashStar = true
				line = bytes.TrimSpace(line[len(goBuildSlashStar):])
				continue
			}
			// First non-comment text: it must be the package clause.
			if !terminated && !final {
				// The line may continue in unread data.
				return h, errGoBuildTruncated
			}
			if err := scanGoBuildPackageName(line); err != nil {
				return h, err
			}
			h.end = consumed
			h.plusBuild = plusBuildLines(data[:blockEnd])
			return h, nil
		}
	}
	return h, errGoBuildTruncated
}

func isGoBuildComment(line []byte) bool {
	if !bytes.HasPrefix(line, goBuildPrefix) {
		return false
	}
	rest := line[len(goBuildPrefix):]
	return len(rest) == 0 || len(bytes.TrimSpace(rest)) < len(rest)
}

// scanGoBuildPackageName verifies the first declaration line is a valid
// `package <ident>` clause, using go/scanner's own lexing. A line that was cut
// off by the end of data is only accepted once it names a package.
func scanGoBuildPackageName(line []byte) error {
	var s scanner.Scanner
	fset := token.NewFileSet()
	s.Init(fset.AddFile("", fset.Base(), len(line)), line, nil, 0)
	if _, tok, _ := s.Scan(); tok != token.PACKAGE {
		return fmt.Errorf("%w: first declaration is not a package clause", errGoBuildMalformed)
	}
	_, tok, lit := s.Scan()
	if tok != token.IDENT {
		if tok == token.EOF {
			// A missing name can belong to the next unread line; without that
			// line the declaration is incomplete.
			return errGoBuildTruncated
		}
		return fmt.Errorf("%w: invalid package name", errGoBuildMalformed)
	}
	if lit == "_" || !goBuildValidPackageName(lit) {
		return fmt.Errorf("%w: invalid package name", errGoBuildMalformed)
	}
	if _, tok, _ := s.Scan(); tok != token.EOF && tok != token.SEMICOLON {
		return fmt.Errorf("%w: unexpected text after package clause", errGoBuildMalformed)
	}
	if s.ErrorCount > 0 {
		return fmt.Errorf("%w: invalid package clause", errGoBuildMalformed)
	}
	return nil
}

func goBuildValidPackageName(name string) bool {
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return name != ""
}

// plusBuildLines are the legacy lines go/build honours: only inside the
// comment block that ends at the last blank line before the package clause.
func plusBuildLines(block []byte) []string {
	var out []string
	for len(block) > 0 {
		line := block
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, block = line[:i], block[i+1:]
		} else {
			block = nil
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, goBuildSlashSlash) || !bytes.Contains(line, []byte("+build")) {
			continue
		}
		if text := string(line); constraint.IsPlusBuild(text) {
			out = append(out, text)
		}
	}
	return out
}

// parseGoBuildHeader returns the file's build expression from a prefix that
// reaches the package clause. A nil expression with a nil error is a valid
// unconstrained header. A //go:build line takes precedence over // +build lines
// (which are ANDed), as in go/build. Malformed, ambiguous, truncated or
// over-budget headers are errors, never "unconstrained".
func parseGoBuildHeader(data []byte) (constraint.Expr, error) {
	final := len(data) <= maxGoBuildHeaderBytes
	if len(data) > maxGoBuildHeaderBytes {
		data = data[:maxGoBuildHeaderBytes]
	}
	h, err := scanGoBuildHeader(data, final)
	if err != nil {
		return nil, err
	}
	var expr constraint.Expr
	if h.goBuild != "" {
		if expr, err = constraint.Parse(h.goBuild); err != nil {
			return nil, fmt.Errorf("%w: %v", errGoBuildMalformed, err)
		}
	} else {
		for _, line := range h.plusBuild {
			if len(line) > maxGoBuildLineBytes {
				return nil, errGoBuildTooComplex
			}
			x, err := constraint.Parse(line)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", errGoBuildMalformed, err)
			}
			if expr == nil {
				expr = x
			} else {
				expr = &constraint.AndExpr{X: expr, Y: x}
			}
		}
	}
	if expr != nil && !goBuildExprWithinBounds(expr, 1) {
		return nil, errGoBuildTooComplex
	}
	return expr, nil
}

// goBuildExprWithinBounds checks depth and total node count.
func goBuildExprWithinBounds(x constraint.Expr, depth int) bool {
	nodes := 0
	var walk func(constraint.Expr, int) bool
	walk = func(x constraint.Expr, depth int) bool {
		if nodes++; nodes > maxGoBuildExprNodes || depth > maxGoBuildExprDepth {
			return false
		}
		switch x := x.(type) {
		case *constraint.AndExpr:
			return walk(x.X, depth+1) && walk(x.Y, depth+1)
		case *constraint.OrExpr:
			return walk(x.X, depth+1) && walk(x.Y, depth+1)
		case *constraint.NotExpr:
			return walk(x.X, depth+1)
		case *constraint.TagExpr:
			return true
		}
		return false
	}
	return walk(x, depth)
}

// ---- fact classification -----------------------------------------------------

type goFactState uint8

const (
	factFalse goFactState = iota
	factTrue
	factUnknown // a reserved fact this package never decides (cgo, race, ...)
	factCustom  // an ordinary tag the solver may enable
)

var goKnownOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true,
	"hurd": true, "illumos": true, "ios": true, "js": true, "linux": true, "nacl": true,
	"netbsd": true, "openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true, "zos": true,
}

var goUnixOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true,
	"hurd": true, "illumos": true, "ios": true, "linux": true, "netbsd": true,
	"openbsd": true, "solaris": true,
}

var goKnownArch = map[string]bool{
	"386": true, "amd64": true, "amd64p32": true, "arm": true, "armbe": true, "arm64": true,
	"arm64be": true, "loong64": true, "mips": true, "mipsle": true, "mips64": true,
	"mips64le": true, "mips64p32": true, "mips64p32le": true, "ppc": true, "ppc64": true,
	"ppc64le": true, "riscv": true, "riscv64": true, "s390": true, "s390x": true,
	"sparc": true, "sparc64": true, "wasm": true,
}

// classify mirrors go/build's matchTag for the fixed facts. Reserved names are
// never custom: they are either decided by the fixed facts or unknown.
func (f goBuildFacts) classify(tag string) goFactState {
	switch {
	case tag == f.goos || tag == f.goarch || tag == f.compiler:
		return factTrue
	case f.goos == "android" && tag == "linux", f.goos == "illumos" && tag == "solaris", f.goos == "ios" && tag == "darwin":
		return factTrue
	case tag == "unix":
		return boolState(goUnixOS[f.goos])
	case goKnownOS[tag], goKnownArch[tag], tag == "gc", tag == "gccgo", tag == "ignore":
		return factFalse
	case tag == "cgo", tag == "race", tag == "msan", tag == "asan", tag == "boringcrypto", strings.HasPrefix(tag, "goexperiment."):
		return factUnknown
	}
	if rest, ok := strings.CutPrefix(tag, "go1."); ok {
		n, err := strconv.Atoi(rest)
		switch {
		case err != nil || n < 1 || strconv.Itoa(n) != rest:
			return factFalse // not a release tag go/build would ever define
		case !f.releaseKnown:
			return factUnknown
		}
		return boolState(n <= f.releaseMinor)
	}
	if head, _, ok := strings.Cut(tag, "."); ok && goKnownArch[head] {
		if head == f.goarch {
			return factUnknown // architecture feature tag (amd64.v1, arm.7, ...)
		}
		return factFalse
	}
	return factCustom
}

func boolState(b bool) goFactState {
	if b {
		return factTrue
	}
	return factFalse
}

func (f goBuildFacts) valid() bool { return f.goos != "" && f.goarch != "" && f.compiler != "" }

// matchesGoFileName applies go/build's filename GOOS/GOARCH rules, including
// the _test suffix and platform aliases. It reports false for files go ignores.
func (f goBuildFacts) matchesGoFileName(name string) bool {
	name = path.Base(name)
	if !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return false
	}
	name, _, _ = strings.Cut(name, ".")
	i := strings.Index(name, "_")
	if i < 0 {
		return true
	}
	l := strings.Split(name[i:], "_")
	if n := len(l); n > 0 && l[n-1] == "test" {
		l = l[:n-1]
	}
	n := len(l)
	switch {
	case n >= 2 && goKnownOS[l[n-2]] && goKnownArch[l[n-1]]:
		return f.classify(l[n-1]) == factTrue && f.classify(l[n-2]) == factTrue
	case n >= 1 && (goKnownOS[l[n-1]] || goKnownArch[l[n-1]]):
		return f.classify(l[n-1]) == factTrue
	}
	return true
}

// ---- solver --------------------------------------------------------------------

type goBuildAtom struct {
	state goFactState
	index int // bit position within the custom or unknown mask
}

// inferGoBuildTags decides which custom tags make the file at name (base name)
// with the given header prefix build under the fixed facts, for every possible
// value of the unknown reserved facts. Tags are the fewest possible, ties
// broken lexicographically, sorted and unique. Filename exclusion is decided
// first, as go/build does, and does not need the header.
func inferGoBuildTags(name string, data []byte, facts goBuildFacts) goBuildSelection {
	if !facts.valid() {
		return goBuildSelection{}
	}
	if !facts.matchesGoFileName(name) {
		return goBuildSelection{known: true}
	}
	expr, err := parseGoBuildHeader(data)
	if err != nil {
		return goBuildSelection{}
	}
	if expr == nil {
		return goBuildSelection{known: true, admitted: true}
	}
	atoms := map[string]*goBuildAtom{}
	var custom, unknown []string
	collectGoBuildAtoms(expr, func(tag string) {
		if _, ok := atoms[tag]; ok {
			return
		}
		atoms[tag] = &goBuildAtom{state: facts.classify(tag)}
		switch atoms[tag].state {
		case factCustom:
			custom = append(custom, tag)
		case factUnknown:
			unknown = append(unknown, tag)
		}
	})
	if len(custom) > maxGoBuildCustomAtoms || len(unknown) > maxGoBuildUnknownAtoms {
		return goBuildSelection{}
	}
	for _, tag := range custom {
		if !validGoBuildTag(tag) {
			return goBuildSelection{}
		}
	}
	slices.Sort(custom)
	slices.Sort(unknown)
	for i, tag := range custom {
		atoms[tag].index = i
	}
	for i, tag := range unknown {
		atoms[tag].index = i
	}
	eval := func(cmask, umask uint) bool {
		return expr.Eval(func(tag string) bool {
			a := atoms[tag]
			switch a.state {
			case factTrue:
				return true
			case factCustom:
				return cmask>>a.index&1 == 1
			case factUnknown:
				return umask>>a.index&1 == 1
			}
			return false
		})
	}
	allUnknown := func(cmask uint) bool {
		for u := range uint(1) << len(unknown) {
			if !eval(cmask, u) {
				return false
			}
		}
		return true
	}
	// Fewest tags first; within a size, combinations come out in lexicographic
	// order of their sorted tag lists.
	for size := 0; size <= len(custom); size++ {
		idx := make([]int, size)
		for i := range idx {
			idx[i] = i
		}
		for {
			var mask uint
			for _, i := range idx {
				mask |= 1 << i
			}
			if allUnknown(mask) {
				var tags []string
				for _, i := range idx {
					tags = append(tags, custom[i])
				}
				return goBuildSelection{tags: tags, known: true, admitted: true}
			}
			if !nextGoBuildCombination(idx, len(custom)) {
				break
			}
		}
	}
	// No custom set works for every unknown assignment. If no assignment of
	// anything makes the file build it is provably excluded; otherwise the
	// outcome depends on a reserved fact we do not decide.
	for c := range uint(1) << len(custom) {
		for u := range uint(1) << len(unknown) {
			if eval(c, u) {
				return goBuildSelection{}
			}
		}
	}
	return goBuildSelection{known: true}
}

// nextGoBuildCombination advances idx (an increasing k-subset of 0..n-1) to the
// next one in lexicographic order.
func nextGoBuildCombination(idx []int, n int) bool {
	k := len(idx)
	for i := k - 1; i >= 0; i-- {
		if idx[i] < n-k+i {
			idx[i]++
			for j := i + 1; j < k; j++ {
				idx[j] = idx[j-1] + 1
			}
			return true
		}
	}
	return false
}

func collectGoBuildAtoms(x constraint.Expr, add func(string)) {
	switch x := x.(type) {
	case *constraint.AndExpr:
		collectGoBuildAtoms(x.X, add)
		collectGoBuildAtoms(x.Y, add)
	case *constraint.OrExpr:
		collectGoBuildAtoms(x.X, add)
		collectGoBuildAtoms(x.Y, add)
	case *constraint.NotExpr:
		collectGoBuildAtoms(x.X, add)
	case *constraint.TagExpr:
		add(x.Tag)
	}
}

// validGoBuildTag accepts what is safe to place in a comma-separated -tags value.
func validGoBuildTag(tag string) bool {
	if tag == "" || len(tag) > maxGoBuildTagBytes {
		return false
	}
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}
