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
