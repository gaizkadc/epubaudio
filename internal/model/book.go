package model

import "fmt"

// Chapter represents a parsed chapter or section of an EPUB.
type Chapter struct {
	Index     int    // Sequential reading order (0-indexed)
	ID        string // OPF manifest ID
	Href      string // Relative path in the EPUB archive
	Title     string // Detected chapter title
	Text      string // Cleaned plain text ready for speech synthesis
	CharCount int    // Number of characters in Text
	WordCount int    // Number of words in Text
}

func (c Chapter) String() string {
	return fmt.Sprintf("[%d] %s (%d chars, ~%d words)", c.Index, c.Title, c.CharCount, c.WordCount)
}

// Book represents an EPUB book with metadata and ordered chapters.
type Book struct {
	FilePath string
	Title    string
	Author   string
	Language string
	Chapters []Chapter
}

func (b Book) TotalCharacters() int {
	total := 0
	for _, ch := range b.Chapters {
		total += ch.CharCount
	}
	return total
}

