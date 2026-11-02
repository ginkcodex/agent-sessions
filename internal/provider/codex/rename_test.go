package codex

import (
	"context"
	"testing"
)

func TestNativeNameOverridesGeneratedTitle(t *testing.T) {
	root := t.TempDir()
	db := stateDB(t, root, "state_5.sqlite", `CREATE TABLE threads (id TEXT PRIMARY KEY, title TEXT, name TEXT);`)
	if _, err := db.Exec(`INSERT INTO threads (id,title,name) VALUES (?, 'generated title', 'native rename')`, indexID); err != nil {
		t.Fatal(err)
	}
	idx, err := New(root, nil).loadIndex(context.Background(), root+"/state_5.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.threads[indexID].meta.Title; got != "native rename" {
		t.Fatalf("title=%q", got)
	}
	if _, err := db.Exec(`UPDATE threads SET name=NULL`); err != nil {
		t.Fatal(err)
	}
	idx, err = New(root, nil).loadIndex(context.Background(), root+"/state_5.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.threads[indexID].meta.Title; got != "generated title" {
		t.Fatalf("fallback=%q", got)
	}
}
