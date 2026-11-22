package reader

import (
	"fmt"
	"regexp"
	"strings"

	"epubaudio/internal/model"
	"github.com/ledongthuc/pdf"
)

var (
	// Regex for un-hyphenating words broken across line breaks (e.g. "expe-\ndition" -> "expedition")
	hyphenBreakRegex = regexp.MustCompile(`(?m)([\p{L}]+)-\n\s*([\p{L}]+)`)

	// Regex for standalone page numbers (e.g., "12" or " 45 ")
	standalonePageNumRegex = regexp.MustCompile(`(?m)^\s*\d+\s*$`)

	// Common ligatures replacement map for cleaner TTS synthesis
	ligatureReplacer = strings.NewReplacer(
		"\u00ad", "", // soft hyphen
		"\u00a0", " ", // non-breaking space
		"ﬁ", "fi",
		"ﬂ", "fl",
		"ﬀ", "ff",
		"ﬃ", "ffi",
		"ﬄ", "ffl",
		"‘", "'",
		"’", "'",
		"“", "\"",
		"”", "\"",
	)
)

// OpenPDF reads and parses a PDF document into a model.Book with detected chapters.
func OpenPDF(path string) (*model.Book, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	numPages := r.NumPage()
	if numPages == 0 {
		return nil, fmt.Errorf("PDF document has 0 pages")
	}

	// 1. Extract metadata from PDF Trailer
	title := CleanTitle(path)
	author := ""
	trailer := r.Trailer()
	info := trailer.Key("Info")
	if !info.IsNull() {
		if t := strings.TrimSpace(info.Key("Title").Text()); t != "" {
			title = t
		}
		if a := strings.TrimSpace(info.Key("Author").Text()); a != "" {
			author = a
		}
	}

	// 2. Extract and clean text from each page
	rawPages := make([]string, 0, numPages)
	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		pageText, err := p.GetPlainText(nil)
		if err != nil {
			// Some pages might fail to decode; continue with empty
			rawPages = append(rawPages, "")
			continue
		}
		rawPages = append(rawPages, pageText)
	}

	cleanedPages := cleanPDFPages(rawPages, title)

	// Combine all text for language detection and check if empty
	var sb strings.Builder
	for _, cp := range cleanedPages {
		if cp != "" {
			sb.WriteString(cp)
			sb.WriteString("\n\n")
		}
	}
	fullText := strings.TrimSpace(sb.String())
	if fullText == "" {
		return nil, fmt.Errorf("no extractable text found in PDF (the document may be scanned or image-based; OCR is not supported)")
	}

	lang := DetectLanguage(fullText)

	// 3. Segment into chapters
	chapters := parsePDFChapters(cleanedPages, fullText, title)
	if len(chapters) == 0 {
		return nil, fmt.Errorf("failed to extract chapters from PDF")
	}

	return &model.Book{
		FilePath: path,
		Title:    title,
		Author:   author,
		Language: lang,
		Chapters: chapters,
	}, nil
}

// cleanPDFPages applies text normalization, removes soft-hyphens/ligatures,
// removes standalone page numbers, and detects/strips repeated running headers/footers.
func cleanPDFPages(pages []string, bookTitle string) []string {
	// First pass: normalize ligatures, hyphenation, and trim
	cleaned := make([]string, len(pages))
	for i, page := range pages {
		cleaned[i] = normalizePDFText(page)
	}

	// Second pass: detect and strip recurring running headers/footers
	// (lines at the top or bottom of pages that repeat across >35% of pages)
	headerFreq := make(map[string]int)
	footerFreq := make(map[string]int)
	totalNonEmptyPages := 0

	for _, p := range cleaned {
		lines := splitLines(p)
		var nonBlank []string
		for _, l := range lines {
			t := strings.TrimSpace(l)
			if t != "" {
				nonBlank = append(nonBlank, t)
			}
		}
		if len(nonBlank) == 0 {
			continue
		}
		totalNonEmptyPages++
		first := nonBlank[0]
		last := nonBlank[len(nonBlank)-1]
		if len(first) < 100 {
			headerFreq[first]++
		}
		if len(last) < 100 {
			footerFreq[last]++
		}
	}

	// Threshold: if repeats in at least 3 pages and >= 35% of non-empty pages
	threshold := 3
	if pct := (totalNonEmptyPages * 35) / 100; pct > threshold {
		threshold = pct
	}

	recurringHeaders := make(map[string]bool)
	for h, count := range headerFreq {
		if count >= threshold || strings.EqualFold(h, bookTitle) {
			recurringHeaders[h] = true
		}
	}

	recurringFooters := make(map[string]bool)
	for f, count := range footerFreq {
		if count >= threshold {
			recurringFooters[f] = true
		}
	}

	// Third pass: remove recurring headers/footers
	for i, p := range cleaned {
		lines := splitLines(p)
		if len(lines) == 0 {
			continue
		}
		var filtered []string
		for idx, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				filtered = append(filtered, line)
				continue
			}
			// Check if header line
			if idx == 0 || idx == 1 {
				if recurringHeaders[trimmed] {
					continue
				}
			}
			// Check if footer line
			if idx >= len(lines)-2 {
				if recurringFooters[trimmed] {
					continue
				}
			}
			filtered = append(filtered, line)
		}
		cleaned[i] = strings.TrimSpace(strings.Join(filtered, "\n"))
	}

	return cleaned
}

