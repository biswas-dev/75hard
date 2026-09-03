// Package journal reads text out of an uploaded journal page.
//
// Two kinds of PDF arrive here and they need different treatment. One was
// typed and carries its text inside the file, where it can be extracted
// exactly and for free. The other is a photograph or scan of handwriting and
// contains no text at all — only a picture of some — and the only way to read
// it is to show it to a model that can see.
//
// Extraction is tried first because when it works it is exact, instant, and
// costs nothing. Only when it comes back empty is the expensive path used.
package journal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// MinExtractedChars is the point below which a PDF is treated as having no
// real text.
//
// A scan is not always completely empty — a header, a page number, or a stray
// artefact can produce a few characters — so "did extraction work" cannot be
// "was anything returned". A page of writing that yields under this is a
// picture of writing, not writing.
const MinExtractedChars = 40

// ErrEncrypted is returned for a PDF that will not open without a password.
var ErrEncrypted = errors.New("journal: the PDF is password protected")

// Text is what could be read from a document.
type Text struct {
	// Content is the text itself.
	Content string
	// Pages is the page count, when known.
	Pages int
	// NeedsVision is true when the file carried no usable text and has to be
	// looked at rather than parsed.
	NeedsVision bool
}

// ExtractText pulls embedded text out of a PDF.
//
// A failure to parse is not treated as an error: plenty of real PDFs are
// malformed in ways a reader rejects, and the answer for all of them is the
// same — fall through to reading the page as an image.
func ExtractText(data []byte) (Text, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "encrypt") {
			return Text{}, ErrEncrypted
		}
		return Text{NeedsVision: true}, nil
	}

	pages := reader.NumPage()

	// The library panics on some malformed files rather than returning an
	// error, and a malformed upload must not take the server down with it.
	var content string
	func() {
		defer func() {
			if recover() != nil {
				content = ""
			}
		}()
		body, perr := reader.GetPlainText()
		if perr != nil {
			return
		}
		var buf bytes.Buffer
		if _, cerr := io.Copy(&buf, body); cerr != nil {
			return
		}
		content = buf.String()
	}()

	content = strings.TrimSpace(collapseSpace(content))
	if len([]rune(content)) < MinExtractedChars {
		return Text{Pages: pages, NeedsVision: true}, nil
	}
	return Text{Content: content, Pages: pages}, nil
}

// collapseSpace tidies extracted text.
//
// PDF extraction produces ragged whitespace because the format stores glyph
// positions rather than lines. Collapsing it keeps the stored text readable
// and stops the search index filling with blank tokens.
func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace, lastNewline := false, false
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r':
			if !lastNewline {
				b.WriteByte('\n')
			}
			lastNewline, lastSpace = true, true
		case r == ' ' || r == '\t':
			if !lastSpace {
				b.WriteByte(' ')
			}
			lastSpace = true
		default:
			b.WriteRune(r)
			lastSpace, lastNewline = false, false
		}
	}
	return b.String()
}

// RasterCommand is the external tool used to turn PDF pages into images.
// Overridable so tests do not need it installed.
var RasterCommand = "pdftoppm"

// ErrNoRasteriser means the tool needed to read handwriting is unavailable.
var ErrNoRasteriser = errors.New("journal: no PDF rasteriser available")

// RasterAvailable reports whether handwriting can be read on this host.
func RasterAvailable() bool {
	_, err := exec.LookPath(RasterCommand)
	return err == nil
}

// Rasterise renders the first maxPages pages of a PDF to PNG images.
//
// Only the first few pages are rendered: each one becomes a separate vision
// call, and a journal entry that runs to forty pages should cost a few
// requests rather than forty.
func Rasterise(data []byte, maxPages int) ([][]byte, error) {
	if !RasterAvailable() {
		return nil, ErrNoRasteriser
	}
	if maxPages <= 0 {
		maxPages = 4
	}

	dir, err := os.MkdirTemp("", "journal-*")
	if err != nil {
		return nil, fmt.Errorf("journal: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		return nil, fmt.Errorf("journal: writing pdf: %w", err)
	}

	// 150 DPI is the point where handwriting stays legible to a model without
	// producing images too large to send.
	cmd := exec.Command(RasterCommand,
		"-png", "-r", "150", "-f", "1", "-l", fmt.Sprint(maxPages),
		src, filepath.Join(dir, "page"))

	// A malformed PDF can make the renderer hang; it must not hold a worker.
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("journal: starting %s: %w", RasterCommand, err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("journal: %s failed: %w", RasterCommand, err)
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("journal: %s timed out", RasterCommand)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "page*.png"))
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("journal: no pages were rendered")
	}

	out := make([][]byte, 0, len(matches))
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("journal: rendered pages could not be read")
	}
	return out, nil
}

// LooksLikePDF reports whether the bytes begin with the PDF marker.
//
// The declared content type on an upload is client-controlled and therefore
// not evidence; the magic number is.
func LooksLikePDF(data []byte) bool {
	return len(data) > 4 && bytes.HasPrefix(data, []byte("%PDF-"))
}
