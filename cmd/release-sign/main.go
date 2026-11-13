// Command release-sign signs a release's SHA256SUMS with minisign, writing
// SHA256SUMS.minisig next to it. The release workflow runs it; the app
// verifies the result with the public keys built into internal/update.
//
//	MINISIGN_KEY="$(cat primary.key)" MINISIGN_PASSWORD=... \
//	  release-sign -version 1.6.0 dist/SHA256SUMS
//
// The signature is checked with the app's own verification before it is
// written, so a key that is not one of the built-in ones fails here and not
// on users' machines. Check a download by hand with
// `minisign -Vm SHA256SUMS -P <public key>`.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"aead.dev/minisign"
	"github.com/ginkcode/agent-sessions/internal/update"
)

func main() {
	version := flag.String("version", "", "release version, recorded in the signed trusted comment")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: release-sign -version X.Y.Z SHA256SUMS")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 || *version == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *version, os.Getenv("MINISIGN_KEY"), os.Getenv("MINISIGN_PASSWORD"), update.Verifies); err != nil {
		fmt.Fprintln(os.Stderr, "release-sign:", err)
		os.Exit(1)
	}
}

// run signs the file at path and writes path + ".minisig". verifies is the
// check the signature must pass first; main passes the app's own.
func run(path, version, keyText, password string, verifies func(message, signature []byte) bool) error {
	if strings.TrimSpace(keyText) == "" {
		return fmt.Errorf("MINISIGN_KEY is empty")
	}
	message, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	key, err := parseKey(keyText, password)
	if err != nil {
		return err
	}
	sig := minisign.SignWithComments(key, message, "version:"+version, "SHA256SUMS of Agent Sessions "+version)
	if !verifies(message, sig) {
		return fmt.Errorf("the signing key %X is not one of the app's built-in keys", key.ID())
	}
	return os.WriteFile(path+".minisig", sig, 0o644)
}

// parseKey reads a minisign secret key file's text, decrypting it when it is
// password protected.
func parseKey(text, password string) (minisign.PrivateKey, error) {
	data := []byte(strings.TrimSpace(text) + "\n")
	if minisign.IsEncrypted(data) {
		key, err := minisign.DecryptKey(password, data)
		if err != nil {
			return minisign.PrivateKey{}, fmt.Errorf("decrypt the signing key (wrong MINISIGN_PASSWORD?): %w", err)
		}
		return key, nil
	}
	var key minisign.PrivateKey
	if err := key.UnmarshalText(data); err != nil {
		return minisign.PrivateKey{}, fmt.Errorf("read the signing key: %w", err)
	}
	return key, nil
}
