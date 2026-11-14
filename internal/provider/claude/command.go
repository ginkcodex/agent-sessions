package claude

import (
	"regexp"
	"strings"
)

// commandInvocation matches a record that is only the markers Claude Code
// writes for a slash command the user typed for a skill or plugin:
// message, then name, then optional args. Built-ins write the name first and
// stay hidden.
var commandInvocation = regexp.MustCompile(`(?s)^\s*<command-message>[^<]*</command-message>\s*<command-name>(/[^\s<]+)</command-name>\s*(?:<command-args>(.*)</command-args>\s*)?$`)

// hiddenCommands mirrors the claude-agent-acp replay list: commands whose
// marker records are terminal bookkeeping, not prompts.
var hiddenCommands = map[string]bool{
	"/context": true, "/heapdump": true, "/extra-usage": true, "/compact": true,
	"/model": true, "/mcp": true, "/status": true, "/usage": true,
}

// commandPrompt returns "/name args" for a marker-only invocation record, the
// same text the ACP adapter replays, and false for anything else.
func commandPrompt(text string) (string, bool) {
	m := commandInvocation.FindStringSubmatch(text)
	if m == nil || hiddenCommands[m[1]] {
		return "", false
	}
	return strings.TrimSpace(m[1] + " " + m[2]), true
}
