package codediagnostics

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type cargoSpan struct {
	File      string `json:"file_name"`
	Start     *int   `json:"byte_start"`
	End       *int   `json:"byte_end"`
	Primary   bool   `json:"is_primary"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Text      []struct {
		Text string `json:"text"`
	} `json:"text"`
}
type cargoParsed struct {
	diagnostics []Diagnostic
	partial     bool
	revisions   map[string]string
}

// Cargo emits one JSON object per line, including an explicit terminal outcome.
// A failed build is a complete compiler result only with compiler errors and exit 101.
func parseCargoCheck(root AuthorizedRoot, dir string, data []byte, exit, budget int) (cargoParsed, error) {
	out := cargoParsed{diagnostics: []Diagnostic{}, revisions: map[string]string{}}
	bad := func() (cargoParsed, error) {
		return out, failure("toolFailed", "Cargo check output was incomplete or could not be verified; prepare the lockfile and cached dependencies separately before retrying offline and locked")
	}
	if len(data) > 10<<20 {
		return out, failure("limitExceeded", "Cargo output exceeded the bounded parser limit")
	}
	if !utf8.Valid(data) || len(data) == 0 || data[len(data)-1] != '\n' {
		return bad()
	}
	finished, success, errorsSeen := false, false, false
	records, sourceBytes, spanBytes := 0, 0, 0
	sources := map[string][]byte{}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		records++
		if records > 20000 || len(line) > 1<<20 {
			return out, failure("limitExceeded", "Cargo record limit exceeded")
		}
		if finished {
			return bad()
		}
		var record struct {
			Reason  string `json:"reason"`
			Success *bool  `json:"success"`
			Message *struct {
				Message string `json:"message"`
				Level   string `json:"level"`
				Code    *struct {
					Code string `json:"code"`
				} `json:"code"`
				Spans []cargoSpan `json:"spans"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &record) != nil {
			return bad()
		}
		switch record.Reason {
		case "build-finished":
			if record.Success == nil {
				return bad()
			}
			finished = true
			success = *record.Success
		case "compiler-artifact", "build-script-executed":
			// Cargo progress and build-script metadata are not findings.
		case "compiler-message":
			m := record.Message
			if m == nil || !bounded(m.Message, 8192) || len(m.Spans) > 256 {
				return bad()
			}
			severity := ""
			switch m.Level {
			case "error", "ice":
				severity = "error"
				errorsSeen = true
			case "warning":
				severity = "warning"
			case "note", "help", "failure-note":
				continue
			default:
				return bad()
			}
			code := ""
			if m.Code != nil {
				code = m.Code.Code
				if !bounded(code, 256) {
					return bad()
				}
			}
			emitted := false
			for _, span := range m.Spans {
				if !span.Primary {
					continue
				}
				path := span.File
				if path == "" || strings.ContainsAny(path, "\x00\\") || strings.HasPrefix(path, "<") {
					out.partial = true
					continue
				}
				if !filepath.IsAbs(path) {
					path = filepath.Join(root.Project().Root, filepath.FromSlash(dir), path)
				}
				rel, err := filepath.Rel(root.Project().Root, path)
				rel = filepath.ToSlash(rel)
				if err != nil || ValidatePath(rel) != nil {
					out.partial = true
					continue
				}
				// Never read a generated/build artifact even when it happens to be contained.
				if slicesContainCargoGenerated(rel) {
					out.partial = true
					continue
				}
				if _, err = root.ResolvePath(root.Project().HostID, rel); err != nil {
					out.partial = true
					continue
				}
				if len(out.diagnostics) >= budget {
					return out, failure("limitExceeded", "Cargo finding limit exceeded")
				}
				text, ok := sources[rel]
				if !ok {
					if len(sources) >= 1024 || sourceBytes >= 16<<20 {
						return out, failure("limitExceeded", "Cargo source budget exceeded")
					}
					text, err = rootedRead(root, rel, min(int64(5<<20), int64(16<<20-sourceBytes)))
					if err != nil {
						out.partial = true
						continue
					}
					sourceBytes += len(text)
					sources[rel] = text
					out.revisions[rel] = cargoRevision(text)
				}
				// Coordinate verification scans source text. Bound aggregate work,
				// including repeated spans in one large file, not just unique reads.
				spanBytes += len(text)
				if spanBytes > 64<<20 {
					return out, failure("limitExceeded", "Cargo span verification budget exceeded")
				}
				d := Diagnostic{Path: rel, Severity: severity, Message: m.Message, Source: "cargo check", Code: code, Freshness: "uncertain", Basis: SavedBasis{Kind: "saved", Revision: out.revisions[rel]}, LocationIssue: "invalidRange"}
				if span.Start != nil && span.End != nil && *span.End >= *span.Start && cargoSpanTextMatches(text, span) {
					start, a := cargoBytePosition(text, *span.Start)
					end, b := cargoBytePosition(text, *span.End)
					if a && b {
						d.Range = &Range{Start: start, End: end}
						d.LocationIssue = ""
					}
				}
				out.diagnostics = append(out.diagnostics, d)
				emitted = true
			}
			if !emitted {
				out.partial = true
			}
		default:
			return bad()
		}
	}
	if !finished || success && (exit != 0 || errorsSeen) || !success && (exit != 101 || !errorsSeen) {
		return bad()
	}
	return out, nil
}

