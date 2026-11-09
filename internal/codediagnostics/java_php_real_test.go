package codediagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestJavaPHPRealSmoke is opt-in: it never downloads or installs a server.
// Run with CD_PHP_SMOKE_PATH or CD_JAVA_SMOKE_PATH (reviewed executable dirs).
func TestJavaPHPRealSmoke(t *testing.T) {
	for _, language := range []string{"php", "java"} {
		t.Run(language, func(t *testing.T) {
			variable := "CD_PHP_SMOKE_PATH"
			file, saved, draft := "main.php", "<?php\nfunction greet(): string { return 'hello'; }\ngreet();\n", "<?php\nfunction greet(): string { return 'hello'; }\ngreet();\n$broken = ;\n"
			line, character := 2, 2
			if language == "java" {
				variable = "CD_JAVA_SMOKE_PATH"
				file = "Main.java"
				saved = "public class Main {\n static int greet() { return 1; }\n public static void main(String[] args) { greet(); }\n}\n"
				draft = "public class Main {\n static int greet() { return 1; }\n public static void main(String[] args) { greet(); int broken = ; }\n}\n"
				character = 42
			}
			dirs := os.Getenv(variable)
			if dirs == "" {
				t.Skip("set " + variable + " to run a real installed server")
			}
			root := goplsRoot(t, map[string]string{file: saved})
			home := t.TempDir()
			var plan livePlan
			var err error
			if language == "php" {
				plan, err = DiscoverPHP(root, filepath.SplitList(dirs), PHPEnvironment{Home: home})
			} else {
				plan, err = DiscoverJava(root, filepath.SplitList(dirs), JavaEnvironment{Home: home, Cache: filepath.Join(t.TempDir(), "cache"), JavaHome: os.Getenv("JAVA_HOME")})
			}
			if err != nil {
				t.Fatal(err)
			}
			cfg, ok := plan.Server()
			if !ok {
				t.Fatalf("server unavailable: %+v", plan.Provider())
			}
			trust := NewTrust(nil)
			h := newManagerForTools(t, root, cfg.Tools())
			bridge := newPushBridge(cfg.ProviderID, root, h.Manager)
			bridge.Notice = func(s string) { t.Log("notice:", s) }
			sessions := NewSessionManager(trust, SessionOptions{Launch: HostSessionLaunch, Notifications: bridge.Handle, FailureLog: func(f SessionFailure) { t.Logf("server failure: %+v", f) }})
			t.Cleanup(sessions.Close)
			fp, err := Fingerprint(root, cfg.Tools())
			if err != nil {
				t.Fatal(err)
			}
			approve(t, trust, root, cfg.Tools(), fp, "session")
			h.SetDocumentProvider(SessionDocuments{Sessions: sessions, Resolve: func(r AuthorizedRoot, path string) (DocumentServer, bool) {
				return DocumentServer{Config: cfg, LanguageID: language}, path == file
			}})
			ctx, c := context.Background(), h.snap.Context
			start := time.Now()
			if _, err = sessions.Attach(ctx, root, cfg); err != nil {
				t.Fatal(err)
			}
			t.Logf("initialized %s in %s", cfg.ProviderID, time.Since(start))
			if _, err = h.OpenDocument(ctx, c, file, "d1", 1, draft, ""); err != nil {
				t.Fatal(err)
			}
			definitionStart := 9
			if language == "java" {
				definitionStart = 12
			}
			for _, step := range []struct {
				version int64
				text    string
				errors  bool
			}{{1, draft, true}, {2, saved, false}, {3, draft, true}} {
				if step.version > 1 {
					if _, err = h.ChangeDocument(ctx, c, "d1", step.version, step.text); err != nil {
						t.Fatal(err)
					}
				}
				deadline := time.Now().Add(45 * time.Second)
				diagnostic, definition := false, false
				for time.Now().Before(deadline) {
					if s, e := h.GetSnapshot(c); e == nil {
						for _, set := range s.ResultSets {
							if set.ProviderID == cfg.ProviderID && set.Scope.DocumentID == "d1" && (len(set.Diagnostics) > 0) == step.errors {
								diagnostic = true
							}
						}
					}
					if !definition {
						result, e := h.Navigate(ctx, c, "d1", step.version, NavigateDefinition, Position{Line: line, Character: character})
						if e == nil {
							for _, location := range result.Locations {
								if location.Path == file && !location.External && location.Range.Start == (Position{Line: 1, Character: definitionStart}) && location.Range.End == (Position{Line: 1, Character: definitionStart + 5}) {
									definition = true
								}
							}
						}
					}
					if diagnostic && definition {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
				if !diagnostic || !definition {
					t.Fatalf("real %s version %d: expected errors=%v diagnostics=%v definition=%v, session=%+v", language, step.version, step.errors, diagnostic, definition, sessions.Info(root, cfg))
				}
				t.Logf("version %d: errors=%v and exact source definition passed", step.version, step.errors)
			}
			data, err := os.ReadFile(filepath.Join(root.Project().Root, file))
			if err != nil || string(data) != saved {
				t.Fatalf("draft edits changed the saved file: %v", err)
			}
		})
	}
}
