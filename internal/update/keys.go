package update

import (
	"strings"

	"aead.dev/minisign"
)

// trustedKeys are the minisign public keys that may sign a release's
// SHA256SUMS: the primary key and an offline backup, so losing the primary
// does not strand installed apps. Each entry is the key's base64 line from
// its .pub file. AU-01 fills this in; until then no release verifies and the
// app keeps its unsigned release-page notice.
var trustedKeys = []string{}

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