func slicesContainCargoGenerated(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == "target" || part == ".cargo" {
			return true
		}
	}
	return false
}
func cargoRevision(text []byte) string {
	sum := sha256.Sum256(text)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Byte offsets, not rustc's character columns, are the authoritative coordinates.
// An exclusive end may be EOF; offsets inside Unicode or CRLF are never rounded.
func cargoBytePosition(text []byte, offset int) (Position, bool) {
	if offset < 0 || offset > len(text) || !utf8.Valid(text) || !utf8.Valid(text[:offset]) || bytes.HasPrefix(text, []byte{0xef, 0xbb, 0xbf}) {
		return Position{}, false
	}
	if offset > 0 && offset < len(text) && text[offset] == '\n' && text[offset-1] == '\r' {
		return Position{}, false
	}
	line := bytes.Count(text[:offset], []byte{'\n'})
	start := bytes.LastIndexByte(text[:offset], '\n') + 1
	return bytePosition(text, line+1, offset-start+1)
}

// rustc carries the source lines used to produce a span. Refuse inline coordinates
// when those disagree with disk, while retaining the contained panel message.
func cargoSpanTextMatches(text []byte, span cargoSpan) bool {
	if span.LineStart < 1 || span.LineEnd < span.LineStart || span.LineEnd-span.LineStart >= 4096 || len(span.Text) != span.LineEnd-span.LineStart+1 {
		return false
	}
	if span.Start == nil || span.End == nil {
		return false
	}
	a, ok := cargoBytePosition(text, *span.Start)
	if !ok || a.Line != span.LineStart-1 {
		return false
	}
	b, ok := cargoBytePosition(text, *span.End)
	if !ok || b.Line != span.LineEnd-1 {
		return false
	}
	start := 0
	for n := 1; n < span.LineStart; n++ {
		i := bytes.IndexByte(text[start:], '\n')
		if i < 0 {
			return false
		}
		start += i + 1
	}
	for _, line := range span.Text {
		end := len(text)
		if i := bytes.IndexByte(text[start:], '\n'); i >= 0 {
			end = start + i
		}
		if string(bytes.TrimSuffix(text[start:end], []byte{'\r'})) != line.Text {
			return false
		}
		start = end
		if start < len(text) {
			start++
		}
	}
	return true
}

func (p cargoParsed) validateSources(root AuthorizedRoot) bool {
	for rel, want := range p.revisions {
		text, err := rootedRead(root, rel, 5<<20)
		if err != nil || cargoRevision(text) != want {
			return false
		}
	}
	return true
}
