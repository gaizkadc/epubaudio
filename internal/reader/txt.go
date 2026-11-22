package reader

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"epubaudio/internal/model"
)

var (
	// Chapter heading patterns (e.g., "Chapter 1", "Capítulo IV", "# The Journey", "Part 2")
	chapterHeadingRegex = regexp.MustCompile(`(?i)^(?:#{1,3}\s+(.+)|(?:chapter|cap[ií]tulo|part|parte|section|secci[oó]n|book|libro)\s+(\d+|[ivxlcdm]+)(?:\s*[:.\-—]\s*(.*))?)$`)
	romanNumeralRegex   = regexp.MustCompile(`^(?:[IVXLCDM]{1,8})\.?$`)
	byAuthorRegex       = regexp.MustCompile(`(?i)^(?:by|autor|por)\s*:\s*(.+)$`)
)

// OpenTXT reads and parses a plain text document into a model.Book with detected chapters.
func OpenTXT(path string) (*model.Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read txt file: %w", err)
	}

	// Remove UTF-8 Byte Order Mark (BOM) if present
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	content := string(data)
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("text file is empty")
	}

	title := CleanTitle(path)
	author := ""
	lang := DetectLanguage(content)

	lines := splitLines(content)

	// Check if top lines contain title or author hints
	lines, title, author = extractHeaderMetadata(lines, title)

	chapters := parseTXTChapters(lines, title)
	if len(chapters) == 0 {
		return nil, fmt.Errorf("failed to extract chapters from text file")
	}

	return &model.Book{
		FilePath: path,
		Title:    title,
		Author:   author,
		Language: lang,
		Chapters: chapters,
	}, nil
}

func extractHeaderMetadata(lines []string, defaultTitle string) ([]string, string, string) {
	title := defaultTitle
	author := ""

	startIdx := 0
	for i := 0; i < len(lines) && i < 15; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Markdown H1 title: # My Book Title
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			startIdx = i + 1
			continue
		}

		// Author line
		if m := byAuthorRegex.FindStringSubmatch(line); len(m) > 1 {
			author = strings.TrimSpace(m[1])
			startIdx = i + 1
			continue
		}
	}

	if startIdx > 0 && startIdx < len(lines) {
		return lines[startIdx:], title, author
	}
	return lines, title, author
}

type chapterDraft struct {
	title string
	lines []string
}

func parseTXTChapters(lines []string, defaultTitle string) []model.Chapter {
	var drafts []chapterDraft
	var currentDraft chapterDraft
	hasExplicitHeadings := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if headingTitle, isHeading := matchChapterHeading(trimmed); isHeading {
			hasExplicitHeadings = true
			if len(currentDraft.lines) > 0 || currentDraft.title != "" {
				drafts = append(drafts, currentDraft)
			}
			currentDraft = chapterDraft{
				title: headingTitle,
				lines: []string{},
			}
			continue
		}

		currentDraft.lines = append(currentDraft.lines, line)
	}

	if len(currentDraft.lines) > 0 || currentDraft.title != "" {
		drafts = append(drafts, currentDraft)
	}

	// Filter out empty preamble drafts
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

	// If explicit headings were found, build chapters from them
	if hasExplicitHeadings && len(validDrafts) > 0 {
		return buildChaptersFromDrafts(validDrafts)
	}

	// Fallback: No headings detected (e.g. raw essay or continuous prose)
	fullText := strings.TrimSpace(strings.Join(lines, "\n"))
	return partitionTextIntoChapters(fullText, defaultTitle)
}

func matchChapterHeading(line string) (string, bool) {
	if len(line) == 0 || len(line) > 100 {
		return "", false
	}

	if m := chapterHeadingRegex.FindStringSubmatch(line); len(m) > 0 {
		if m[1] != "" {
			return strings.TrimSpace(m[1]), true
		}
		num := m[2]
		rest := strings.TrimSpace(m[3])
		if rest != "" {
			return fmt.Sprintf("Chapter %s: %s", num, rest), true
		}
		return fmt.Sprintf("Chapter %s", num), true
	}

	// Check standalone Roman numerals (I, II, III, IV, etc.)
	if romanNumeralRegex.MatchString(line) {
		clean := strings.TrimSuffix(line, ".")
		return fmt.Sprintf("Chapter %s", clean), true
	}

	return "", false
}

func partitionTextIntoChapters(fullText string, defaultTitle string) []model.Chapter {
	words := strings.Fields(fullText)
	const wordsPerChapter = 2500

	// If short, return as a single chapter
	if len(words) <= wordsPerChapter {
		return []model.Chapter{
			{
				Index:     0,
				Title:     defaultTitle,
				Text:      fullText,
				CharCount: len(fullText),
				WordCount: len(words),
			},
		}
	}

	// Partition by paragraphs to avoid cutting mid-sentence
	paragraphs := strings.Split(fullText, "\n\n")
	var chapters []model.Chapter
	var currentParas []string
	currentWordCount := 0
	partIndex := 1

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pWords := len(strings.Fields(p))
		currentParas = append(currentParas, p)
		currentWordCount += pWords

		if currentWordCount >= wordsPerChapter {
			chText := strings.Join(currentParas, "\n\n")
			chapters = append(chapters, model.Chapter{
				Index:     len(chapters),
				Title:     fmt.Sprintf("Part %d", partIndex),
				Text:      chText,
				CharCount: len(chText),
				WordCount: currentWordCount,
			})
			partIndex++
			currentParas = nil
			currentWordCount = 0
		}
	}

	if len(currentParas) > 0 {
		chText := strings.Join(currentParas, "\n\n")
		chapters = append(chapters, model.Chapter{
			Index:     len(chapters),
			Title:     fmt.Sprintf("Part %d", partIndex),
			Text:      chText,
			CharCount: len(chText),
			WordCount: currentWordCount,
		})
	}

	return chapters
}

func buildChaptersFromDrafts(drafts []chapterDraft) []model.Chapter {
	chapters := make([]model.Chapter, 0, len(drafts))
	for i, d := range drafts {
		text := strings.TrimSpace(strings.Join(d.lines, "\n"))
		words := len(strings.Fields(text))
		chapters = append(chapters, model.Chapter{
			Index:     i,
			Title:     d.title,
			Text:      text,
			CharCount: len(text),
			WordCount: words,
		})
	}
	return chapters
}

func splitLines(content string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
