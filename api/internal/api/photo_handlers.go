package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/photo"
	"github.com/anchoo2kewl/75hard/api/internal/program"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Photo is a stored image as returned to the SPA. Paths on disk are never
// exposed — the client only ever gets ids and the URLs built from them.
type Photo struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Pose      string `json:"pose"`
	DayID     *int64 `json:"day_id"`
	DayNumber *int   `json:"day_number"`
	Caption   string `json:"caption"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Bytes     int64  `json:"bytes"`
	TakenAt   string `json:"taken_at"`
	URL       string `json:"url"`
	ThumbURL  string `json:"thumb_url"`
}

// HandleUploadPhoto accepts a multipart image, compresses and stores it, and
// links it to a day.
//
// A progress photo also satisfies the photo task for that day, which is why
// the upload path recomputes the day status.
func (s *Server) HandleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		respondError(w, http.StatusRequestEntityTooLarge, "image is too large", "too_large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "a file field is required", "missing_file")
		return
	}
	defer file.Close()

	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = photo.KindProgress
	}
	if !photo.ValidKind(kind) {
		respondError(w, http.StatusBadRequest, "unknown photo kind", "invalid_kind")
		return
	}

	pose := strings.TrimSpace(r.FormValue("pose"))
	if !photo.ValidPose(pose) {
		respondError(w, http.StatusBadRequest, "unknown pose", "invalid_pose")
		return
	}

	// Resolve the day this photo belongs to. Defaults to today on the active
	// program so the common case needs no parameters at all.
	var (
		programID *int64
		dayID     *int64
	)
	if pid, err := s.activeProgramID(r); err == nil {
		programID = &pid

		var startDate string
		var length int
		if err := s.db.QueryRowContext(ctx,
			`SELECT start_date, length_days FROM programs WHERE id = ?`, pid).
			Scan(&startDate, &length); err == nil {
			onDate := clampToProgram(startDate, length, program.LocalDate(time.Now(), s.userLocation(r)))
			if raw := strings.TrimSpace(r.FormValue("day_number")); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
					onDate = program.DateForDay(startDate, n)
				}
			}
			if id, err := s.ensureDay(ctx, pid, startDate, onDate); err == nil {
				dayID = &id
			}
		}
	}

	saved, err := s.photos.Save(file, userID, kind, s.cfg.MaxUploadBytes)
	if err != nil {
		if errors.Is(err, photo.ErrUnsupportedType) {
			respondError(w, http.StatusBadRequest, "that file is not a supported image", "invalid_image")
			return
		}
		s.log.Error("save photo", zap.Error(err), zap.String("filename", header.Filename))
		respondError(w, http.StatusInternalServerError, "could not save image", "internal")
		return
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO photos (user_id, program_id, day_id, kind, pose, rel_path, thumb_path,
		                    mime, width, height, bytes, sha256, caption)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, programID, dayID, kind, pose, saved.RelPath, saved.ThumbPath, saved.Mime,
		saved.Width, saved.Height, saved.Bytes, saved.SHA256, strings.TrimSpace(r.FormValue("caption")))
	if err != nil {
		// Don't leave bytes on disk that no row points at.
		s.photos.Remove(saved.RelPath, saved.ThumbPath)
		s.log.Error("insert photo", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not save image", "internal")
		return
	}
	id, _ := res.LastInsertId()

	if programID != nil && dayID != nil && kind == photo.KindProgress {
		if err := s.refreshDayStatus(r, *programID, *dayID); err != nil {
			s.log.Error("refresh day after photo", zap.Error(err))
		}
	}

	p, err := s.photoByID(ctx, userID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load image", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, p)
}

// HandleServePhoto streams a stored image. Ownership is checked on every
// request — the id alone is never authorisation.
func (s *Server) HandleServePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "photoID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid photo id", "invalid_id")
		return
	}

	var relPath, thumbPath, mime string
	err = s.db.QueryRowContext(r.Context(),
		`SELECT rel_path, thumb_path, mime FROM photos WHERE id = ? AND user_id = ?`,
		id, UserID(r.Context())).Scan(&relPath, &thumbPath, &mime)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "photo not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load photo", "internal")
		return
	}

	path := relPath
	if r.URL.Query().Get("size") == "thumb" && thumbPath != "" {
		path = thumbPath
	}

	f, err := s.photos.Open(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "photo file missing", "not_found")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", mime)
	// Private: these are progress photos behind auth, and must never be held
	// by a shared cache.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if info, err := f.Stat(); err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}
	if _, err := io.Copy(w, f); err != nil {
		s.log.Warn("stream photo", zap.Error(err))
	}
}

// HandleListPhotos returns the caller's photos, newest first, optionally
// filtered by kind.
func (s *Server) HandleListPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	query := `
		SELECT p.id, p.kind, p.pose, p.day_id, d.day_number, p.caption, p.width, p.height,
		       p.bytes, p.taken_at
		FROM photos p
		LEFT JOIN days d ON d.id = p.day_id
		WHERE p.user_id = ?`
	args := []any{UserID(ctx)}

	if kind := r.URL.Query().Get("kind"); kind != "" {
		if !photo.ValidKind(kind) {
			respondError(w, http.StatusBadRequest, "unknown photo kind", "invalid_kind")
			return
		}
		query += ` AND p.kind = ?`
		args = append(args, kind)
	}
	if pose := r.URL.Query().Get("pose"); pose != "" {
		if !photo.ValidPose(pose) {
			respondError(w, http.StatusBadRequest, "unknown pose", "invalid_pose")
			return
		}
		query += ` AND p.pose = ?`
		args = append(args, pose)
	}
	query += ` ORDER BY p.taken_at DESC, p.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list photos", "internal")
		return
	}
	defer rows.Close()

	out := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not read photos", "internal")
			return
		}
		out = append(out, p)
	}
	respondJSON(w, http.StatusOK, out)
}

