package api

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/journal"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// MaxJournalUpload bounds an uploaded page.
//
// A phone photographing a page of handwriting produces a few megabytes; twenty
// leaves room for a multi-page scan without letting somebody park a film in
// the photo volume.
const MaxJournalUpload = 20 << 20

// JournalEntry is one entry as returned to the client.
type JournalEntry struct {
	ID        int64  `json:"id"`
	DayID     *int64 `json:"day_id,omitempty"`
	DayNumber *int   `json:"day_number,omitempty"`
	Title     string `json:"title"`
	// Kind is typed or pdf.
	Kind string `json:"kind"`
	Body string `json:"body"`
	// FileName and PageCount describe an upload.
	FileName  string `json:"file_name,omitempty"`
	FileBytes int64  `json:"file_bytes,omitempty"`
	PageCount int    `json:"page_count,omitempty"`
	// ParsedText is what was read out of an upload. Kept apart from Body so a
	// machine transcription can never be mistaken for what somebody wrote.
	ParsedText  string    `json:"parsed_text,omitempty"`
	ParseStatus string    `json:"parse_status"`
	ParseError  string    `json:"parse_error,omitempty"`
	HasFile     bool      `json:"has_file"`
	CreatedAt   time.Time `json:"created_at"`
	// Snippet is set on search results: the matching text with the query
	// marked, so a result can be judged without opening it.
	Snippet string `json:"snippet,omitempty"`
}

type createJournalRequest struct {
	DayNumber *int   `json:"day_number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

// HandleCreateJournal writes a typed entry.
func (s *Server) HandleCreateJournal(w http.ResponseWriter, r *http.Request) {
	var req createJournalRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	body := strings.TrimSpace(req.Body)
	if body == "" {
		respondError(w, http.StatusBadRequest, "an entry needs something in it", "empty_entry")
		return
	}

	_, dayID, ok := s.resolveDay(w, r, req.DayNumber)
	if !ok {
		return
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO journal_entries (user_id, day_id, title, kind, body)
		VALUES (?, ?, ?, 'typed', ?)`,
		userID, dayID, strings.TrimSpace(req.Title), body)
	if err != nil {
		s.log.Error("create journal entry", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not save the entry", "internal")
		return
	}
	id, _ := res.LastInsertId()

	entry, err := s.journalByID(ctx, userID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load the entry", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, entry)
}

type updateJournalRequest struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

// HandleUpdateJournal edits a typed entry.
//
// Only the title and body: the transcription of an upload is not editable
// here, because correcting a machine's reading of a page and rewriting what
// you wrote are different acts and merging them loses which is which.
func (s *Server) HandleUpdateJournal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "entryID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid entry id", "invalid_id")
		return
	}
	var req updateJournalRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()

	sets, args := []string{}, []any{}
	if req.Title != nil {
		sets, args = append(sets, "title = ?"), append(args, strings.TrimSpace(*req.Title))
	}
	if req.Body != nil {
		sets, args = append(sets, "body = ?"), append(args, strings.TrimSpace(*req.Body))
	}
	if len(sets) == 0 {
		respondError(w, http.StatusBadRequest, "nothing to update", "no_changes")
		return
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id, UserID(ctx))

	if _, err := s.db.ExecContext(ctx,
		`UPDATE journal_entries SET `+strings.Join(sets, ", ")+
			` WHERE id = ? AND user_id = ?`, args...); err != nil {
		respondError(w, http.StatusInternalServerError, "could not update the entry", "internal")
		return
	}

	entry, err := s.journalByID(ctx, UserID(ctx), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "entry not found", "not_found")
		return
	}
	respondJSON(w, http.StatusOK, entry)
}

