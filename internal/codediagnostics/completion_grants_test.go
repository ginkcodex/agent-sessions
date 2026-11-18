package codediagnostics

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// CC-08 turned on ruby-lsp completion and Pyright auto-imports. Both settings
// are bound into the servers' fingerprints, so a project approved before must
// stay approved after, in both approval modes.

// rubyLspServers are the real ruby-lsp config and the one before CC-08.
func rubyLspServers(t *testing.T) (AuthorizedRoot, ServerConfig, ServerConfig) {
	t.Helper()
	rt, env, _ := rubyLspUser(t)
	root := rubocopRoot(t, map[string]string{"Gemfile": "source \"https://rubygems.org\"\n", "Gemfile.lock": lockWith("1.92.2", "")})
	p, err := DiscoverRubyLsp(root, []string{filepath.Join(rt.Prefix, "bin")}, env)
	if err != nil || p.Availability() != "available" {
		t.Fatal(err, p.Provider().Notice)
	}
	cfg, _ := p.Server()
	old := cfg
	old.AdapterPolicy = "ruby-lsp-v1:min-0.26:" + runtime.GOOS + "/" + runtime.GOARCH + ":ruby-wrapper:no-addons:navigation-only:no-edits:opened-documents"
	old.InitializationOptions = json.RawMessage(strings.Replace(rubyLspOptions, `"completion":true`, `"completion":false`, 1))
	if string(old.InitializationOptions) == rubyLspOptions {
		t.Fatal("completion is not on")
	}
	return root, old, cfg
}

// pyrightServers are the real Pyright config and the one before CC-08.
func pyrightServers(t *testing.T) (AuthorizedRoot, ServerConfig, ServerConfig) {
	t.Helper()
	skipUnlessProduction(t)
	root := goplsRoot(t, map[string]string{"pyproject.toml": "", "a.py": ""})
	writeVenv(t, root, ".venv")
	path, env, _ := pyrightHost(t)
	p, err := DiscoverPyright(root, path, env)
	cfg, ok := p.Server()
	if err != nil || !ok {
		t.Fatal(err, p.Provider().Notice)
	}
	old := cfg
	old.Settings = json.RawMessage(strings.Replace(string(cfg.Settings), `"autoImportCompletions":true`, `"autoImportCompletions":false`, 1))
	if string(old.Settings) == string(cfg.Settings) {
		t.Fatalf("auto-imports are not on: %s", cfg.Settings)
	}
	return root, old, cfg
}

func TestCompletionSettingsKeepApproval(t *testing.T) {
	for name, servers := range map[string]func(*testing.T) (AuthorizedRoot, ServerConfig, ServerConfig){
		"ruby-lsp": rubyLspServers,
		"pyright":  pyrightServers,
	} {
		t.Run(name, func(t *testing.T) {
			root, old, cfg := servers(t)
			oldFp, err := Fingerprint(root, old.Tools())
			if err != nil {
				t.Fatal(err)
			}
			newFp, err := Fingerprint(root, cfg.Tools())
			if err != nil || newFp == oldFp {
				t.Fatal("the new settings are not part of approval", err)
			}

			t.Run("session", func(t *testing.T) {
				tr := NewTrust(nil)
				defer tr.Close()
				approve(t, tr, root, old.Tools(), oldFp, "session")
				lease, err := tr.Preflight(root, old.Tools())
				if err != nil {
					t.Fatal(err)
				}
				if a, err := tr.Lookup(root, newFp); err != nil || a.State != "approved" || a.Mode != "session" || a.Fingerprint != newFp {
					t.Fatalf("%+v %v", a, err)
				}
				// The running server's lease ends, so it restarts with the new settings.
				select {
				case <-lease.Context.Done():
				default:
					t.Fatal("the old server's lease did not end")
				}
				if next, err := tr.Preflight(root, cfg.Tools()); err != nil || next.fingerprint != newFp {
					t.Fatalf("new settings not leased: %v", err)
				}
			})

			t.Run("always allow", func(t *testing.T) {
				s := store(t)
				tr := NewTrust(s)
				defer tr.Close()
				approve(t, tr, root, old.Tools(), oldFp, "persistent")
				// After an update the app starts afresh and reads the stored grant.
				restarted := NewTrust(&GrantStore{path: s.path})
				defer restarted.Close()
				if a, err := restarted.Lookup(root, newFp); err != nil || a.State != "approved" || a.Mode != "persistent" || a.Fingerprint != newFp {
					t.Fatalf("%+v %v", a, err)
				}
				if found, err := hasGrant(s, root.project, newFp); err != nil || !found {
					t.Fatalf("grant not carried over: %v", err)
				}
				if _, err := restarted.Preflight(root, cfg.Tools()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