// normalizePDFText replaces typographic ligatures, unhyphenates split words,
// strips standalone page number lines, and cleans excess whitespace.
func normalizePDFText(text string) string {
	if text == "" {
		return ""
	}

	// 1. Replace ligatures and typographic quotes
	text = ligatureReplacer.Replace(text)

	// 2. Un-hyphenate words broken across line breaks: "ex-\nample" -> "example"
	text = hyphenBreakRegex.ReplaceAllString(text, "$1$2")

	// 3. Strip standalone page numbers: lines consisting solely of digits
	text = standalonePageNumRegex.ReplaceAllString(text, "")

	// 4. Normalize multiple consecutive blank lines to double newlines
	lines := splitLines(text)
	var result []string
	consecutiveEmpty := 0
	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			consecutiveEmpty++
			if consecutiveEmpty <= 2 {
				result = append(result, "")
			}
		} else {
			consecutiveEmpty = 0
			result = append(result, trimmed)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// parsePDFChapters attempts to segment pages using detected chapter headings.
// If headings are found, it gathers pages/sections into chapters.
// Otherwise, it falls back to paragraph-based chapter partitioning (~2500 words per chapter).
func parsePDFChapters(pages []string, fullText string, defaultTitle string) []model.Chapter {
	type headingOccurrence struct {
		pageIdx   int
		lineIdx   int
		title     string
		fullMatch string
	}

	var headings []headingOccurrence
	for pIdx, page := range pages {
		lines := splitLines(page)
		for lIdx, line := range lines {
			trimmed := strings.TrimSpace(line)
			if title, isHeading := matchChapterHeading(trimmed); isHeading {
				headings = append(headings, headingOccurrence{
					pageIdx:   pIdx,
					lineIdx:   lIdx,
					title:     title,
					fullMatch: trimmed,
				})
			}
		}
	}

	// If at least 2 chapter headings were found, or 1 heading not at the very end
	if len(headings) >= 2 || (len(headings) == 1 && headings[0].pageIdx < len(pages)-1) {
		var drafts []chapterDraft
		var currentDraft chapterDraft

		for _, page := range pages {
			lines := splitLines(page)
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if title, isHeading := matchChapterHeading(trimmed); isHeading {
					if len(currentDraft.lines) > 0 || currentDraft.title != "" {
						drafts = append(drafts, currentDraft)
					}
					currentDraft = chapterDraft{
						title: title,
						lines: []string{},
					}
					continue
				}
				currentDraft.lines = append(currentDraft.lines, line)
			}
		}
		if len(currentDraft.lines) > 0 || currentDraft.title != "" {
			drafts = append(drafts, currentDraft)
		}

		var validDrafts []chapterDraft
		for _, d := range drafts {
			text := strings.TrimSpace(strings.Join(d.lines, "\n"))
			if text != "" {
				if d.title == "" {
					d.title = defaultTitle
				}
				validDrafts = append(validDrafts, d)
			}
		}

		if len(validDrafts) > 0 {
			return buildChaptersFromDrafts(validDrafts)
		}
	}

	// Fallback: partition by paragraphs (~2500 words/chapter)
	return partitionTextIntoChapters(fullText, defaultTitle)
}
