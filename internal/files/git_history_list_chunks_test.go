package files

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestGitHistoryListChunkedSearch(t *testing.T) {
	root, git := blameRepo(t)
	var stream strings.Builder
	for i := 0; i < historyChunkSize+5; i++ {
		message := "ordinary"
		if i == 0 || i == historyChunkSize+4 {
			message = "wanted"
		}
		fmt.Fprintf(&stream, "commit refs/heads/chunks\ncommitter Chunk Author <chunks@example.test> %d +0000\ndata %d\n%s\n\n", 1700000000+i, len(message), message)
	}
	cmd := exec.Command("git", "-C", root, "fast-import", "--quiet")
	cmd.Stdin = strings.NewReader(stream.String())
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("import: %v %s", err, data)
	}
	req := GitHistoryListRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Ref: "refs/heads/chunks", Query: "wanted", Limit: 1}
	page, err := GitHistoryList(context.Background(), req)
	if err != nil || len(page.Commits) != 1 || page.Cursor == "" {
		t.Fatalf("first: %+v %v", page, err)
	}
	req.Cursor = page.Cursor
	page, err = GitHistoryList(context.Background(), req)
	if err != nil || len(page.Commits) != 1 || page.Cursor != "" {
		t.Fatalf("last match across chunks: %+v %v", page, err)
	}
	// A valid continuation with no later matches is a successful empty page.
	first, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: req.GitHistoryTarget, Ref: req.Ref, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.RawURLEncoding.DecodeString(first.Cursor)
	var cursor gitHistoryCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		t.Fatal(err)
	}
	cursor.Query = "absent"
	data, _ = json.Marshal(cursor)
	req.Query = "absent"
	req.Cursor = base64.RawURLEncoding.EncodeToString(data)
	page, err = GitHistoryList(context.Background(), req)
	if err != nil || len(page.Commits) != 0 || page.Cursor != "" {
		t.Fatalf("empty continuation: %+v %v", page, err)
	}
	// Hash prefixes are ASCII hexadecimal and case insensitive.
	req.Query = strings.ToUpper(git("rev-parse", "refs/heads/chunks")[:6])
	req.Cursor = ""
	page, err = GitHistoryList(context.Background(), req)
	if err != nil || len(page.Commits) != 1 {
		t.Fatalf("uppercase hash: %+v %v", page, err)
	}
}
