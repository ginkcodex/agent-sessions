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
			h := NewManagerForTest(t, root)
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
			deadline := time.Now().Add(90 * time.Second)
			diagnostic := false
			definition := false
			for time.Now().Before(deadline) {
				if s, e := h.GetSnapshot(c); e == nil {
					for _, set := range s.ResultSets {
						if len(set.Diagnostics) > 0 {
							diagnostic = true
						}
					}
				}
				if !definition {
					result, e := h.Navigate(ctx, c, "d1", 1, NavigateDefinition, Position{Line: line, Character: character})
					if e == nil && len(result.Locations) > 0 {
						definition = true
						t.Logf("definition: %+v", result.Locations)
					}
				}
				if diagnostic && definition {
					t.Log("opened-draft diagnostic and source definition passed")
					return
				}
				time.Sleep(250 * time.Millisecond)
			}
			t.Fatalf("real %s: diagnostics=%v definition=%v, session=%+v", language, diagnostic, definition, sessions.Info(root, cfg))
		})
	}
}
