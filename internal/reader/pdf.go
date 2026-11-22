package reader

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

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
	cleanFileTitle := CleanTitle(path)
	title := cleanFileTitle
	author := ""
	trailer := r.Trailer()
	info := trailer.Key("Info")
	if !info.IsNull() {
		if t := strings.TrimSpace(info.Key("Title").Text()); t != "" {
			title = t
		}
		if a := strings.TrimSpace(info.Key("Author").Text()); a != "" {
			author = a
			if idx := strings.Index(author, "|"); idx != -1 {
				author = strings.TrimSpace(author[:idx])
			}
		}
	}

	// 2. Extract and inspect raw pages
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

	// Refine Title and Author from Page 1 raw lines
	// Often PDF trailer metadata is stale or concatenates title + author
	if len(rawPages) > 0 {
		var p1NonEmpty []string
		for _, l := range splitLines(rawPages[0]) {
			t := strings.TrimSpace(l)
			if t != "" {
				p1NonEmpty = append(p1NonEmpty, t)
			}
		}
		if len(p1NonEmpty) > 0 {
			first := p1NonEmpty[0]
			normFirst := normalizeAlpha(first)
			normClean := normalizeAlpha(cleanFileTitle)
			normTitle := normalizeAlpha(title)
			if strings.EqualFold(first, cleanFileTitle) ||
				strings.Contains(normFirst, normClean) ||
				strings.Contains(normClean, normFirst) ||
				(normTitle != "" && strings.HasPrefix(normTitle, normFirst)) ||
				strings.EqualFold(title, "untitled") ||
				strings.EqualFold(title, "el-pitido") {
				title = first
				if len(p1NonEmpty) > 1 && (author == "" || strings.EqualFold(author, "untitled")) {
					second := p1NonEmpty[1]
					if len(second) < 50 && !endsSentence(second) && isAllUpper(second) {
						author = second
					}
				}
			}
		}
	}

	cleanedPages := cleanPDFPages(rawPages, title)

	// Combine all text for language detection and check if empty
	var sb strings.Builder
	for _, cp := range cleanedPages {
		if cp == "" {
			continue
		}
		if sb.Len() > 0 {
			prev := strings.TrimSpace(sb.String())
			firstRune := ' '
			for _, r := range cp {
				if !unicode.IsSpace(r) {
					firstRune = r
					break
				}
			}
			// If previous text ended with a sentence terminator, OR next page starts with uppercase / dialogue / heading:
			if endsSentence(prev) || unicode.IsUpper(firstRune) || firstRune == '—' || firstRune == '–' || firstRune == '"' || firstRune == '“' || firstRune == '«' {
				sb.WriteString("\n\n")
			} else {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(cp)
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

		// Check if page 0 is a dedicated cover/title page (has <= 2 non-blank lines)
		nonBlankCount := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				nonBlankCount++
			}
		}
		isCoverPage := (i == 0 && nonBlankCount <= 2)

		var filtered []string
		for idx, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				filtered = append(filtered, line)
				continue
			}
			// Check if header line (do not strip title from a dedicated cover page)
			if idx == 0 || idx == 1 {
				if recurringHeaders[trimmed] && !isCoverPage {
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
		rawPageText := strings.TrimSpace(strings.Join(filtered, "\n"))
		cleaned[i] = reflowPDFText(rawPageText, bookTitle)
	}

	return cleaned
}

func endsSentence(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 {
		return false
	}
	return strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "!") ||
		strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, ":") ||
		strings.HasSuffix(trimmed, "\"") || strings.HasSuffix(trimmed, "”") ||
		strings.HasSuffix(trimmed, "»") || strings.HasSuffix(trimmed, "...")
}

