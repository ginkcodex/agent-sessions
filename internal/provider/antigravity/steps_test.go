package antigravity

import "testing"

func TestUserRequest(t *testing.T) {
	for content, want := range map[string]string{
		"<USER_REQUEST>\nRead AGENTS.md\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\ntime\n</ADDITIONAL_METADATA>\n<USER_SETTINGS_CHANGE>\nmodel\n</USER_SETTINGS_CHANGE>": "Read AGENTS.md",
		"plain text\n<ADDITIONAL_METADATA>\ntime\n</ADDITIONAL_METADATA>":                                                                                              "plain text",
		"a <USER_SETTINGS_CHANGE>x</USER_SETTINGS_CHANGE> b":                                                                                                           "a  b",
		"<USER_REQUEST>\nunterminated":          "unterminated",
		"cut <ADDITIONAL_METADATA>never closed": "cut",
	} {
		if got := userRequest(content); got != want {
			t.Errorf("userRequest(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestParseSystemMessage(t *testing.T) {
	content := "The following is a <SYSTEM_MESSAGE> not actually sent by the user.\n\n<SYSTEM_MESSAGE>\n" +
		"[Message] timestamp=2026-11-19T03:55:09Z sender=" + parentID + "/task-54 priority=MESSAGE_PRIORITY_HIGH content=Task finished with result: x content=y\n</SYSTEM_MESSAGE>"
	msg, ok := parseSystemMessage(content)
	if !ok || msg.Sender != parentID+"/task-54" || msg.Text != "Task finished with result: x content=y" || msg.fromSession() {
		t.Fatalf("got %+v %v", msg, ok)
	}
	msg, ok = parseSystemMessage("<SYSTEM_MESSAGE>\n[Message] sender=system content=restarted\n</SYSTEM_MESSAGE>")
	if !ok || msg.Sender != "system" || msg.Text != "restarted" || msg.fromSession() {
		t.Fatalf("got %+v %v", msg, ok)
	}
	if msg, _ := parseSystemMessage("<SYSTEM_MESSAGE>\n[Message] sender=" + childID + " content=report"); !msg.fromSession() {
		t.Fatalf("a child's report is from a session: %+v", msg)
	}
	if _, ok := parseSystemMessage("no header"); ok {
		t.Fatal("parsed content with no header")
	}
}
