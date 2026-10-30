package files

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGitHistoryContractRoundTrip(t *testing.T) {
	fixtures := []struct {
		name  string
		value any
	}{
		{"repositories", GitHistoryRepositoryList{Repositories: []GitHistoryRepository{{Path: ".", Name: "project", Scope: "src/"}, {Path: "tools/child", Name: "child"}}}},
		{"refs", GitHistoryRefList{Refs: []GitHistoryRef{{Ref: "refs/heads/main", Name: "main", OID: "0123456789012345678901234567890123456789", Current: true}, {Ref: "refs/remotes/origin/main", Name: "origin/main", Remote: true}}, Shallow: true}},
		{"page", GitHistoryPage{Tip: "0123456789012345678901234567890123456789", Commits: []GitHistorySummary{{OID: "0123456789012345678901234567890123456789", Subject: "fix: unicode 日本語", Author: GitHistoryPerson{Name: "Example", Email: "example@example.test", Time: 1700000000}, Parents: []string{}}}, Cursor: "continuation"}},
		{"detail", GitHistoryCommitDetail{GitHistorySummary: GitHistorySummary{OID: "0123456789012345678901234567890123456789", Parents: []string{"abcdefabcdefabcdefabcdefabcdefabcdefabcd", "1111111111111111111111111111111111111111"}}, Message: "Merge\n\nFull message", Base: "abcdefabcdefabcdefabcdefabcdefabcdefabcd", Changes: []GitHistoryChange{{Path: "new.txt", OldPath: "old.txt", Status: "R", OldMode: "100644", NewMode: "100644"}, {Path: "deleted.txt", Status: "D", OldMode: "100644", NewMode: "000000"}}}},
		{"diff", GitHistoryDiff{Content: Content{Path: "file.txt", Text: "new\n", Changes: []LineChange{{Kind: "modified", Start: 1, Count: 1, Removed: []string{"old"}}}, Edit: &EditInfo{Reason: EditReadOnly}}, Kind: "text"}},
		{"binary", GitHistoryDiff{Content: Content{Path: "image.bin", Binary: true, Edit: &EditInfo{Reason: EditReadOnly}}, Kind: "binary"}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := json.Marshal(fixture.value)
			if err != nil {
				t.Fatal(err)
			}
			decoded := reflect.New(reflect.TypeOf(fixture.value)).Interface()
			if err := json.Unmarshal(data, decoded); err != nil {
				t.Fatal(err)
			}
			again, err := json.Marshal(decoded)
			if err != nil {
				t.Fatal(err)
			}
			var first, second any
			if err := json.Unmarshal(data, &first); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(again, &second); err != nil {
				t.Fatal(err)
			}
			if string(mustJSON(t, first)) != string(mustJSON(t, second)) {
				t.Fatal("contract round trip changed payload")
			}
		})
	}
	// Unknown capability is false for old peers; it must not be inferred from
	// working-tree changes support.
	var capabilities struct {
		GitHistory bool `json:"gitHistory"`
	}
	if err := json.Unmarshal([]byte(`{"gitChanges":true}`), &capabilities); err != nil {
		t.Fatal(err)
	}
	if capabilities.GitHistory {
		t.Fatal("old peer unexpectedly supports history")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