// HandleListJournal returns entries, newest first, or the results of a search.
func (s *Server) HandleListJournal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		s.searchJournal(w, r, userID, q)
		return
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT j.id, j.day_id, d.day_number, j.title, j.kind, j.body,
		       j.file_name, j.file_bytes, j.page_count, j.parsed_text,
		       j.parse_status, j.parse_error, j.rel_path != '', j.created_at
		  FROM journal_entries j
		  LEFT JOIN days d ON d.id = j.day_id
		 WHERE j.user_id = ?
		 ORDER BY j.created_at DESC
		 LIMIT 200`, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list entries", "internal")
		return
	}
	defer rows.Close()

	out := []JournalEntry{}
	for rows.Next() {
		e, err := scanJournal(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not read entries", "internal")
			return
		}
		out = append(out, e)
	}
	respondJSON(w, http.StatusOK, out)
}

// searchJournal runs a full-text query across typed entries and transcriptions.
func (s *Server) searchJournal(w http.ResponseWriter, r *http.Request, userID int64, q string) {
	// FTS5 treats punctuation as syntax, so an apostrophe or a colon in a
	// perfectly ordinary sentence becomes a query error. Quoting each word
	// turns the input into a plain phrase search, which is what somebody
	// typing into a search box means.
	terms := strings.Fields(q)
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(t, `"`, "")+`"`)
	}
	if len(quoted) == 0 {
		respondJSON(w, http.StatusOK, []JournalEntry{})
		return
	}
	match := strings.Join(quoted, " ")

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT j.id, j.day_id, d.day_number, j.title, j.kind, j.body,
		       j.file_name, j.file_bytes, j.page_count, j.parsed_text,
		       j.parse_status, j.parse_error, j.rel_path != '', j.created_at,
		       snippet(journal_fts, 1, '[', ']', '…', 12)
		  FROM journal_fts
		  JOIN journal_entries j ON j.id = journal_fts.entry_id
		  LEFT JOIN days d ON d.id = j.day_id
		 WHERE journal_fts MATCH ? AND j.user_id = ?
		 ORDER BY rank
		 LIMIT 100`, match, userID)
	if err != nil {
		s.log.Warn("journal search", zap.String("query", q), zap.Error(err))
		respondError(w, http.StatusBadRequest, "could not search for that", "bad_query")
		return
	}
	defer rows.Close()

	out := []JournalEntry{}
	for rows.Next() {
		e, err := scanJournalWithSnippet(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not read results", "internal")
			return
		}
		out = append(out, e)
	}
	respondJSON(w, http.StatusOK, out)
}

