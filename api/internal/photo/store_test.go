package photo

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testJPEG builds a real JPEG of the given size so the tests exercise the
// actual decode path rather than a stub.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir(), 1600, 320)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestSaveDownscalesAndThumbnails(t *testing.T) {
	s := newTestStore(t)
	src := testJPEG(t, 4000, 3000) // a phone-camera-sized shot

	got, err := s.Save(bytes.NewReader(src), 7, KindProgress, 15<<20)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got.Width != 1600 {
		t.Errorf("stored width = %d, want 1600 (longest edge capped)", got.Width)
	}
	if got.Height != 1200 {
		t.Errorf("stored height = %d, want 1200 (aspect ratio preserved)", got.Height)
	}
	if got.Bytes >= int64(len(src)) {
		t.Errorf("stored %d bytes from a %d byte original; upload should shrink", got.Bytes, len(src))
	}
	if got.Mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", got.Mime)
	}
	if len(got.SHA256) != 64 {
		t.Errorf("sha256 = %q, want 64 hex chars", got.SHA256)
	}

	// Both files must actually exist under the root.
	for _, rel := range []string{got.RelPath, got.ThumbPath} {
		if _, err := os.Stat(filepath.Join(s.Root(), filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected file at %s: %v", rel, err)
		}
	}

	// The thumbnail must genuinely be thumbnail-sized.
	f, err := s.Open(got.ThumbPath)
	if err != nil {
		t.Fatalf("Open thumb: %v", err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}
	if cfg.Width != 320 {
		t.Errorf("thumb width = %d, want 320", cfg.Width)
	}
}

func TestSaveDoesNotUpscaleSmallImages(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Save(bytes.NewReader(testJPEG(t, 200, 150)), 1, KindFood, 15<<20)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.Width != 200 || got.Height != 150 {
		t.Errorf("got %dx%d, want 200x150 — small images must pass through unscaled", got.Width, got.Height)
	}
}

func TestSaveRejectsNonImages(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Save(strings.NewReader("#!/bin/sh\nrm -rf /\n"), 1, KindFood, 15<<20)
	if err == nil {
		t.Fatal("expected an error for a non-image upload")
	}
}

func TestSaveEnforcesSizeLimit(t *testing.T) {
	s := newTestStore(t)
	src := testJPEG(t, 2000, 2000)
	if _, err := s.Save(bytes.NewReader(src), 1, KindProgress, 1024); err == nil {
		t.Fatal("expected an error when the upload exceeds maxBytes")
	}
}

func TestOpenRejectsTraversal(t *testing.T) {
	s := newTestStore(t)
	// Write a file next to the root that a traversal would reach.
	outside := filepath.Join(filepath.Dir(s.Root()), "secret.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"../secret.txt", "../../etc/passwd", "/etc/passwd"} {
		if f, err := s.Open(p); err == nil {
			f.Close()
			t.Errorf("Open(%q) succeeded; traversal must be rejected", p)
		}
	}
}

func TestValidKind(t *testing.T) {
	for _, k := range []string{KindProgress, KindFood, KindIngredients} {
		if !ValidKind(k) {
			t.Errorf("ValidKind(%q) = false, want true", k)
		}
	}
	if ValidKind("../../etc") {
		t.Error("ValidKind should reject unknown kinds")
	}
}

func TestRemove(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Save(bytes.NewReader(testJPEG(t, 100, 100)), 1, KindProgress, 15<<20)
	if err != nil {
		t.Fatal(err)
	}
	s.Remove(got.RelPath, got.ThumbPath)
	if _, err := os.Stat(filepath.Join(s.Root(), filepath.FromSlash(got.RelPath))); !os.IsNotExist(err) {
		t.Error("Remove should have deleted the stored image")
	}
}
