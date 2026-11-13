package main

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"
	"github.com/ginkcode/agent-sessions/internal/update"
)

func keyPair(t *testing.T) (minisign.PublicKey, string) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	text, err := priv.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	return pub, string(text)
}

func sums(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SHA256SUMS")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 64)+"  agent-sessions_1.6.0_macos_universal.dmg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func verifiesWith(pub minisign.PublicKey) func(message, signature []byte) bool {
	return func(message, signature []byte) bool { return minisign.Verify(pub, message, signature) }
}

func TestRunWritesASignatureThatVerifies(t *testing.T) {
	pub, key := keyPair(t)
	path := sums(t)
	if err := run(path, "1.6.0", key, "", verifiesWith(pub)); err != nil {
		t.Fatal(err)
	}
	message, _ := os.ReadFile(path)
	sig, err := os.ReadFile(path + ".minisig")
	if err != nil {
		t.Fatal(err)
	}
	if !minisign.Verify(pub, message, sig) {
		t.Fatal("the signature does not verify")
	}
	if !strings.Contains(string(sig), "trusted comment: version:1.6.0") {
		t.Fatalf("the signed comment does not name the version:\n%s", sig)
	}
	// A changed file no longer verifies.
	if minisign.Verify(pub, append(message, '\n'), sig) {
		t.Fatal("a changed SHA256SUMS still verifies")
	}
}

func TestRunRefusesAKeyTheAppDoesNotTrust(t *testing.T) {
	_, key := keyPair(t)
	path := sums(t)
	// The real check: only the app's built-in keys count.
	err := run(path, "1.6.0", key, "", update.Verifies)
	if err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("run = %v, want a refusal naming the built-in keys", err)
	}
	if _, statErr := os.Stat(path + ".minisig"); statErr == nil {
		t.Fatal("a signature the app would reject was written")
	}
}

func TestRunRefusesMissingOrBrokenInput(t *testing.T) {
	pub, key := keyPair(t)
	for name, tc := range map[string]struct{ path, key string }{
		"no key":         {sums(t), ""},
		"garbage key":    {sums(t), "not a key"},
		"missing sums":   {filepath.Join(t.TempDir(), "SHA256SUMS"), key},
		"whitespace key": {sums(t), "  \n"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(tc.path, "1.6.0", tc.key, "", verifiesWith(pub)); err == nil {
				t.Fatal("run succeeded")
			}
			if _, err := os.Stat(tc.path + ".minisig"); err == nil {
				t.Fatal("a signature was written")
			}
		})
	}
}

// An encrypted key uses minisign's sensitive scrypt cost (1 GiB of memory),
// which is what the release secret is, so this is skipped in short runs.
func TestRunDecryptsAPasswordProtectedKey(t *testing.T) {
	if testing.Short() {
		t.Skip("scrypt at minisign's sensitive cost takes seconds and 1 GiB")
	}
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := minisign.EncryptKey("correct horse", priv)
	if err != nil {
		t.Fatal(err)
	}
	path := sums(t)
	if err := run(path, "1.6.0", string(enc), "wrong", verifiesWith(pub)); err == nil {
		t.Fatal("a wrong password was accepted")
	}
	if err := run(path, "1.6.0", string(enc), "correct horse", verifiesWith(pub)); err != nil {
		t.Fatal(err)
	}
}
