package codediagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func cargoMessage(path, level string, start, end int, source string) string {
	record := map[string]any{"reason": "compiler-message", "message": map[string]any{"message": "compiler finding", "level": level, "code": map[string]any{"code": "E0308"}, "spans": []any{map[string]any{"file_name": path, "byte_start": start, "byte_end": end, "line_start": 1, "line_end": 1, "is_primary": true, "text": []any{map[string]any{"text": source}}}}}}
	b, _ := json.Marshal(record)
	return string(b) + "\n"
}

const cargoSuccess = "{\"reason\":\"build-finished\",\"success\":true}\n"
const cargoFailure = "{\"reason\":\"build-finished\",\"success\":false}\n"

func TestCargoCheckParserCompletion(t *testing.T) {
	root := vetRoot(t, map[string]string{"src/lib.rs": "é😀 x\r\nnext\n"})
	finding := cargoMessage("src/lib.rs", "error", 2, 6, "é😀 x")
	for _, tc := range []struct {
		name, text string
		exit       int
		valid      bool
	}{
		{"clean", cargoSuccess, 0, true},
		{"compiler-errors", finding + cargoFailure, 101, true},
		{"warning", cargoMessage("src/lib.rs", "warning", 2, 6, "é😀 x") + cargoSuccess, 0, true},
		{"normal-progress", "{\"reason\":\"compiler-artifact\"}\n{\"reason\":\"build-script-executed\"}\n" + cargoSuccess, 0, true},
		{"operational", cargoFailure, 101, false},
		{"missing-terminal", finding, 101, false},
		{"empty", "", 0, false},
		{"truncated", strings.TrimSuffix(cargoSuccess, "\n"), 0, false},
		{"truncated-json", "{\"reason\":", 101, false},
		{"exit-mismatch", finding + cargoFailure, 0, false},
		{"error-success", finding + cargoSuccess, 0, false},
		{"post-terminal", cargoSuccess + finding, 0, false},
		{"unknown", "{\"reason\":\"new-unverified-shape\"}\n" + cargoSuccess, 0, false},
		{"oversized", strings.Repeat("x", (1<<20)+1) + "\n", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCargoCheck(root, "", []byte(tc.text), tc.exit, 100)
			if (err == nil) != tc.valid {
				t.Fatalf("got %+v, %v", got, err)
			}
			if tc.name == "compiler-errors" {
				d := got.diagnostics[0]
				if d.Range == nil || *d.Range != (Range{Position{0, 1}, Position{0, 3}}) || d.Code != "E0308" || d.Severity != "error" || d.Basis.Revision != cargoRevision([]byte("é😀 x\r\nnext\n")) || d.Freshness != "uncertain" {
					t.Fatal(d)
				}
			}
		})
	}
}
func TestCargoCheckSourceContainmentAndCoordinates(t *testing.T) {
	root := vetRoot(t, map[string]string{"src/lib.rs": "é😀 x\r\nnext\n", "target/generated.rs": "x"})
	for _, tc := range []struct {
		name, path, text string
		start, end       int
		ranged, omitted  bool
	}{
		{"valid", "src/lib.rs", "é😀 x", 2, 6, true, false},
		{"unicode-interior", "src/lib.rs", "é😀 x", 3, 6, false, false},
		{"crlf-interior", "src/lib.rs", "é😀 x", 2, 9, false, false},
		{"reversed", "src/lib.rs", "é😀 x", 6, 2, false, false},
		{"changed-source", "src/lib.rs", "different", 2, 6, false, false},
		{"negative", "src/lib.rs", "é😀 x", -1, 6, false, false},
		{"external", "../outside.rs", "x", 0, 1, false, true},
		{"generated", "target/generated.rs", "x", 0, 1, false, true},
		{"synthetic", "<macro expansion>", "x", 0, 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCargoCheck(root, "", []byte(cargoMessage(tc.path, "error", tc.start, tc.end, tc.text)+cargoFailure), 101, 100)
			if err != nil {
				t.Fatal(err)
			}
			if tc.omitted {
				if len(got.diagnostics) != 0 || !got.partial {
					t.Fatal(got)
				}
				return
			}
			if len(got.diagnostics) != 1 || (got.diagnostics[0].Range != nil) != tc.ranged {
				t.Fatal(got)
			}
		})
	}
	if _, err := parseCargoCheck(root, "", []byte(cargoMessage("src/lib.rs", "error", 2, 6, "é😀 x")+cargoFailure), 101, 0); err == nil {
		t.Fatal("finding budget ignored")
	}
	text := []byte("é😀\r\nx")
	for _, tc := range []struct {
		off int
		p   Position
		ok  bool
	}{{0, Position{0, 0}, true}, {2, Position{0, 1}, true}, {6, Position{0, 3}, true}, {7, Position{}, false}, {8, Position{1, 0}, true}, {9, Position{1, 1}, true}, {1, Position{}, false}} {
		got, ok := cargoBytePosition(text, tc.off)
		if ok != tc.ok || ok && got != tc.p {
			t.Fatalf("offset %d: %v %v", tc.off, got, ok)
		}
	}
	parsed, err := parseCargoCheck(root, "", []byte(cargoMessage("src/lib.rs", "warning", 2, 6, "é😀 x")+cargoSuccess), 0, 100)
	if err != nil || !parsed.validateSources(root) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root.Project().Root, "src/lib.rs"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if parsed.validateSources(root) {
		t.Fatal("source change accepted")
	}
}

