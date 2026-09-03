// Package photo is this app's photo domain — which kinds of photo it stores
// and which poses a progress shot can be tagged with — over the shared ingest
// pipeline in go-photo.
//
// The pipeline itself (sniffing the real type, decoding, downscaling,
// thumbnailing, and writing somewhere a crafted stored name cannot escape)
// lives in github.com/anchoo2kewl/go-photo, so this app and pool.biswas.me
// accept and reject the same files for the same reasons.
//
// The browser already compresses before upload (canvas -> WebP at 1600px), but
// nothing stops a client from posting a 15MB original straight at the API, so
// the same limits are enforced again here.
package photo

import (
	"io"
	"os"
	"strconv"

	gophoto "github.com/anchoo2kewl/go-photo"
)

// ErrUnsupportedType is returned when the bytes are not an image we accept.
var ErrUnsupportedType = gophoto.ErrUnsupportedType

// ErrTooLarge is returned when an upload exceeds the configured ceiling.
var ErrTooLarge = gophoto.ErrTooLarge

// Kinds of photo the app stores.
const (
	KindProgress    = "progress"
	KindFood        = "food"
	KindIngredients = "ingredients"
)

// Poses a progress photo can be taken from.
//
// Three angles rather than two: the side view shows the changes a front shot
// flatters away, and it is the one people skip. Empty is allowed — the daily
// task is satisfied by any photo, and a streak should not depend on
// remembering to tag one.
const (
	PoseFront = "front"
	PoseSide  = "side"
	PoseBack  = "back"
)

// Poses lists the angles offered, in the order they are shown.
func Poses() []string { return []string{PoseFront, PoseSide, PoseBack} }

// ValidPose reports whether pose is one we store. The empty string is valid
// and means "untagged".
func ValidPose(pose string) bool {
	switch pose {
	case "", PoseFront, PoseSide, PoseBack:
		return true
	}
	return false
}

// ValidKind reports whether kind is one we store.
func ValidKind(kind string) bool {
	switch kind {
	case KindProgress, KindFood, KindIngredients:
		return true
	}
	return false
}

// Store writes images beneath a single root directory.
type Store struct {
	inner *gophoto.Store
	opt   gophoto.Options
}

// NewStore creates a Store rooted at dir. maxEdge and thumbEdge are the
// longest-side limits for the stored image and its thumbnail.
func NewStore(dir string, maxEdge, thumbEdge int) (*Store, error) {
	if maxEdge <= 0 {
		maxEdge = gophoto.DefaultMaxEdge
	}
	if thumbEdge <= 0 {
		thumbEdge = 320
	}
	opt := gophoto.Options{
		MaxEdge: maxEdge, ThumbEdge: thumbEdge, Quality: 82,
		// Every stored photo must be one we produced: the re-encode is what
		// strips the EXIF, and a progress photo's EXIF is the GPS coordinates
		// of somebody's bedroom.
		ForceRecompress: true,
	}
	inner, err := gophoto.NewStore(dir, opt)
	if err != nil {
		return nil, err
	}
	return &Store{inner: inner, opt: opt}, nil
}

// Root returns the directory the store writes into.
func (s *Store) Root() string { return s.inner.Root() }

// Saved describes an image that has been written to disk.
type Saved struct {
	RelPath   string
	ThumbPath string
	Mime      string
	Width     int
	Height    int
	Bytes     int64
	SHA256    string
}

// Save decodes r, downscales it to fit maxEdge, writes it plus a thumbnail
// under {userID}/{yyyy}/{mm}/, and reports what it stored.
//
// Everything is re-encoded as JPEG rather than passed through: it strips EXIF
// (which carries GPS coordinates from a phone camera), guarantees the stored
// bytes really are the image we decoded, and gives one predictable format to
// serve.
func (s *Store) Save(r io.Reader, userID int64, kind string, maxBytes int64) (*Saved, error) {
	// The byte ceiling is per-request rather than per-store, because the
	// config can change it without rebuilding the store.
	opt := s.opt
	opt.MaxBytes = maxBytes

	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	img, err := gophoto.Process(raw, "", opt)
	if err != nil {
		return nil, err
	}
	saved, err := s.inner.SaveImage(img, gophoto.Dated(strconv.FormatInt(userID, 10), kind))
	if err != nil {
		return nil, err
	}
	return &Saved{
		RelPath:   saved.RelPath,
		ThumbPath: saved.ThumbPath,
		Mime:      saved.ContentType,
		Width:     saved.Width,
		Height:    saved.Height,
		Bytes:     saved.Bytes(),
		SHA256:    saved.SHA256,
	}, nil
}

// SaveDocument stores a file that is not an image, byte for byte.
//
// A journal page arrives as a PDF, and re-encoding it the way a photo is
// re-encoded would destroy it. go-photo's KeepUnsupported path writes the
// original under the same dated, path-safe naming rule, so a document and a
// photograph are stored and addressed identically.
func (s *Store) SaveDocument(r io.Reader, userID int64, kind, filename string, maxBytes int64) (*Saved, error) {
	opt := s.opt
	opt.MaxBytes = maxBytes
	opt.KeepUnsupported = true

	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, gophoto.ErrTooLarge
	}
	doc, err := gophoto.Process(raw, filename, opt)
	if err != nil {
		return nil, err
	}
	saved, err := s.inner.SaveImage(doc, gophoto.Dated(strconv.FormatInt(userID, 10), kind))
	if err != nil {
		return nil, err
	}
	return &Saved{
		RelPath: saved.RelPath,
		Mime:    saved.ContentType,
		Bytes:   saved.Bytes(),
		SHA256:  saved.SHA256,
	}, nil
}

// Open returns a reader for a stored path, rejecting anything that tries to
// escape the root.
func (s *Store) Open(rel string) (*os.File, error) { return s.inner.Open(rel) }

// Remove deletes stored images, ignoring missing files.
func (s *Store) Remove(paths ...string) { _ = s.inner.Remove(paths...) }
