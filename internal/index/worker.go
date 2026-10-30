package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

const (
	ftsQuietPeriod   = 500 * time.Millisecond
	ftsMaxDelay      = 2 * time.Second
	ftsPollInterval  = 150 * time.Millisecond
	ftsProgressEvery = 250 * time.Millisecond
)

var errDBClosed = errors.New("index: database closed")

type ftsJob struct {
	ref       string
	revision  int64
	updatedAt int64
	attempts  int
}

// TranscriptLoader loads a session's transcript for indexing. The result must
// be no older than the session's source when LoadForIndex was called: a job is
// queued after the scan that saw the change, so any later read covers it.
type TranscriptLoader interface {
	LoadForIndex(ctx context.Context, ref model.SessionRef) (*model.Transcript, error)
}

// ProviderLoader loads transcripts directly from providers, parsing each
// session in full.
func ProviderLoader(providers provider.Set) TranscriptLoader {
	return providerLoader{providers}
}

type providerLoader struct{ providers provider.Set }

func (l providerLoader) LoadForIndex(ctx context.Context, ref model.SessionRef) (*model.Transcript, error) {
	p, ok := l.providers.Get(ref.Agent)
	if !ok {
		return nil, fmt.Errorf("index: no provider for %s", ref.Agent)
	}
	return p.Load(ctx, ref)
}

// Indexer runs one background FTS worker, loading transcripts outside write
// transactions. Jobs are persisted in SQLite, so shutdown/restart resumes at
// the next revision without losing work.
type Indexer struct {
	db     *DB
	loader TranscriptLoader
	emit   func(FTSProgress)

	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}
	once   sync.Once
}

// StartIndexer begins indexing pending jobs. Stop before closing the database.
func StartIndexer(ctx context.Context, db *DB, loader TranscriptLoader, emit func(FTSProgress)) *Indexer {
	ctx, cancel := context.WithCancel(ctx)
	w := &Indexer{
		db: db, loader: loader, emit: emit, cancel: cancel,
		done: make(chan struct{}), wake: make(chan struct{}, 1),
	}
	go func() {
		defer close(w.done)
		w.run(ctx)
	}()
	return w
}

// Notify prompts a queue check after a scanner commit. The periodic check
// still catches jobs after a restart or commits without a notification.
func (w *Indexer) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Close cancels the worker and waits until it has stopped using the DB.
func (w *Indexer) Close() {
	if w == nil {
		return
	}
	w.once.Do(w.cancel)
	<-w.done
}

// IndexPending processes all jobs present, without starting a long-lived
// goroutine. It is useful when importing or rebuilding an index from a CLI;
// it honors the same debounce and cancellation rules as the background worker.
func (d *DB) IndexPending(ctx context.Context, loader TranscriptLoader, emit func(FTSProgress)) error {
	w := &Indexer{db: d, loader: loader, emit: emit}
	return w.processUntilEmpty(ctx)
}

func (w *Indexer) run(ctx context.Context) {
	ticker := time.NewTicker(ftsPollInterval)
	defer ticker.Stop()
	firstSeen := make(map[string]time.Time)
	retryAt := make(map[string]time.Time)
	var lastEmit time.Time
	var lastProgress FTSProgress
	emitProgress := func(running, force bool) {
		if w.emit == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastEmit) < ftsProgressEvery {
			return
		}
		p, err := w.db.ftsProgress(ctx, running)
		if err != nil {
			return
		}
		if !force && p == lastProgress {
			return
		}
		lastEmit, lastProgress = now, p
		w.emit(p)
	}
	emitProgress(false, true)
	_ = w.db.maintainFTS(ctx)
	for ctx.Err() == nil {
		jobs, err := w.db.pendingFTSJobs(ctx)
		if err == nil {
			now := time.Now()
			present := make(map[string]struct{}, len(jobs))
			for _, job := range jobs {
				present[job.ref] = struct{}{}
				if _, ok := firstSeen[job.ref]; !ok {
					firstSeen[job.ref] = now
				}
			}
			for ref := range firstSeen {
				if _, ok := present[ref]; !ok {
					delete(firstSeen, ref)
					delete(retryAt, ref)
				}
			}
			// Newest eligible job wins, but pending debounce and backoff
			// never block another session's eligible job.
			for _, job := range jobs {
				if until := retryAt[job.ref]; now.Before(until) {
					continue
				}
				if now.Sub(time.UnixMilli(job.updatedAt)) < ftsQuietPeriod && now.Sub(firstSeen[job.ref]) < ftsMaxDelay {
					continue
				}
				emitProgress(true, true)
				err := w.processJob(ctx, job)
				if err != nil && ctx.Err() == nil {
					_ = w.db.failFTSJob(ctx, job, err)
					// Retry failures with exponential backoff, capped at 30s.
					shift := min(job.attempts, 5)
					retryAt[job.ref] = now.Add(min(time.Second<<shift, 30*time.Second))
				} else {
					delete(firstSeen, job.ref)
					delete(retryAt, job.ref)
				}
				emitProgress(false, false)
				break
			}
		}
		emitProgress(false, false)
		select {
		case <-ctx.Done():
		case <-ticker.C:
		case <-w.wake:
		}
	}
	// Use a fresh bounded context for final progress after cancellation.
	if w.emit != nil {
		finalCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if p, err := w.db.ftsProgress(finalCtx, false); err == nil {
			w.emit(p)
		}
	}
}