func cargoTestPlan(t *testing.T) *CargoCheckPlan {
	t.Helper()
	root := vetRoot(t, map[string]string{"Cargo.toml": "[package]\nname=\"fixture\"\nversion=\"0.1.0\"\nedition=\"2021\"\n", "Cargo.lock": "# fixture\n", "src/lib.rs": "é😀 x\n"})
	bin := t.TempDir()
	for _, name := range []string{"cargo", "rustc"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte("fake tool identity, never executed"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	p, err := DiscoverCargoCheck(root, []string{bin}, RustEnvironment{Home: home, CargoHome: filepath.Join(home, ".cargo"), RustupHome: filepath.Join(home, ".rustup")})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestCargoCheckFixtureUsesHostExecutableNames(t *testing.T) {
	p := cargoTestPlan(t)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	if got := filepath.Base(p.commands[0].Executable); got != "cargo"+suffix {
		t.Fatalf("Cargo fixture executable %q", got)
	}
	if got := filepath.Base(p.discovery.Rustc()); got != "rustc"+suffix {
		t.Fatalf("rustc fixture executable %q", got)
	}
}

func TestCargoCheckSupervisor(t *testing.T) {
	for _, mode := range []string{"clean", "stderr", "compiler", "operational", "incomplete", "config-created", "config-during", "source-during", "cancel", "composite-change"} {
		t.Run(mode, func(t *testing.T) {
			p := cargoTestPlan(t)
			c := p.commands[0]
			if !reflect.DeepEqual(c.Args, []string{"check", "--workspace", "--offline", "--locked", "--message-format=json"}) {
				t.Fatal(c.Args)
			}
			tools := p.Tools()
			extra := filepath.Join(t.TempDir(), "other-tool")
			if err := os.WriteFile(extra, []byte("reviewed"), 0700); err != nil {
				t.Fatal(err)
			}
			tools = append(tools, ToolInput{ProviderID: "other-provider", Executable: extra, Policy: "other-policy"})
			tr := NewTrust(nil)
			fp, err := Fingerprint(p.root, tools)
			if err != nil {
				t.Fatal(err)
			}
			approve(t, tr, p.root, tools, fp, "session")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			s := NewSupervisor(tr, SupervisorOptions{Start: func(in Invocation) (Process, error) {
				calls++
				proc := newFakeProcess()
				text := cargoSuccess
				switch mode {
				case "stderr":
					_, _ = in.Stderr.Write([]byte("Checking fixture\nFinished dev profile\n"))
				case "compiler":
					text = cargoMessage("src/lib.rs", "error", 2, 6, "é😀 x") + cargoFailure
					proc.exit = 101
				case "operational":
					text = cargoFailure
					proc.exit = 101
					_, _ = in.Stderr.Write([]byte("missing offline dependency or locked lockfile"))
				case "incomplete":
					text = cargoMessage("src/lib.rs", "error", 2, 6, "é😀 x")
					proc.exit = 101
				case "config-during":
					if err := os.WriteFile(filepath.Join(p.root.Project().Root, "Cargo.lock"), []byte("changed"), 0600); err != nil {
						t.Error(err)
					}
				case "source-during":
					text = cargoMessage("src/lib.rs", "error", 2, 6, "old source") + cargoFailure
					proc.exit = 101
				case "cancel":
					cancel()
				}
				_, _ = in.Stdout.Write([]byte(text))
				_ = proc.Terminate()
				return proc, nil
			}})
			defer s.Close()
			if mode == "config-created" {
				if err := os.MkdirAll(filepath.Join(p.root.Project().Root, ".cargo"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p.root.Project().Root, ".cargo/config.toml"), []byte("[build]\njobs=1\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "composite-change" {
				if err := os.WriteFile(extra, []byte("changed"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			got := p.RunApproved(ctx, s, "cargo-fixture", tools)
			// A changed tool is covered by the project's approval.
			success := mode == "clean" || mode == "stderr" || mode == "compiler" || mode == "source-during" || mode == "composite-change"
			if got.Replace != success || !success && (got.Failure == nil || got.Freshness != "stale" || !got.Partial) {
				t.Fatalf("%+v", got)
			}
			if mode == "config-created" && calls != 0 {
				t.Fatal("unapproved execution")
			}
			if mode == "source-during" && got.Diagnostics[0].Range != nil {
				t.Fatal("changed source got inline range")
			}
		})
	}
}

// Submit can queue behind other work on the same project. Rust inputs created
// after submission must be re-enumerated at launch, not only before enqueueing.
func TestCargoCheckRevalidatesInputsAtLaunch(t *testing.T) {
	p := cargoTestPlan(t)
	c := p.commands[0]
	if c.ValidateStart == nil {
		t.Fatal("Cargo command does not re-enumerate inputs at launch")
	}
	blockerExe := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blockerExe, []byte("reviewed"), 0700); err != nil {
		t.Fatal(err)
	}
	blocker := Command{ProviderID: "blocker", Executable: blockerExe, AdapterPolicy: "blocker-v1", AcceptedExits: []int{0}}
	tools := append(p.Tools(), ToolInput{ProviderID: blocker.ProviderID, Executable: blocker.Executable, Policy: blocker.Policy()})
	tr := NewTrust(nil)
	fp, err := Fingerprint(p.root, tools)
	if err != nil {
		t.Fatal(err)
	}
	approve(t, tr, p.root, tools, fp, "session")
	starts := make(chan started, 4)
	s := NewSupervisor(tr, SupervisorOptions{Start: fakeFactory(starts)})
	defer s.Close()
	first := submit(t, s, p.root, tools, blocker, "blocker")
	running := nextStart(t, starts)
	queued := submit(t, s, p.root, tools, c, "cargo")
	// A newly created config is not in the approved ConfigPaths, so only
	// re-enumeration can detect it.
	if err := os.MkdirAll(filepath.Join(p.root.Project().Root, ".cargo"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.root.Project().Root, ".cargo/config.toml"), []byte("[net]\noffline=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = running.process.Terminate()
	result(t, first)
	if result(t, queued).Kind != OutcomeTrust {
		t.Fatal("Cargo launched with unreviewed configuration")
	}
	select {
	case <-starts:
		t.Fatal("Cargo spawned after configuration changed")
	default:
	}
}

func TestCargoCheckRepeatedSpanWorkBound(t *testing.T) {
	root := vetRoot(t, map[string]string{"src/lib.rs": strings.Repeat("x", 1<<20)})
	data := strings.Repeat(cargoMessage("src/lib.rs", "error", 0, 1, "x"), 65) + cargoFailure
	if _, err := parseCargoCheck(root, "", []byte(data), 101, 100); errCode(err) != "limitExceeded" {
		t.Fatalf("unbounded repeated span work: %v", err)
	}
}
