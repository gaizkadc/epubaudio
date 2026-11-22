package reader

import (
	"strings"
	"testing"
)

func TestNormalizePDFText(t *testing.T) {
	input := "The ﬁrst step is to un-\nderstand the ﬂow of infor-\nmation.\n42\nAll rights reserved."
	cleaned := normalizePDFText(input)

	// Check ligature replacement
	if strings.Contains(cleaned, "ﬁ") {
		t.Errorf("expected ligature 'fi' to be replaced, got: %s", cleaned)
	}
	if !strings.Contains(cleaned, "first") {
		t.Errorf("expected 'first', got: %s", cleaned)
	}

	// Check un-hyphenation across line breaks
	if !strings.Contains(cleaned, "understand") {
		t.Errorf("expected 'understand', got: %s", cleaned)
	}
	if !strings.Contains(cleaned, "information") {
		t.Errorf("expected 'information', got: %s", cleaned)
	}

	// Check standalone page number removed
	lines := splitLines(cleaned)
	for _, l := range lines {
		if strings.TrimSpace(l) == "42" {
			t.Errorf("expected standalone page number 42 to be stripped, got line: %s", l)
		}
	}
}

func TestReflowPDFText(t *testing.T) {
	input := `The hallway was dark, but the light from the
bathroom cut the floor in two. The traveler paused at the
edge of the stones.
—Do not stop —whispered the guide from the shadows—. The road ahead is clear.
We packed our bags and continued on our journey toward
the mountains where the sun was beginning to rise behind
the misty peaks.
`
	reflowed := reflowPDFText(input, "Sample Book")
	paras := strings.Split(reflowed, "\n\n")

	if len(paras) != 3 {
		t.Fatalf("expected 3 paragraphs, got %d: %v", len(paras), paras)
	}

	// Paragraph 1 should be joined into a continuous sentence
	if !strings.Contains(paras[0], "bathroom cut the floor in two. The traveler paused at the edge of the stones.") {
		t.Errorf("expected paragraph 1 to be joined into a continuous sentence, got: %s", paras[0])
	}

	// Paragraph 2 should be the dialogue line
	if !strings.HasPrefix(paras[1], "—Do not stop") {
		t.Errorf("expected paragraph 2 to be dialogue, got: %s", paras[1])
	}

	// Paragraph 3 should be joined
	if !strings.Contains(paras[2], "journey toward the mountains where the sun was beginning to rise behind the misty peaks.") {
		t.Errorf("expected paragraph 3 to be joined, got: %s", paras[2])
	}
}

func TestCleanPDFPages_RecurringHeaders(t *testing.T) {
	pages := []string{
		"Global Annual Report\nChapter 1\nThe financial year was positive.\nPage 1",
		"Global Annual Report\nFurther details on operations and development.\nPage 2",
		"Global Annual Report\nSummary of quarterly achievements and milestones.\nPage 3",
		"Global Annual Report\nConclusion and forward looking outlook for the team.\nPage 4",
	}

	cleaned := cleanPDFPages(pages, "Global Annual Report")

	for i, page := range cleaned {
		if strings.Contains(page, "Global Annual Report") {
			t.Errorf("page %d still contains recurring header: %s", i, page)
		}
	}
}

func TestKuklosPDF(t *testing.T) {
	book, err := OpenPDF("../../input/kuklos.pdf")
	if err != nil {
		t.Skipf("kuklos.pdf not found or error: %v", err)
	}
	if book.Title != "Kuklos" {
		t.Errorf("expected Title 'Kuklos', got %q", book.Title)
	}
	if len(book.Chapters) == 0 {
		t.Fatalf("expected at least 1 chapter")
	}
	txt := book.Chapters[0].Text
	if !strings.HasPrefix(txt, "Kuklos.\n\nGAIZKA DC.\n\n") {
		limit := len(txt)
		if limit > 100 {
			limit = 100
		}
		t.Errorf("expected chapter text to start with 'Kuklos.\\n\\nGAIZKA DC.\\n\\n', got %q", txt[:limit])
	}

	book2, err2 := OpenPDF("../../input/fallo-contencion.pdf")
	if err2 == nil {
		if book2.Title != "Fallo de contención" {
			t.Errorf("expected Title 'Fallo de contención', got %q", book2.Title)
		}
	}
}

