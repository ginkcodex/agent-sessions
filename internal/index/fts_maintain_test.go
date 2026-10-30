package index

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

type transcriptFunc func(context.Context, model.SessionRef) (*model.Transcript, error)

func (f transcriptFunc) LoadForIndex(ctx context.Context, ref model.SessionRef) (*model.Transcript, error) {
	return f(ctx, ref)
}

// indexTranscript commits a scan of tr's session and indexes it.
func indexTranscript(t *testing.T, db *DB, tr *model.Transcript) {
	t.Helper()
	ctx := t.Context()
	if err := db.CommitScan(ctx, model.AgentClaude, provider.ScanResult{Changed: []model.SessionMeta{tr.Meta}}); err != nil {
		t.Fatalf("CommitScan: %v", err)
	}
	loader := transcriptFunc(func(context.Context, model.SessionRef) (*model.Transcript, error) { return tr, nil })
	if err := db.IndexPending(ctx, loader, nil); err != nil {
		t.Fatalf("IndexPending: %v", err)
	}
}

func textTranscript(id, title string, texts ...string) *model.Transcript {
	tr := &model.Transcript{Meta: model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentClaude, ID: id}, Title: title}}
	for _, text := range texts {
		tr.Messages = append(tr.Messages, model.Message{Role: model.RoleUser, Parts: []model.Part{{Kind: model.PartText, Text: text}}})
	}
	return tr
}

func requireFTSSound(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), "INSERT INTO fts_messages(fts_messages) VALUES('integrity-check')"); err != nil {
		t.Fatalf("FTS integrity: %v", err)
	}
}

// Reindexing a changed session writes only the docs that changed, so the
// FTS index gains no tombstones for the messages that stayed the same.
func TestReplaceFTSDocsWritesOnlyChanges(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	indexTranscript(t, db, textTranscript("s", "first title", "kept alpha", "edited bravo", "dropped charlie"))
	rowids := map[int]int64{}
	rows, err := db.SQLDB().QueryContext(ctx, "SELECT message_index, rowid FROM fts_docs")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var idx int
		var rowid int64
		if err := rows.Scan(&idx, &rowid); err != nil {
			t.Fatal(err)
		}
		rowids[idx] = rowid
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(rowids) != 4 {
		t.Fatalf("indexed docs = %v", rowids)
	}

	// Record every later write to fts_docs.
	if _, err := db.SQLDB().ExecContext(ctx, `
CREATE TABLE doc_writes (op TEXT, rowid INTEGER);
CREATE TRIGGER doc_ins AFTER INSERT ON fts_docs BEGIN INSERT INTO doc_writes VALUES ('insert', new.rowid); END;
CREATE TRIGGER doc_upd AFTER UPDATE ON fts_docs BEGIN INSERT INTO doc_writes VALUES ('update', old.rowid); END;
CREATE TRIGGER doc_del AFTER DELETE ON fts_docs BEGIN INSERT INTO doc_writes VALUES ('delete', old.rowid); END;`); err != nil {
		t.Fatal(err)
	}
	// Message 2 becomes empty, which drops its doc.
	indexTranscript(t, db, textTranscript("s", "second title", "kept alpha", "edited bravo now", "", "added delta"))

	got := map[string]int{}
	rows, err = db.SQLDB().QueryContext(ctx, "SELECT op, rowid FROM doc_writes")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var op string
		var rowid int64
		if err := rows.Scan(&op, &rowid); err != nil {
			t.Fatal(err)
		}
		got[fmt.Sprintf("%s %d", op, rowid)]++
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	var added int64
	if err := db.SQLDB().QueryRowContext(ctx, "SELECT rowid FROM fts_docs WHERE message_index = 3").Scan(&added); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		fmt.Sprintf("update %d", rowids[-1]): 1,
		fmt.Sprintf("update %d", rowids[1]):  1,
		fmt.Sprintf("delete %d", rowids[2]):  1,
		fmt.Sprintf("insert %d", added):      1,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}

	for query, n := range map[string]int{
		"alpha": 1, "bravo": 1, "now": 1, "charlie": 0, "delta": 1, "first": 0, "second": 1,
	} {
		if got := countRows(t, db.SQLDB(), "SELECT count(*) FROM fts_messages WHERE fts_messages MATCH ?", query); got != n {
			t.Errorf("MATCH %q = %d, want %d", query, got, n)
		}
	}
	requireFTSSound(t, db.SQLDB())
}

