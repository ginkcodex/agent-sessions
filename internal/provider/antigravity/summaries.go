package antigravity

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider/sqliteread"
)

// row is what a session uses of its conversation_summaries row. Scan keeps
// it in the checkpoint, so a locked index reuses the last row read.
type row struct {
	Title        string    `json:"title,omitempty"`
	Preview      string    `json:"preview,omitempty"`
	Status       string    `json:"status,omitempty"`
	CWD          string    `json:"cwd,omitempty"`
	AgentName    string    `json:"agentName,omitempty"`
	ParentID     string    `json:"parentId,omitempty"`
	Modified     time.Time `json:"modified"`
	NotFullyIdle bool      `json:"notFullyIdle,omitempty"`
	Killed       bool      `json:"killed,omitempty"`
}

func sameRow(a, b *row) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, y := *a, *b
	x.Modified, y.Modified = time.Time{}, time.Time{}
	return x == y && a.Modified.Equal(b.Modified)
}

// summaryColumns are read when present; a missing one reads as NULL.
var summaryColumns = []string{
	"conversation_id", "title", "preview", "status", "workspace_uris", "agent_name",
	"parent_conversation_id", "last_modified_time", "not_fully_idle", "killed",
}

// readSummaries reads every row of conversation_summaries by session ID. A
// missing index is no rows and no error.
func (p *Provider) readSummaries(ctx context.Context) (map[string]row, error) {
	rows := map[string]row{}
	if _, err := os.Stat(p.dbPath()); os.IsNotExist(err) {
		return rows, nil
	}
	db, err := sqliteread.Open(ctx, p.dbPath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	columns, err := sqliteread.Columns(ctx, db, "conversation_summaries")
	if err != nil {
		return nil, err
	}
	if !columns["conversation_id"] {
		return nil, fmt.Errorf("conversation_summaries table or its ID column is missing")
	}
	fields := make([]string, len(summaryColumns))
	for i, name := range summaryColumns {
		if columns[name] {
			fields[i] = "`" + name + "`"
		} else {
			fields[i] = "NULL"
		}
	}
	result, err := db.QueryContext(ctx, "SELECT "+strings.Join(fields, ", ")+" FROM conversation_summaries")
	if err != nil {
		return nil, fmt.Errorf("query conversation_summaries: %w", err)
	}
	for result.Next() {
		var id, title, preview, status, uris, agent, parent sql.NullString
		var modified any
		var notIdle, killed sql.NullBool
		if err = result.Scan(&id, &title, &preview, &status, &uris, &agent, &parent, &modified, &notIdle, &killed); err != nil {
			break
		}
		if id.String == "" {
			continue
		}
		rows[id.String] = row{
			Title: title.String, Preview: preview.String, Status: status.String,
			CWD: firstWorkspace(uris.String), AgentName: agent.String, ParentID: parent.String,
			Modified: dbTime(modified), NotFullyIdle: notIdle.Bool, Killed: killed.Bool,
		}
	}
	if err == nil {
		err = result.Err()
	}
	closeErr := result.Close() // free the single connection for the next query
	if err != nil {
		return nil, fmt.Errorf("read conversation_summaries: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close conversation_summaries: %w", closeErr)
	}
	if err := readParents(ctx, db, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// readParents fills a parent missing from parent_conversation_id from the
// conversation_parents table (empty in every store seen so far).
func readParents(ctx context.Context, db *sql.DB, rows map[string]row) error {
	columns, err := sqliteread.Columns(ctx, db, "conversation_parents")
	if err != nil {
		return err
	}
	if !columns["conversation_id"] || !columns["parent_id"] {
		return nil
	}
	result, err := db.QueryContext(ctx, "SELECT conversation_id, parent_id FROM conversation_parents")
	if err != nil {
		return fmt.Errorf("query conversation_parents: %w", err)
	}
	defer func() { _ = result.Close() }()
	for result.Next() {
		var id, parent sql.NullString
		if err := result.Scan(&id, &parent); err != nil {
			return fmt.Errorf("read conversation_parents: %w", err)
		}
		if r, ok := rows[id.String]; ok && r.ParentID == "" {
			r.ParentID = parent.String
			rows[id.String] = r
		}
	}
	return result.Err()
}

// dbTime reads a datetime column. agy writes text such as
// "2026-11-19 01:50:40.875170637+00:00"; the driver may already parse it.
func dbTime(v any) time.Time {
	var s string
	switch v := v.(type) {
	case time.Time:
		return v.UTC()
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
