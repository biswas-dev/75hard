package api

import (
	"context"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/journal"
	"go.uber.org/zap"
)

// journalJob is one uploaded page awaiting transcription.
type journalJob struct {
	userID  int64
	entryID int64
	relPath string
}

// JournalMaxPages bounds how much of a document is read.
//
// Each page is a separate vision call. A journal entry that runs to forty
// pages should cost a few requests, not forty, and the first few pages are
// where the entry actually is.
const JournalMaxPages = 4

// JournalParser transcribes handwritten pages off the request path.
//
// Reading a page of handwriting means rendering it and showing it to a vision
// model, which takes about as long as a food photo — far too long to hold an
// upload open for. The file is stored immediately and the text arrives behind
// it, exactly as a food photo's calories do.
type JournalParser struct {
	srv     *Server
	jobs    chan journalJob
	workers int
	done    chan struct{}
	timeout time.Duration
}

// NewJournalParser builds the transcriber. Nothing runs until Start.
func NewJournalParser(srv *Server, workers, queue int) *JournalParser {
	if workers < 1 {
		workers = 1
	}
	if queue < 1 {
		queue = 32
	}
	return &JournalParser{
		srv: srv, jobs: make(chan journalJob, queue), workers: workers,
		done: make(chan struct{}), timeout: 4 * time.Minute,
	}
}

// Start launches the workers and picks up anything a restart left pending.
func (p *JournalParser) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		go p.run(ctx)
	}
	go p.resume(ctx)
}

// Stop halts the workers. Anything unfinished stays pending and is collected
// on the next boot.
func (p *JournalParser) Stop() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}

// Enqueue submits a page, reporting whether it was accepted. A full queue
// leaves the entry pending rather than blocking the upload.
func (p *JournalParser) Enqueue(job journalJob) bool {
	select {
	case p.jobs <- job:
		return true
	default:
		p.srv.log.Warn("journal parse queue is full; leaving it pending",
			zap.Int64("entry", job.entryID))
		return false
	}
}

func (p *JournalParser) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case job := <-p.jobs:
			p.process(ctx, job)
		}
	}
}

func (p *JournalParser) resume(ctx context.Context) {
	rows, err := p.srv.db.QueryContext(ctx, `
		SELECT id, user_id, rel_path FROM journal_entries
		 WHERE parse_status = 'pending' AND rel_path != '' LIMIT 50`)
	if err != nil {
		return
	}
	var pending []journalJob
	for rows.Next() {
		var j journalJob
		if err := rows.Scan(&j.entryID, &j.userID, &j.relPath); err != nil {
			break
		}
		pending = append(pending, j)
	}
	// Release the cursor before enqueueing: a worker writes to the same
	// database, and holding a read cursor across that has deadlocked this
	// pool before.
	rows.Close()

	for _, j := range pending {
		if !p.Enqueue(j) {
			return
		}
	}
}

func (p *JournalParser) process(ctx context.Context, job journalJob) {
	s := p.srv

	// Detached: the upload response went out long ago, and a shutdown should
	// not cancel a transcription already paid for.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.timeout)
	defer cancel()

	f, err := s.photos.Open(job.relPath)
	if err != nil {
		p.fail(callCtx, job.entryID, "the file could not be read")
		return
	}
	raw, err := readAll(f)
	f.Close()
	if err != nil {
		p.fail(callCtx, job.entryID, "the file could not be read")
		return
	}

	pages, err := journal.Rasterise(raw, JournalMaxPages)
	if err != nil {
		s.log.Warn("rasterising a journal page", zap.Error(err))
		p.fail(callCtx, job.entryID, "the page could not be rendered")
		return
	}

	svc := s.aiForUser(callCtx, job.userID)
	if !svc.Enabled() {
		p.fail(callCtx, job.entryID, "no AI provider is configured")
		return
	}

	var out []string
	for i, page := range pages {
		if used, err := s.aiCallsToday(callCtx, job.userID); err == nil && used >= DailyAILimit {
			p.fail(callCtx, job.entryID, "daily AI limit reached; retry tomorrow")
			return
		}
		text, meta, err := svc.ReadHandwriting(callCtx, page, "image/png")
		s.recordAIRun(callCtx, job.userID, "journal_page", meta, err)
		if err != nil {
			s.log.Warn("reading a journal page",
				zap.Int64("entry", job.entryID), zap.Int("page", i+1), zap.Error(err))
			// Keep whatever earlier pages produced: a partial transcription is
			// worth more than none, and the page images remain either way.
			break
		}
		if t := strings.TrimSpace(text); t != "" {
			out = append(out, t)
		}
	}

	if len(out) == 0 {
		p.fail(callCtx, job.entryID, "nothing readable was found on the page")
		return
	}

	if _, err := s.db.ExecContext(callCtx, `
		UPDATE journal_entries
		   SET parsed_text = ?, parse_status = 'done', parse_error = '',
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, strings.Join(out, "\n\n"), job.entryID); err != nil {
		s.log.Error("saving a transcription", zap.Error(err))
		return
	}
	s.log.Info("journal page transcribed",
		zap.Int64("entry", job.entryID), zap.Int("pages", len(out)))
}

func (p *JournalParser) fail(ctx context.Context, entryID int64, reason string) {
	if _, err := p.srv.db.ExecContext(ctx, `
		UPDATE journal_entries SET parse_status = 'failed', parse_error = ?,
		                           updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, reason, entryID); err != nil {
		p.srv.log.Error("marking a transcription failed", zap.Error(err))
	}
}
