package claude

import (
	"strings"

	"github.com/tidwall/gjson"
)

// manualBashRecord recognizes dedicated Claude Code ! command records, not
// prose containing examples of the wrappers. Payloads remain literal text.
type manualBashRecord struct {
	command string
}

func parseManualBash(text string) (manualBashRecord, bool) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "<bash-input>") && strings.HasSuffix(text, "</bash-input>") {
		body := strings.TrimSuffix(strings.TrimPrefix(text, "<bash-input>"), "</bash-input>")
		if !strings.Contains(body, "</bash-input>") {
			return manualBashRecord{command: strings.TrimSpace(body)}, true
		}
	}
	// Output is two ordered streams. Requiring the complete envelope avoids
	// classifying partial writes or snippets embedded in ordinary user prose.
	if rest, ok := strings.CutPrefix(text, "<bash-stdout>"); ok {
		_, rest, ok = strings.Cut(rest, "</bash-stdout>")
		if ok {
			rest, ok = strings.CutPrefix(strings.TrimSpace(rest), "<bash-stderr>")
			if ok && strings.HasSuffix(rest, "</bash-stderr>") && !strings.Contains(strings.TrimSuffix(rest, "</bash-stderr>"), "</bash-stderr>") {
				return manualBashRecord{}, true
			}
		}
	}
	return manualBashRecord{}, false
}

// manualBashContent only accepts text-only records. Attachments/tool results
// and mixed prose retain their ordinary metadata and transcript interpretation.
func manualBashContent(content gjson.Result) (manualBashRecord, bool) {
	if content.Type == gjson.String {
		return parseManualBash(content.String())
	}
	if !content.IsArray() {
		return manualBashRecord{}, false
	}
	var texts []string
	for _, part := range content.Array() {
		if part.Get("type").String() != "text" {
			return manualBashRecord{}, false
		}
		texts = append(texts, part.Get("text").String())
	}
	return parseManualBash(strings.Join(texts, "\n"))
}
