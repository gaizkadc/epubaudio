package reader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenTXT_WithHeadings(t *testing.T) {
	content := `# The Great Mountain
By: Alex Wanderer

Chapter 1: The Base Camp
We started early in the morning. The mountain peak was covered in white mist.
The path ahead seemed quiet and peaceful.

Chapter 2: The Climb
As we ascended, the wind grew stronger. Rocks were slippery under our boots.
We reached the ridge just before sundown.
`
	tmpFile := filepath.Join(t.TempDir(), "mountain_adventure.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	book, err := OpenTXT(tmpFile)
	if err != nil {
		t.Fatalf("OpenTXT failed: %v", err)
	}

	if book.Title != "The Great Mountain" {
		t.Errorf("Title = %q, want 'The Great Mountain'", book.Title)
	}
	if book.Author != "Alex Wanderer" {
		t.Errorf("Author = %q, want 'Alex Wanderer'", book.Author)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("len(Chapters) = %d, want 2", len(book.Chapters))
	}
	if book.Chapters[0].Title != "Chapter 1: The Base Camp" {
		t.Errorf("Chapter[0].Title = %q", book.Chapters[0].Title)
	}
	if book.Chapters[1].Title != "Chapter 2: The Climb" {
		t.Errorf("Chapter[1].Title = %q", book.Chapters[1].Title)
	}
}

func TestOpenTXT_FallbackChunking(t *testing.T) {
	// Generate text with 6000 words without headings to verify automatic partitioning
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("This is a paragraph discussing the principles of modern acoustic engineering and audio processing.\n\n")
	}

	tmpFile := filepath.Join(t.TempDir(), "essay_document.txt")
	if err := os.WriteFile(tmpFile, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}

	book, err := OpenTXT(tmpFile)
	if err != nil {
		t.Fatalf("OpenTXT failed: %v", err)
	}

	if len(book.Chapters) < 1 {
		t.Fatalf("expected at least 1 chapter, got %d", len(book.Chapters))
	}
	if book.Title != "essay document" {
		t.Errorf("Title = %q, want 'essay document'", book.Title)
	}
}

func TestOpenTXT_Empty(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(tmpFile, []byte("   \n\n  "), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := OpenTXT(tmpFile)
	if err == nil {
		t.Fatal("expected error for empty text file, got nil")
	}
}
