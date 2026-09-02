package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/photo"
	"github.com/anchoo2kewl/75hard/api/internal/program"
	"github.com/go-chi/chi/v5"
)

// nowFunc is a seam so tests can pin the clock.
var nowFunc = time.Now

// RollDay is one day's worth of progress photos.
type RollDay struct {
	DayNumber int     `json:"day_number"`
	Date      string  `json:"date"`
	Status    string  `json:"status"`
	Photos    []Photo `json:"photos"`
}

// RollResponse is the camera roll: every day that has a photo, newest first.
type RollResponse struct {
	ProgramID  int64     `json:"program_id"`
	StartDate  string    `json:"start_date"`
	LengthDays int       `json:"length_days"`
	CurrentDay int       `json:"current_day"`
	Days       []RollDay `json:"days"`
	Total      int       `json:"total"`
	// Poses present in the roll, so the client only offers filters that
	// would actually return something.
	Poses []string `json:"poses"`
	// FirstByPose and LatestByPose drive the before/after comparison. Keyed by
	// pose, with "" holding untagged photos.
	FirstByPose  map[string]*Photo `json:"first_by_pose"`
	LatestByPose map[string]*Photo `json:"latest_by_pose"`
}

// HandleGetRoll returns the progress-photo camera roll for a program.
//
// Grouped by day rather than returned flat: a progress roll is read as a
// timeline, and the day number is the thing being compared, not the timestamp.
func (s *Server) HandleGetRoll(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	out := RollResponse{
		ProgramID:    programID,
		Days:         []RollDay{},
		Poses:        []string{},
		FirstByPose:  map[string]*Photo{},
		LatestByPose: map[string]*Photo{},
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, length_days FROM programs WHERE id = ?`, programID).
		Scan(&out.StartDate, &out.LengthDays); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return
	}
	out.CurrentDay = program.DayNumber(out.StartDate, program.LocalDate(nowFunc(), s.userLocation(r)))

	poseFilter := strings.TrimSpace(r.URL.Query().Get("pose"))
	if !photo.ValidPose(poseFilter) {
		respondError(w, http.StatusBadRequest, "unknown pose", "invalid_pose")
		return
	}

	query := `
		SELECT p.id, p.kind, p.pose, p.day_id, d.day_number, p.caption, p.width, p.height,
		       p.bytes, p.taken_at, d.on_date, d.status
		FROM photos p
		JOIN days d ON d.id = p.day_id
		WHERE d.program_id = ? AND p.user_id = ? AND p.kind = ?
		ORDER BY d.day_number DESC, p.taken_at ASC, p.id ASC`
	args := []any{programID, UserID(ctx), photo.KindProgress}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load the roll", "internal")
		return
	}
	defer rows.Close()

	seenPose := map[string]bool{}
	byDay := map[int]*RollDay{}
	order := []int{}

	for rows.Next() {
		var p Photo
		var onDate, status string
		if err := rows.Scan(&p.ID, &p.Kind, &p.Pose, &p.DayID, &p.DayNumber, &p.Caption,
			&p.Width, &p.Height, &p.Bytes, &p.TakenAt, &onDate, &status); err != nil {
			respondError(w, http.StatusInternalServerError, "could not read the roll", "internal")
			return
		}
		p.URL = "/api/photos/" + strconv.FormatInt(p.ID, 10) + "/file"
		p.ThumbURL = p.URL + "?size=thumb"

		// Every pose present is offered as a filter, even when the current
		// request is filtered to one of them.
		if !seenPose[p.Pose] {
			seenPose[p.Pose] = true
			out.Poses = append(out.Poses, p.Pose)
		}

		// Track the earliest and latest of each pose for the comparison. Rows
		// arrive newest day first, so the last one seen is the earliest.
		shot := p
		if _, ok := out.LatestByPose[p.Pose]; !ok {
			out.LatestByPose[p.Pose] = &shot
		}
		out.FirstByPose[p.Pose] = &shot

		if poseFilter != "" && p.Pose != poseFilter {
			continue
		}

		n := 0
		if p.DayNumber != nil {
			n = *p.DayNumber
		}
		day, ok := byDay[n]
		if !ok {
			day = &RollDay{DayNumber: n, Date: onDate, Status: status, Photos: []Photo{}}
			byDay[n] = day
			order = append(order, n)
		}
		day.Photos = append(day.Photos, p)
		out.Total++
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "could not read the roll", "internal")
		return
	}

	for _, n := range order {
		out.Days = append(out.Days, *byDay[n])
	}

	respondJSON(w, http.StatusOK, out)
}

type updatePhotoRequest struct {
	Pose    *string `json:"pose"`
	Caption *string `json:"caption"`
}

// HandleUpdatePhoto edits a photo's pose or caption, so an angle can be tagged
// after the fact rather than only at the moment of upload.
func (s *Server) HandleUpdatePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "photoID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid photo id", "invalid_id")
		return
	}
	var req updatePhotoRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	sets := []string{}
	args := []any{}
	if req.Pose != nil {
		pose := strings.TrimSpace(*req.Pose)
		if !photo.ValidPose(pose) {
			respondError(w, http.StatusBadRequest, "unknown pose", "invalid_pose")
			return
		}
		sets, args = append(sets, "pose = ?"), append(args, pose)
	}
	if req.Caption != nil {
		sets, args = append(sets, "caption = ?"), append(args, strings.TrimSpace(*req.Caption))
	}
	if len(sets) == 0 {
		respondError(w, http.StatusBadRequest, "nothing to update", "no_changes")
		return
	}

	args = append(args, id, UserID(r.Context()))
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE photos SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...); err != nil {
		respondError(w, http.StatusInternalServerError, "could not update photo", "internal")
		return
	}

	p, err := s.photoByID(r.Context(), UserID(r.Context()), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "photo not found", "not_found")
		return
	}
	respondJSON(w, http.StatusOK, p)
}
