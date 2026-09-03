package journal

import (
	"strings"
	"testing"
)

// typedPDF builds a minimal one-page PDF carrying real text.
func typedPDF(t *testing.T, body string) []byte {
	t.Helper()
	stream := []byte("BT /F1 18 Tf 72 700 Td (" + body + ") Tj ET")
	objs := [][]byte{
		[]byte("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj"),
		[]byte("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj"),
		[]byte("3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 4 0 R>>>>/Contents 5 0 R>>endobj"),
		[]byte("4 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj"),
		append(append([]byte("5 0 obj<</Length "), []byte(itoa(len(stream)))...),
			append([]byte(">>stream\n"), append(stream, []byte("\nendstream endobj")...)...)...),
	}

	out := []byte("%PDF-1.4\n")
	var offs []int
	for _, o := range objs {
		offs = append(offs, len(out))
		out = append(out, o...)
		out = append(out, '\n')
	}
	xref := len(out)
	out = append(out, []byte("xref\n0 "+itoa(len(objs)+1)+"\n0000000000 65535 f \n")...)
	for _, off := range offs {
		out = append(out, []byte(pad10(off)+" 00000 n \n")...)
	}
	out = append(out, []byte("trailer<</Size "+itoa(len(objs)+1)+"/Root 1 0 R>>\nstartxref\n"+itoa(xref)+"\n%%EOF")...)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

func TestExtractTypedPDF(t *testing.T) {
	const body = "Day 12 the outdoor walk was cold but I went anyway and felt better for it"
	got, err := ExtractText(typedPDF(t, body))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if got.NeedsVision {
		t.Error("a typed PDF was sent down the vision path; extraction is exact and free")
	}
	if !strings.Contains(got.Content, "outdoor walk was cold") {
		t.Errorf("content = %q", got.Content)
	}
	if got.Pages != 1 {
		t.Errorf("pages = %d, want 1", got.Pages)
	}
}

func TestScannedPDFFallsThroughToVision(t *testing.T) {
	// A scan carries a picture of writing, not writing. A handful of stray
	// characters must not be mistaken for a successful extraction.
	got, err := ExtractText(typedPDF(t, "p. 4"))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !got.NeedsVision {
		t.Errorf("a near-empty PDF was treated as readable text: %q", got.Content)
	}
}

func TestMalformedInputIsNotFatal(t *testing.T) {
	// Uploads are hostile by default. None of these may panic or error out;
	// they should simply route to the vision path.
	for name, data := range map[string][]byte{
		"empty":       {},
		"plain text":  []byte("this is not a pdf at all"),
		"header only": []byte("%PDF-1.4\n"),
		"truncated":   typedPDF(t, "hello")[:60],
	} {
		got, err := ExtractText(data)
		if err != nil {
			t.Errorf("%s: returned an error: %v", name, err)
			continue
		}
		if !got.NeedsVision {
			t.Errorf("%s: was treated as readable text", name)
		}
	}
}

func TestLooksLikePDF(t *testing.T) {
	// The declared content type is client-controlled; the magic number is not.
	if !LooksLikePDF([]byte("%PDF-1.7\nrest")) {
		t.Error("a real PDF was rejected")
	}
	for _, bad := range [][]byte{
		[]byte("<?php system($_GET[0]); ?>"),
		[]byte("\x89PNG\r\n\x1a\n"),
		[]byte(""),
		[]byte("%PDF"),
	} {
		if LooksLikePDF(bad) {
			t.Errorf("%q was accepted as a PDF", bad)
		}
	}
}

func TestCollapseSpace(t *testing.T) {
	// Extraction produces ragged whitespace because the format stores glyph
	// positions, not lines. Left alone it fills the search index with blanks.
	got := collapseSpace("a  \t b\n\n\nc   \n  d")
	if strings.Contains(got, "  ") || strings.Contains(got, "\n\n") {
		t.Errorf("whitespace was not collapsed: %q", got)
	}
	if !strings.Contains(got, "a b") || !strings.Contains(got, "c") {
		t.Errorf("content was lost: %q", got)
	}
}

func TestRasteriseWithoutTheToolIsAClearError(t *testing.T) {
	// The host may not have a rasteriser. That has to be a named condition the
	// caller can report, not a mystery failure.
	old := RasterCommand
	RasterCommand = "definitely-not-a-real-binary-xyz"
	defer func() { RasterCommand = old }()

	if RasterAvailable() {
		t.Fatal("a nonexistent command reported available")
	}
	if _, err := Rasterise([]byte("%PDF-1.4"), 2); err != ErrNoRasteriser {
		t.Errorf("got %v, want ErrNoRasteriser", err)
	}
}