// HandleDeletePhoto removes a photo and its file.
func (s *Server) HandleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "photoID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid photo id", "invalid_id")
		return
	}
	ctx := r.Context()

	var relPath, thumbPath string
	var programID, dayID *int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT rel_path, thumb_path, program_id, day_id FROM photos WHERE id = ? AND user_id = ?`,
		id, UserID(ctx)).Scan(&relPath, &thumbPath, &programID, &dayID); err != nil {
		respondError(w, http.StatusNotFound, "photo not found", "not_found")
		return
	}

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM photos WHERE id = ? AND user_id = ?`, id, UserID(ctx)); err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete photo", "internal")
		return
	}
	s.photos.Remove(relPath, thumbPath)

	// Removing the day's only progress photo can un-complete the day.
	if programID != nil && dayID != nil {
		if err := s.refreshDayStatus(r, *programID, *dayID); err != nil {
			s.log.Error("refresh day after photo delete", zap.Error(err))
		}
	}

	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) photoByID(ctx context.Context, userID, id int64) (Photo, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT p.id, p.kind, p.pose, p.day_id, d.day_number, p.caption, p.width, p.height,
		       p.bytes, p.taken_at
		FROM photos p
		LEFT JOIN days d ON d.id = p.day_id
		WHERE p.id = ? AND p.user_id = ?`, id, userID)
	return scanPhoto(row)
}

func (s *Server) photosForDay(ctx context.Context, dayID int64) ([]Photo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.kind, p.pose, p.day_id, d.day_number, p.caption, p.width, p.height,
		       p.bytes, p.taken_at
		FROM photos p
		LEFT JOIN days d ON d.id = p.day_id
		WHERE p.day_id = ?
		ORDER BY p.taken_at DESC, p.id DESC`, dayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPhoto(row scanner) (Photo, error) {
	var p Photo
	if err := row.Scan(&p.ID, &p.Kind, &p.Pose, &p.DayID, &p.DayNumber, &p.Caption,
		&p.Width, &p.Height, &p.Bytes, &p.TakenAt); err != nil {
		return p, err
	}
	p.URL = "/api/photos/" + strconv.FormatInt(p.ID, 10) + "/file"
	p.ThumbURL = p.URL + "?size=thumb"
	return p, nil
}
