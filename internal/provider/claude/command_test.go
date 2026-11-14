package claude

import "testing"

func TestCommandPrompt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"<command-message>ponytail:ponytail</command-message>\n<command-name>/ponytail:ponytail</command-name>", "/ponytail:ponytail", true},
		{"<command-message>ponytail:ponytail</command-message>\n<command-name>/ponytail:ponytail</command-name>\n<command-args>ultra</command-args>", "/ponytail:ponytail ultra", true},
		{"<command-message>understand-anything:understand-chat</command-message>\n<command-name>/understand-anything:understand-chat</command-name>\n<command-args>how does\nauth work?  </command-args>\n", "/understand-anything:understand-chat how does\nauth work?", true},
		{"<command-message>review</command-message>\n<command-name>/review</command-name>\n<command-args></command-args>", "/review", true},
		// Built-ins put the name first; local output and hidden commands stay meta.
		{"<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args></command-args>", "", false},
		{"<command-name>/help</command-name>", "", false},
		{"<local-command-stdout>Set model</local-command-stdout>", "", false},
		{"<command-message>compact</command-message>\n<command-name>/compact</command-name>", "", false},
		// Markers mixed with prose are an ordinary prompt, not an invocation.
		{"<command-message>x</command-message><command-name>/x</command-name> and more", "", false},
		{"Please explain <command-name>/x</command-name>", "", false},
		{"<command-message>x</command-message><command-name>x</command-name>", "", false},
		{"<command-message>x</command-message><command-name>/a b</command-name>", "", false},
		{"", "", false},
	} {
		got, ok := commandPrompt(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("commandPrompt(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
