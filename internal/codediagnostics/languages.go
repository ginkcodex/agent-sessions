package codediagnostics

import (
	"path"
	"slices"
	"strings"
)

// Language groups the providers of one language, so it can be turned on or off
// as a whole. The list is closed: adding a language is a code change.
type Language struct {
	ID, Name string
	// Live are opened-document language servers; they also serve navigation.
	Live []string
	// Saved are saved-project checkers and in-process analyzers.
	Saved []string
	// Tools are the managed tools that can be downloaded for it.
	Tools []string
}

var languages = []Language{
	{ID: "go", Name: "Go", Live: []string{goplsProviderID}, Saved: []string{"go-vet", GoSyntaxProviderID}, Tools: []string{"gopls"}},
	{ID: "typescript", Name: "TypeScript / JavaScript", Live: []string{tsProviderID}, Tools: []string{"vtsls"}},
	{ID: "svelte", Name: "Svelte", Live: []string{svelteLSProviderID}, Saved: []string{"svelte-check"}, Tools: []string{"svelte-language-server"}},
	{ID: "python", Name: "Python", Live: []string{ruffProviderID, pyrightProviderID}, Tools: []string{"ruff", "pyright"}},
	{ID: "rust", Name: "Rust", Live: []string{rustAnalyzerProviderID}, Saved: []string{"cargo-check"}, Tools: []string{"rust-analyzer"}},
}

// languageFiles tells, by language ID, whether a file is one of its sources.
var languageFiles = map[string]func(file string) bool{
	"go":         func(file string) bool { return strings.HasSuffix(file, ".go") },
	"typescript": func(file string) bool { _, ok := TSLanguageID(file); return ok },
	"svelte":     func(file string) bool { return strings.ToLower(path.Ext(file)) == ".svelte" },
	"python":     func(file string) bool { _, ok := PythonLanguageID(file); return ok },
	"rust":       func(file string) bool { return strings.HasSuffix(file, ".rs") },
}

// Languages returns the supported languages in display order.
func Languages() []Language {
	out := make([]Language, len(languages))
	for i, l := range languages {
		out[i] = Language{ID: l.ID, Name: l.Name, Live: slices.Clone(l.Live), Saved: slices.Clone(l.Saved), Tools: slices.Clone(l.Tools)}
	}
	return out
}

// LanguageOf returns the language a provider belongs to.
func LanguageOf(providerID string) (Language, bool) {
	for i, l := range languages {
		if slices.Contains(l.Live, providerID) || slices.Contains(l.Saved, providerID) {
			return Languages()[i], true
		}
	}
	return Language{}, false
}

func languageID(providerID string) string {
	for _, l := range languages {
		if slices.Contains(l.Live, providerID) || slices.Contains(l.Saved, providerID) {
			return l.ID
		}
	}
	return ""
}

// LiveProviderIDs lists every language server provider, in language order.
func LiveProviderIDs() []string {
	var out []string
	for _, l := range languages {
		out = append(out, l.Live...)
	}
	return out
}

// ManagedToolNames lists every tool a language can download, in language order.
func ManagedToolNames() []string {
	var out []string
	for _, l := range languages {
		out = append(out, l.Tools...)
	}
	return out
}

// DisabledLanguages holds the IDs of the languages turned off in Settings.
type DisabledLanguages map[string]bool

// NewDisabledLanguages ignores IDs that name no language.
func NewDisabledLanguages(ids []string) DisabledLanguages {
	off := DisabledLanguages{}
	for _, l := range languages {
		if slices.Contains(ids, l.ID) {
			off[l.ID] = true
		}
	}
	return off
}

// Disables reports whether the provider's language is off.
func (d DisabledLanguages) Disables(providerID string) bool {
	id := languageID(providerID)
	return id != "" && d[id]
}

// Without drops the registrations of providers whose language is off.
func (d DisabledLanguages) Without(registrations []Registration) []Registration {
	if len(d) == 0 {
		return registrations
	}
	return slices.DeleteFunc(registrations, func(r Registration) bool { return d.Disables(r.ProviderID) })
}

const disabledNotice = "Off in Settings"

// DisabledRegistrations reports, for each language that is off and has sources
// in the project, every provider of that language as disabled. They carry no
// adapter, tool or server, so they add nothing to the approval proposal.
func DisabledRegistrations(root AuthorizedRoot, off DisabledLanguages) []Registration {
	if len(off) == 0 {
		return nil
	}
	scan, err := scanLiveWorkspaces(root)
	if err != nil {
		return nil
	}
	var out []Registration
	for _, l := range languages {
		if !off[l.ID] || !scanHas(scan, languageFiles[l.ID]) {
			continue
		}
		for _, id := range l.Live {
			state := ProviderState{ProviderID: id, Availability: "disabled", Notice: disabledNotice, Phase: "idle", Freshness: "uncertain",
				Coverage: Coverage{Kind: "openedDocuments", Roots: []string{}, ExternalChanges: CoverageExternalChanges}}
			out = append(out, Registration{ProviderID: id, Availability: "disabled", Live: &LiveRegistration{State: state}})
		}
		for _, id := range l.Saved {
			out = append(out, Registration{ProviderID: id, Availability: "disabled", Notice: disabledNotice})
		}
	}
	return out
}

func scanHas(scan workspaceScan, source func(string) bool) bool {
	for _, d := range scan.dirs {
		for _, f := range d.files {
			if source(f) {
				return true
			}
		}
	}
	return false
}
