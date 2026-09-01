// Package photo handles image ingest: sniffing the real type, decoding,
// downscaling, thumbnailing and writing to the photo volume.
//
// The browser already compresses before upload (canvas -> WebP at 1600px), but
// nothing stops a client from posting a 15MB original straight at the API, so
// the same limits are enforced again here.
package photo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
	// Registers the WebP decoder. Browsers upload WebP by preference, so this
	// is the common path, not an exotic one. (Encoding stays JPEG — there is
	// no WebP encoder in x/image.)
	_ "golang.org/x/image/webp"
)

// ErrUnsupportedType is returned when the bytes are not an image we accept.
var ErrUnsupportedType = errors.New("photo: unsupported image type")

// Kinds of photo the app stores.
const (
	KindProgress    = "progress"
	KindFood        = "food"
	KindIngredients = "ingredients"
)

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
	root      string
	maxEdge   int
	thumbEdge int
	quality   int
}

// NewStore creates a Store rooted at dir. maxEdge and thumbEdge are the
// longest-side limits for the stored image and its thumbnail.
func NewStore(dir string, maxEdge, thumbEdge int) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("photo: create root: %w", err)
	}
	if maxEdge <= 0 {
		maxEdge = 1600
	}
	if thumbEdge <= 0 {
		thumbEdge = 320
	}
	return &Store{root: dir, maxEdge: maxEdge, thumbEdge: thumbEdge, quality: 82}, nil
}

// Root returns the directory the store writes into.
func (s *Store) Root() string { return s.root }

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
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("photo: read: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("photo: image exceeds %d bytes", maxBytes)
	}
	if !looksLikeImage(raw) {
		return nil, ErrUnsupportedType
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("photo: decode: %w", ErrUnsupportedType)
	}

	full := fit(img, s.maxEdge)
	thumb := fit(img, s.thumbEdge)

	fullBytes, err := encodeJPEG(full, s.quality)
	if err != nil {
		return nil, err
	}
	thumbBytes, err := encodeJPEG(thumb, 75)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(fullBytes)
	hash := hex.EncodeToString(sum[:])

	now := time.Now().UTC()
	dir := filepath.Join(fmt.Sprintf("%d", userID), now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(filepath.Join(s.root, dir), 0o755); err != nil {
		return nil, fmt.Errorf("photo: create dir: %w", err)
	}

	base := fmt.Sprintf("%s-%d-%s", kind, now.UnixNano(), hash[:8])
	relFull := filepath.Join(dir, base+".jpg")
	relThumb := filepath.Join(dir, base+"-thumb.jpg")

	if err := writeFile(filepath.Join(s.root, relFull), fullBytes); err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(s.root, relThumb), thumbBytes); err != nil {
		// Don't leave the full image orphaned if the thumbnail fails.
		_ = os.Remove(filepath.Join(s.root, relFull))
		return nil, err
	}

	b := full.Bounds()
	return &Saved{
		RelPath:   filepath.ToSlash(relFull),
		ThumbPath: filepath.ToSlash(relThumb),
		Mime:      "image/jpeg",
		Width:     b.Dx(),
		Height:    b.Dy(),
		Bytes:     int64(len(fullBytes)),
		SHA256:    hash,
	}, nil
}

// Open returns a reader for a stored path, rejecting anything that tries to
// escape the root.
func (s *Store) Open(rel string) (*os.File, error) {
	clean := filepath.Clean(filepath.FromSlash("/" + rel))
	full := filepath.Join(s.root, clean)
	if !strings.HasPrefix(full, filepath.Clean(s.root)+string(os.PathSeparator)) {
		return nil, errors.New("photo: path outside store")
	}
	return os.Open(full)
}

// Remove deletes a stored image and its thumbnail, ignoring missing files.
func (s *Store) Remove(paths ...string) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash("/" + p))
		full := filepath.Join(s.root, clean)
		if strings.HasPrefix(full, filepath.Clean(s.root)+string(os.PathSeparator)) {
			_ = os.Remove(full)
		}
	}
}

// fit scales img down so its longest side is at most edge, preserving aspect
// ratio. Images already within the limit are returned untouched — upscaling a
// small photo only wastes bytes.
func fit(img image.Image, edge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= edge && h <= edge {
		return img
	}

	var nw, nh int
	if w >= h {
		nw = edge
		nh = int(float64(h) * float64(edge) / float64(w))
	} else {
		nh = edge
		nw = int(float64(w) * float64(edge) / float64(h))
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	// CatmullRom is slower than bilinear but visibly sharper on the downscale
	// factors phone photos hit, and this runs once per upload.
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("photo: encode: %w", err)
	}
	return buf.Bytes(), nil
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("photo: write %s: %w", path, err)
	}
	return nil
}

// looksLikeImage sniffs the magic bytes. The declared Content-Type on a
// multipart part is client-controlled, so it is not evidence of anything.
func looksLikeImage(b []byte) bool {
	switch {
	case len(b) < 12:
		return false
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}): // JPEG
		return true
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")): // PNG
		return true
	case bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return true
	case bytes.HasPrefix(b, []byte("GIF8")):
		return true
	}
	return false
}

// Ensure the PNG decoder is linked in; image.Decode needs the format
// registered to handle screenshots pasted from a desktop.
var _ = png.Decode
