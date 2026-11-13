package update

import (
	"strings"

	"aead.dev/minisign"
)

// trustedKeys are the minisign public keys that may sign a release's
// SHA256SUMS: the primary key, whose secret half is a secret of the release
// workflow, and an offline backup, so losing the primary does not strand
// installed apps. Each entry is the key's base64 line from its .pub file.
// Both are listed in the README for checking a download by hand.
var trustedKeys = []string{
	"RWR127s0GEvTCatCwsSCdVikj8CsSg6vv53ICkzxytDmPta9Dfks6/bF", // primary
	"RWQxgXSi+X7Z7MotztNJWrJtPUAIB/aE/kVTprTClstsg16ZMQVgDsBR", // backup (offline)
}

// extraKeys adds public keys, comma separated, at build time, for QA builds
// that check a feed signed with a throwaway key:
//
//	-ldflags "-X github.com/ginkcode/agent-sessions/internal/update.extraKeys=RWS..."
//
// A release build leaves it empty.
var extraKeys string

// builtinKeys parses the trusted keys. An entry that does not parse is
// skipped, so a typo cannot widen what is trusted.
func builtinKeys() []minisign.PublicKey {
	var keys []minisign.PublicKey
	entries := append([]string(nil), trustedKeys...)
	entries = append(entries, strings.Split(extraKeys, ",")...)
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		var key minisign.PublicKey
		if err := key.UnmarshalText([]byte(entry)); err != nil {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// Verifies reports whether any built-in key signed message. The release
// workflow uses it to check a fresh signature exactly as the app will.
func Verifies(message, signature []byte) bool {
	for _, key := range builtinKeys() {
		if minisign.Verify(key, message, signature) {
			return true
		}
	}
	return false
}