// reflowPDFText un-wraps visual line wraps within paragraphs into continuous sentences
// so TTS engines do not insert awkward pauses after every single line return.
func reflowPDFText(text string, bookTitle string) string {
	rawLines := splitLines(text)
	if len(rawLines) == 0 {
		return ""
	}

	// Calculate average line length for non-empty lines
	totalLen := 0
	count := 0
	for _, l := range rawLines {
		t := strings.TrimSpace(l)
		if len(t) > 0 {
			totalLen += len(t)
			count++
		}
	}
	avgLen := 70
	if count > 0 {
		avgLen = totalLen / count
	}
	shortThreshold := (avgLen * 65) / 100
	if shortThreshold < 40 {
		shortThreshold = 40
	}

	var paragraphs []string
	var currentPara strings.Builder

	flushPara := func() {
		if currentPara.Len() > 0 {
			paragraphs = append(paragraphs, strings.TrimSpace(currentPara.String()))
			currentPara.Reset()
		}
	}

	isDialogueOrBullet := func(s string) bool {
		trimmed := strings.TrimSpace(s)
		return strings.HasPrefix(trimmed, "—") || strings.HasPrefix(trimmed, "–") ||
			strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "• ") ||
			strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "\"") ||
			strings.HasPrefix(trimmed, "“") || strings.HasPrefix(trimmed, "«")
	}

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flushPara()
			continue
		}

		// Standalone chapter heading, book title, author, or subtitle line
		if isStandaloneHeading(trimmed, bookTitle, avgLen) {
			flushPara()
			heading := trimmed
			if !endsSentence(heading) && !strings.HasSuffix(heading, ":") && !strings.HasSuffix(heading, "—") {
				heading += "."
			}
			paragraphs = append(paragraphs, heading)
			continue
		}

		// Dialogue dash starts a new paragraph
		if isDialogueOrBullet(trimmed) {
			flushPara()
			currentPara.WriteString(trimmed)
			if len(trimmed) <= shortThreshold && endsSentence(trimmed) {
				flushPara()
			}
			continue
		}

		if currentPara.Len() == 0 {
			currentPara.WriteString(trimmed)
		} else {
			prevText := strings.TrimSpace(currentPara.String())
			// If previous line was dialogue and ended with punctuation, break paragraph
			if isDialogueOrBullet(prevText) && endsSentence(prevText) {
				flushPara()
				currentPara.WriteString(trimmed)
			} else {
				currentPara.WriteString(" ")
				currentPara.WriteString(trimmed)
			}
		}

		// If current line is noticeably short and ends with a sentence terminator, it marks end of paragraph
		if len(trimmed) <= shortThreshold && endsSentence(trimmed) {
			flushPara()
		}
	}
	flushPara()

	return strings.Join(paragraphs, "\n\n")
}

// isStandaloneHeading returns true if line is a chapter heading, title, author, or subtitle
// that should not be merged with subsequent prose.
func isStandaloneHeading(line string, bookTitle string, avgLen int) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// 1. Chapter heading regex (e.g. "Capítulo 1", "Chapter 2", "Prologue", "III")
	if _, isHeading := matchChapterHeading(trimmed); isHeading {
		return true
	}

	// 2. Exact or normalized match with book title
	if bookTitle != "" {
		if strings.EqualFold(trimmed, bookTitle) || strings.EqualFold(normalizeAlpha(trimmed), normalizeAlpha(bookTitle)) {
			return true
		}
	}

	// If line ends with standard sentence punctuation, it is regular prose
	if endsSentence(trimmed) {
		return false
	}

	// 3. Short line (< 60 chars) that is ALL UPPERCASE letters (e.g. "GAIZKA DC", "KUKLOS", "EPÍLOGO")
	if len(trimmed) < 60 && isAllUpper(trimmed) {
		return true
	}

	// 4. Short line (< 50 chars) starting with capital letter with at most 7 words,
	// when significantly shorter than the average body line length (< 65% of avgLen).
	words := strings.Fields(trimmed)
	if len(trimmed) <= 50 && len(words) >= 1 && len(words) <= 7 {
		firstRune := []rune(trimmed)[0]
		if unicode.IsUpper(firstRune) {
			if isTitleCaseOrHeading(trimmed) || len(trimmed) <= (avgLen*65)/100 {
				return true
			}
		}
	}

	return false
}

func normalizeAlpha(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'á', 'à', 'ä', 'â':
			sb.WriteRune('a')
		case 'é', 'è', 'ë', 'ê':
			sb.WriteRune('e')
		case 'í', 'ì', 'ï', 'î':
			sb.WriteRune('i')
		case 'ó', 'ò', 'ö', 'ô':
			sb.WriteRune('o')
		case 'ú', 'ù', 'ü', 'û':
			sb.WriteRune('u')
		case 'ñ':
			sb.WriteRune('n')
		default:
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				sb.WriteRune(r)
			}
		}
	}
	return sb.String()
}

func isAllUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}

func isTitleCaseOrHeading(s string) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}
	firstRune := []rune(words[0])[0]
	if !unicode.IsUpper(firstRune) {
		return false
	}
	// Common lowercase articles, conjunctions, and prepositions in ES/EN/FR/DE
	allowedLower := map[string]bool{
		"de": true, "del": true, "la": true, "el": true, "los": true, "las": true,
		"en": true, "y": true, "o": true, "a": true, "un": true, "una": true, "por": true, "con": true,
		"the": true, "of": true, "and": true, "in": true, "to": true, "for": true, "with": true, "on": true, "at": true,
		"le": true, "les": true, "des": true, "du": true, "et": true, "pour": true, "dans": true,
		"der": true, "die": true, "das": true, "und": true, "von": true, "zu": true, "mit": true,
	}
	letterWords := 0
	upperOrAllowed := 0
	for _, w := range words {
		runes := []rune(w)
		if len(runes) == 0 || !unicode.IsLetter(runes[0]) {
			continue
		}
		letterWords++
		if unicode.IsUpper(runes[0]) || allowedLower[strings.ToLower(w)] {
			upperOrAllowed++
		}
	}
	return letterWords > 0 && upperOrAllowed == letterWords
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
