package reader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my_great_document.txt", "my great document"},
		{"/path/to/some_fancy_book.pdf", "some fancy book"},
		{"simple.epub", "simple"},
		{"Document_With_Underscores.txt", "Document With Underscores"},
	}

	for _, tt := range tests {
		got := CleanTitle(tt.input)
		if got != tt.expected {
			t.Errorf("CleanTitle(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsSupported(t *testing.T) {
	valid := []string{"book.epub", "notes.txt", "manual.pdf", "FILE.EPUB", "Doc.PDF", "NOTES.TXT"}
	for _, f := range valid {
		if !IsSupported(f) {
			t.Errorf("expected IsSupported(%q) = true, got false", f)
		}
	}

	invalid := []string{"audio.mp3", "doc.docx", "data.json", "image.png", "noext"}
	for _, f := range invalid {
		if IsSupported(f) {
			t.Errorf("expected IsSupported(%q) = false, got true", f)
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	spanishText := "En un lugar de la llanura de cuyo nombre no quiero acordarme vivía un caballero con su caballo."
	if lang := DetectLanguage(spanishText); lang != "es" {
		t.Errorf("DetectLanguage(spanish) = %q, want 'es'", lang)
	}

	englishText := "The quick brown fox jumps over the lazy dog and runs to the forest with a happy leap."
	if lang := DetectLanguage(englishText); lang != "en" {
		t.Errorf("DetectLanguage(english) = %q, want 'en'", lang)
	}

	frenchText := "Le petit prince habitait sur une planete a peine plus grande que lui et avait besoin d'un ami."
	if lang := DetectLanguage(frenchText); lang != "fr" {
		t.Errorf("DetectLanguage(french) = %q, want 'fr'", lang)
	}
}

func TestOpenUnsupported(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.xyz")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Open(tmpFile)
	if err == nil || !strings.Contains(err.Error(), "unsupported document format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}