// HandleDeleteJournal removes an entry and any file behind it.
func (s *Server) HandleDeleteJournal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "entryID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid entry id", "invalid_id")
		return
	}
	ctx := r.Context()

	var relPath string
	_ = s.db.QueryRowContext(ctx,
		`SELECT rel_path FROM journal_entries WHERE id = ? AND user_id = ?`,
		id, UserID(ctx)).Scan(&relPath)

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM journal_entries WHERE id = ? AND user_id = ?`, id, UserID(ctx)); err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete the entry", "internal")
		return
	}
	// The row is gone either way; a file left behind is waste, not a failure.
	if relPath != "" {
		s.photos.Remove(relPath, "")
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleServeJournalFile streams an uploaded page back to its owner.
func (s *Server) HandleServeJournalFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "entryID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid entry id", "invalid_id")
		return
	}

	var relPath, fileName string
	err = s.db.QueryRowContext(r.Context(),
		`SELECT rel_path, file_name FROM journal_entries WHERE id = ? AND user_id = ?`,
		id, UserID(r.Context())).Scan(&relPath, &fileName)
	if errors.Is(err, sql.ErrNoRows) || relPath == "" {
		respondError(w, http.StatusNotFound, "no file for that entry", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load the entry", "internal")
		return
	}

	f, err := s.photos.Open(relPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "the file is missing", "not_found")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/pdf")
	// Private: a journal is the most personal thing in here and must never sit
	// in a shared cache.
	w.Header().Set("Cache-Control", "private, max-age=300")
	if fileName != "" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("inline; filename=%q", fileName))
	}
	// ServeContent gives range requests, which a PDF viewer uses to page
	// through a large file without pulling the whole thing.
	http.ServeContent(w, r, fileName, time.Time{}, f)
}

func (s *Server) journalByID(ctx context.Context, userID, id int64) (JournalEntry, error) {
	return scanJournal(s.db.QueryRowContext(ctx, `
		SELECT j.id, j.day_id, d.day_number, j.title, j.kind, j.body,
		       j.file_name, j.file_bytes, j.page_count, j.parsed_text,
		       j.parse_status, j.parse_error, j.rel_path != '', j.created_at
		  FROM journal_entries j
		  LEFT JOIN days d ON d.id = j.day_id
		 WHERE j.id = ? AND j.user_id = ?`, id, userID))
}

func scanJournal(row scanner) (JournalEntry, error) {
	var e JournalEntry
	var dayID sql.NullInt64
	var dayNumber sql.NullInt64
	var hasFile int
	err := row.Scan(&e.ID, &dayID, &dayNumber, &e.Title, &e.Kind, &e.Body,
		&e.FileName, &e.FileBytes, &e.PageCount, &e.ParsedText,
		&e.ParseStatus, &e.ParseError, &hasFile, &e.CreatedAt)
	if dayID.Valid {
		e.DayID = &dayID.Int64
	}
	if dayNumber.Valid {
		n := int(dayNumber.Int64)
		e.DayNumber = &n
	}
	e.HasFile = hasFile == 1
	return e, err
}

func scanJournalWithSnippet(row scanner) (JournalEntry, error) {
	var e JournalEntry
	var dayID, dayNumber sql.NullInt64
	var hasFile int
	err := row.Scan(&e.ID, &dayID, &dayNumber, &e.Title, &e.Kind, &e.Body,
		&e.FileName, &e.FileBytes, &e.PageCount, &e.ParsedText,
		&e.ParseStatus, &e.ParseError, &hasFile, &e.CreatedAt, &e.Snippet)
	if dayID.Valid {
		e.DayID = &dayID.Int64
	}
	if dayNumber.Valid {
		n := int(dayNumber.Int64)
		e.DayNumber = &n
	}
	e.HasFile = hasFile == 1
	return e, err
}

// HandleUploadJournal stores a PDF page and reads what it can out of it.
//
// A typed PDF carries its text and is parsed on the spot, exactly and for
// nothing. A photographed or scanned page carries only a picture of writing,
// and reading that needs a vision model — so it is queued rather than done
// here, because nobody should wait on a model to find out their upload
// succeeded.
func (s *Server) HandleUploadJournal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	r.Body = http.MaxBytesReader(w, r.Body, MaxJournalUpload+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		respondError(w, http.StatusRequestEntityTooLarge, "that file is too large", "too_large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "a file field is required", "missing_file")
		return
	}
	defer file.Close()

	raw, err := readAll(file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not read the file", "internal")
		return
	}
	// The declared content type is client-controlled and therefore not
	// evidence; the magic number is.
	if !journal.LooksLikePDF(raw) {
		respondError(w, http.StatusBadRequest, "that file is not a PDF", "not_pdf")
		return
	}

	var dayNumber *int
	if raw := strings.TrimSpace(r.FormValue("day_number")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			dayNumber = &n
		}
	}
	_, dayID, ok := s.resolveDay(w, r, dayNumber)
	if !ok {
		return
	}

	// Extraction first: when it works it is exact, instant and free.
	text, err := journal.ExtractText(raw)
	if errors.Is(err, journal.ErrEncrypted) {
		respondError(w, http.StatusBadRequest,
			"that PDF is password protected", "pdf_encrypted")
		return
	}

	saved, err := s.photos.SaveDocument(bytesReader(raw), userID, "journal",
		header.Filename, MaxJournalUpload)
	if err != nil {
		s.log.Error("save journal file", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not store the file", "internal")
		return
	}

	status, parsed := "", text.Content
	if text.NeedsVision {
		// Queued for a vision model, if this account has one and the host can
		// render a page. Otherwise the page is kept and simply not searchable.
		parsed = ""
		if s.journalParser != nil && s.aiForUser(ctx, userID).Enabled() && journal.RasterAvailable() {
			status = "pending"
		} else {
			status = "failed"
		}
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO journal_entries
			(user_id, day_id, title, kind, rel_path, file_name, file_bytes,
			 page_count, parsed_text, parse_status, parse_error)
		VALUES (?, ?, ?, 'pdf', ?, ?, ?, ?, ?, ?, ?)`,
		userID, dayID, strings.TrimSpace(r.FormValue("title")),
		saved.RelPath, header.Filename, saved.Bytes, text.Pages, parsed, status,
		failureReason(status, s, ctx, userID))
	if err != nil {
		s.photos.Remove(saved.RelPath)
		s.log.Error("create journal upload", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not save the entry", "internal")
		return
	}
	id, _ := res.LastInsertId()

	if status == "pending" {
		s.journalParser.Enqueue(journalJob{
			userID: userID, entryID: id, relPath: saved.RelPath,
		})
	}

	entry, err := s.journalByID(ctx, userID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load the entry", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, entry)
}

// failureReason explains why a handwritten page will not be transcribed, so
// the entry says something more useful than "failed".
func failureReason(status string, s *Server, ctx context.Context, userID int64) string {
	if status != "failed" {
		return ""
	}
	if !s.aiForUser(ctx, userID).Enabled() {
		return "no AI provider is configured, so handwriting cannot be read"
	}
	if !journal.RasterAvailable() {
		return "this server cannot render PDF pages, so handwriting cannot be read"
	}
	return "handwriting could not be read"
}

// bytesReader adapts a byte slice for the store, which takes a reader.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