func (w *Indexer) processUntilEmpty(ctx context.Context) error {
	firstSeen := make(map[string]time.Time)
	for {
		jobs, err := w.db.pendingFTSJobs(ctx)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		now := time.Now()
		for _, job := range jobs {
			if _, ok := firstSeen[job.ref]; !ok {
				firstSeen[job.ref] = now
			}
			if now.Sub(time.UnixMilli(job.updatedAt)) < ftsQuietPeriod && now.Sub(firstSeen[job.ref]) < ftsMaxDelay {
				continue
			}
			if err := w.processJob(ctx, job); err != nil {
				_ = w.db.failFTSJob(ctx, job, err)
				return err
			}
			delete(firstSeen, job.ref)
			if w.emit != nil {
				if p, err := w.db.ftsProgress(ctx, false); err == nil {
					w.emit(p)
				}
			}
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ftsPollInterval):
		}
	}
}

func (d *DB) pendingFTSJobs(ctx context.Context) ([]ftsJob, error) {
	if d.db == nil {
		return nil, errDBClosed
	}
	rows, err := d.db.QueryContext(ctx, `SELECT ref, revision, updated_at, attempts FROM fts_jobs ORDER BY updated_at DESC, ref`)
	if err != nil {
		return nil, fmt.Errorf("index: read fts jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []ftsJob
	for rows.Next() {
		var job ftsJob
		if err := rows.Scan(&job.ref, &job.revision, &job.updatedAt, &job.attempts); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (w *Indexer) processJob(ctx context.Context, job ftsJob) error {
	ref, err := parseFTSRef(job.ref)
	if err != nil {
		return err
	}
	tr, err := w.loader.LoadForIndex(ctx, ref)
	if err != nil {
		return fmt.Errorf("index: load %s for FTS: %w", job.ref, err)
	}
	if tr == nil {
		return fmt.Errorf("index: load %s returned nil transcript", job.ref)
	}
	// Cache metadata comes from the scan, not Load: Load's title can lag
	// behind a newer metadata scan. The revision guard protects the write.
	return w.db.replaceFTSDocs(ctx, job, tr)
}

func parseFTSRef(key string) (model.SessionRef, error) {
	agent, id, ok := strings.Cut(key, ":")
	if !ok || agent == "" || id == "" {
		return model.SessionRef{}, fmt.Errorf("index: invalid session ref %q", key)
	}
	return model.SessionRef{Agent: model.AgentID(agent), ID: id}, nil
}

func (d *DB) replaceFTSDocs(ctx context.Context, job ftsJob, tr *model.Transcript) error {
	// Text extraction is done before acquiring the write lock, outside the
	// SQLite transaction. Large transcript parsing never holds a DB write.
	docs := buildFTSDocs(job.ref, tr)
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if d.db == nil {
		return errDBClosed
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("index: begin fts replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current, queued int64
	err = tx.QueryRowContext(ctx, `
SELECT s.fts_revision, j.revision FROM sessions s
JOIN fts_jobs j ON j.ref=s.ref WHERE s.ref=?`, job.ref).Scan(&current, &queued)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (current != job.revision || queued != job.revision)) {
		// The ref was deleted or a newer scan enqueued a different revision;
		// its fresh job (and existing docs) must not be touched.
		return nil
	}
	if err != nil {
		return fmt.Errorf("index: check fts revision: %w", err)
	}
	// Only docs that changed are written. Every FTS5 delete leaves a
	// tombstone in the index until a merge drops it, so rewriting all of a
	// growing session's messages on each change bloats the index without bound.
	old, err := storedFTSDocs(ctx, tx, job.ref)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		prev, ok := old[doc.MessageIndex]
		delete(old, doc.MessageIndex)
		switch {
		case !ok:
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO fts_docs (ref, message_index, kind, body) VALUES (?, ?, ?, ?)",
				doc.Ref, doc.MessageIndex, doc.Kind, doc.Body); err != nil {
				return fmt.Errorf("index: insert fts doc: %w", err)
			}
		case prev.kind != doc.Kind || prev.body != doc.Body:
			if _, err := tx.ExecContext(ctx,
				"UPDATE fts_docs SET kind = ?, body = ? WHERE rowid = ?",
				doc.Kind, doc.Body, prev.rowid); err != nil {
				return fmt.Errorf("index: update fts doc: %w", err)
			}
		}
	}
	for _, prev := range old {
		if _, err := tx.ExecContext(ctx, "DELETE FROM fts_docs WHERE rowid = ?", prev.rowid); err != nil {
			return fmt.Errorf("index: delete old fts doc: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET fts_indexed_revision = ? WHERE ref = ?", job.revision, job.ref); err != nil {
		return fmt.Errorf("index: mark fts revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM fts_jobs WHERE ref = ? AND revision = ?", job.ref, job.revision); err != nil {
		return fmt.Errorf("index: dequeue fts job: %w", err)
	}
	return tx.Commit()
}

// A sound full-text index, prefix indexes included, takes a few times the
// text it holds. A database much larger than that is mostly tombstones, as
// builds that rewrote every doc of a session on each change left behind.
var (
	ftsBloatFactor int64 = 16
	ftsBloatSlack  int64 = 64 << 20
)

// maintainFTS rebuilds the full-text index from fts_docs when it is bloated
// or fails FTS5's own integrity check, which Open leaves out because it reads
// the whole index. A rebuild only reads fts_docs, so it is cheap however big
// the old index is. Run by the indexer before its first job.
func (d *DB) maintainFTS(ctx context.Context) error {
	if d.db == nil {
		return errDBClosed
	}
	var used, text int64
	if err := d.db.QueryRowContext(ctx, `
SELECT (p.page_count - f.freelist_count) * s.page_size
FROM pragma_page_count() p, pragma_freelist_count() f, pragma_page_size() s`).Scan(&used); err != nil {
		return fmt.Errorf("index: measure database: %w", err)
	}
	if err := d.db.QueryRowContext(ctx,
		"SELECT coalesce(sum(length(CAST(body AS BLOB))), 0) FROM fts_docs").Scan(&text); err != nil {
		return fmt.Errorf("index: measure fts text: %w", err)
	}
	if used <= ftsBloatFactor*text+ftsBloatSlack {
		// Damage can also surface as an SQLITE_CORRUPT error, not a report.
		if err := quickCheck(ctx, d.db, "fts_messages"); err == nil || ctx.Err() != nil {
			return err
		}
	}
	return d.rebuildFTS(ctx)
}

// rebuildFTS replaces the full-text index with one built from fts_docs, and
// truncates the write-ahead log, which by then holds the zeroed old pages.
func (d *DB) rebuildFTS(ctx context.Context) error {
	if d.readOnly {
		return errReadOnly
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if d.db == nil {
		return errDBClosed
	}
	if _, err := d.db.ExecContext(ctx, "INSERT INTO fts_messages(fts_messages) VALUES('rebuild')"); err != nil {
		return fmt.Errorf("index: rebuild full-text index: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("index: checkpoint: %w", err)
	}
	return nil
}

type storedFTSDoc struct {
	rowid      int64
	kind, body string
}

// storedFTSDocs returns ref's indexed docs by message index.
func storedFTSDocs(ctx context.Context, tx *sql.Tx, ref string) (map[int]storedFTSDoc, error) {
	rows, err := tx.QueryContext(ctx, "SELECT rowid, message_index, kind, body FROM fts_docs WHERE ref = ?", ref)
	if err != nil {
		return nil, fmt.Errorf("index: read old fts docs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int]storedFTSDoc)
	for rows.Next() {
		var doc storedFTSDoc
		var idx int
		if err := rows.Scan(&doc.rowid, &idx, &doc.kind, &doc.body); err != nil {
			return nil, fmt.Errorf("index: scan old fts doc: %w", err)
		}
		out[idx] = doc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("index: read old fts docs: %w", err)
	}
	return out, nil
}

func (d *DB) failFTSJob(ctx context.Context, job ftsJob, loadErr error) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if d.db == nil {
		return errDBClosed
	}
	msg := truncateUTF8(loadErr.Error(), 500)
	_, err := d.db.ExecContext(ctx, `
UPDATE fts_jobs SET attempts = attempts + 1, last_error = ?
WHERE ref = ? AND revision = ?`, msg, job.ref, job.revision)
	return err
}
