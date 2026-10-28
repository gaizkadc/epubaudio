package chunker

import (
	"strings"
	"testing"
)

func TestChunkerShort(t *testing.T) {
	c := New(100)
	text := "This is a short test."
	chunks := c.Split(text)
	if len(chunks) != 1 || chunks[0] != text {
		t.Fatalf("expected 1 chunk identical to input, got %v", chunks)
	}
}

func TestChunkerParagraphs(t *testing.T) {
	c := New(50)
	p1 := "Paragraph one with some nice words."
	p2 := "Paragraph two with additional words."
	text := p1 + "\n\n" + p2

	chunks := c.Split(text)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != p1 || chunks[1] != p2 {
		t.Fatalf("chunks content mismatch: %v", chunks)
	}
}

func TestChunkerLongSentence(t *testing.T) {
	c := New(40)
	text := "First sentence is here. Second sentence is also here. Third sentence is final."
	chunks := c.Split(text)
	for i, ch := range chunks {
		if len(ch) > 40 {
			t.Errorf("chunk %d exceeded limit: %d > 40 (%q)", i, len(ch), ch)
		}
	}
	joined := strings.Join(chunks, " ")
	if !strings.Contains(joined, "First sentence") || !strings.Contains(joined, "Third sentence") {
		t.Fatalf("lost content: %q", joined)
	}
}

