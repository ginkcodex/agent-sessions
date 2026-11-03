package app

import (
	"reflect"
	"testing"
)

func TestDialogFilters(t *testing.T) {
	bundles := FileFilter{DisplayName: "Bundles", Pattern: "*.agent-session.zip;*.zip"}
	bundleOnly := FileFilter{DisplayName: "Bundles", Pattern: "*.agent-session.zip"}
	markdown := FileFilter{DisplayName: "Markdown", Pattern: "*.md"}
	all := FileFilter{DisplayName: "All Files", Pattern: "*.*"}

	tests := []struct {
		name    string
		goos    string
		filters []FileFilter
		want    []FileFilter
	}{
		{
			name:    "linux keeps globs",
			goos:    "linux",
			filters: []FileFilter{bundles, all},
			want:    []FileFilter{bundles, all},
		},
		{
			name:    "darwin drops dotted and wildcard patterns",
			goos:    "darwin",
			filters: []FileFilter{bundles, all},
			want:    []FileFilter{{DisplayName: "Bundles", Pattern: "*.zip"}},
		},
		{
			name:    "darwin keeps plain extensions",
			goos:    "darwin",
			filters: []FileFilter{markdown, all},
			want:    []FileFilter{markdown},
		},
		{
			name:    "darwin with nothing usable allows every file",
			goos:    "darwin",
			filters: []FileFilter{bundleOnly},
			want:    nil,
		},
		{
			name: "darwin trims spaces and skips odd patterns",
			goos: "darwin",
			filters: []FileFilter{
				{DisplayName: "Mixed", Pattern: " *.json ; *; *.; report.txt; *.tar.gz ;*.JSONL"},
			},
			want: []FileFilter{{DisplayName: "Mixed", Pattern: "*.json;*.JSONL"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dialogFilters(tt.goos, tt.filters)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("dialogFilters(%q) = %#v, want %#v", tt.goos, got, tt.want)
			}
		})
	}
}