// Open does not run FTS5's own check; a damaged full-text index keeps the
// catalog and is rebuilt from fts_docs by maintainFTS.
func TestMaintainFTSRebuildsDamagedIndex(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := Open(ctx, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	indexTranscript(t, db, textTranscript("s", "title", "needle in a haystack"))
	if _, err := db.SQLDB().ExecContext(ctx,
		"UPDATE fts_messages_data SET block = zeroblob(length(block)) WHERE id = (SELECT max(id) FROM fts_messages_data)"); err != nil {
		t.Fatal(err)
	}
	if err := quickCheck(ctx, db.SQLDB(), "fts_messages"); err == nil {
		t.Fatal("damaged index passed the FTS check")
	}
	_ = db.Close()

	db, err = Open(ctx, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if n := countRows(t, db.SQLDB(), "SELECT count(*) FROM sessions"); n != 1 {
		t.Fatalf("Open reset the cache over FTS damage: %d sessions", n)
	}
	if err := db.maintainFTS(ctx); err != nil {
		t.Fatalf("maintainFTS: %v", err)
	}
	requireFTSSound(t, db.SQLDB())
	if n := countRows(t, db.SQLDB(), "SELECT count(*) FROM fts_messages WHERE fts_messages MATCH 'needle'"); n != 1 {
		t.Errorf("MATCH after rebuild = %d", n)
	}
}

// A sound index is left alone; one far larger than its text is rebuilt,
// which drops the tombstones that rewriting whole sessions left behind.
func TestMaintainFTSRebuildsBloatedIndex(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	commitOne(t, db, "s")
	// What earlier builds did on every change: delete and reinsert every doc.
	for i := range 50 {
		if _, err := db.SQLDB().ExecContext(ctx, "DELETE FROM fts_docs WHERE ref = ?", claudeRef("s")); err != nil {
			t.Fatal(err)
		}
		for m := range 20 {
			if _, err := db.SQLDB().ExecContext(ctx, "INSERT INTO fts_docs (ref, message_index, body) VALUES (?, ?, ?)",
				claudeRef("s"), m, fmt.Sprintf("message %d revision %d words to index", m, i)); err != nil {
				t.Fatal(err)
			}
		}
	}
	blocks := func() int { return countRows(t, db.SQLDB(), "SELECT count(*) FROM fts_messages_data") }
	before := blocks()

	if err := db.maintainFTS(ctx); err != nil {
		t.Fatalf("maintainFTS: %v", err)
	}
	if got := blocks(); got != before {
		t.Fatalf("sound index of a small cache rewritten: %d blocks, was %d", got, before)
	}

	factor, slack := ftsBloatFactor, ftsBloatSlack
	ftsBloatFactor, ftsBloatSlack = 0, 0
	defer func() { ftsBloatFactor, ftsBloatSlack = factor, slack }()
	if err := db.maintainFTS(ctx); err != nil {
		t.Fatalf("maintainFTS: %v", err)
	}
	if got := blocks(); got >= before {
		t.Fatalf("bloated index not rebuilt: %d blocks, was %d", got, before)
	}
	requireFTSSound(t, db.SQLDB())
	if n := countRows(t, db.SQLDB(), "SELECT count(*) FROM fts_messages WHERE fts_messages MATCH 'revision'"); n != 20 {
		t.Errorf("MATCH after rebuild = %d", n)
	}
}
