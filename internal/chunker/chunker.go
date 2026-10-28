package chunker

import (
	"strings"
	"unicode"
)

const (
	// DefaultMaxChunkSize is the default character threshold before splitting a chunk.
	DefaultMaxChunkSize = 4000
)

// Chunker splits large text into manageable chunks respecting sentence and paragraph boundaries.
type Chunker struct {
	MaxChars int
}

// New creates a new Chunker with the specified maximum chunk character size.
func New(maxChars int) *Chunker {
	if maxChars <= 0 {
		maxChars = DefaultMaxChunkSize
	}
	return &Chunker{MaxChars: maxChars}
}

// Split splits text into chunks, prioritizing paragraph breaks, then sentence boundaries.
func (c *Chunker) Split(text string) []string {
	text = strings.TrimSpace(text)
	if len(text) <= c.MaxChars {
		if text == "" {
			return nil
		}
		return []string{text}
	}

	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var currentChunk strings.Builder

	flushCurrent := func() {
		if currentChunk.Len() > 0 {
			res := strings.TrimSpace(currentChunk.String())
			if res != "" {
				chunks = append(chunks, res)
			}
			currentChunk.Reset()
		}
	}

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If paragraph fits in current chunk:
		if currentChunk.Len()+len(para)+2 <= c.MaxChars {
			if currentChunk.Len() > 0 {
				currentChunk.WriteString("\n\n")
			}
			currentChunk.WriteString(para)
			continue
		}

		// If current chunk has content, flush it
		flushCurrent()

		// If the paragraph itself fits within MaxChars, start a new chunk with it
		if len(para) <= c.MaxChars {
			currentChunk.WriteString(para)
			continue
		}

		// Paragraph exceeds MaxChars: split by sentences
		sentenceChunks := c.splitParagraph(para)
		for _, s := range sentenceChunks {
			if currentChunk.Len()+len(s)+1 <= c.MaxChars {
				if currentChunk.Len() > 0 {
					currentChunk.WriteString(" ")
				}
				currentChunk.WriteString(s)
			} else {
				flushCurrent()
				currentChunk.WriteString(s)
			}
		}
	}

	flushCurrent()
	return chunks
}

// splitParagraph splits a single long paragraph by sentence punctuation (. ! ?).
func (c *Chunker) splitParagraph(para string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(para)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		current.WriteRune(r)

		// Check if this rune is sentence-ending punctuation (. ! ? …)
		isPunct := r == '.' || r == '!' || r == '?' || r == '…' || r == '。' || r == '！' || r == '？'
		if isPunct {
			// Check if followed by whitespace or quote + whitespace or end of string
			isEnd := false
			if i+1 == n {
				isEnd = true
			} else if unicode.IsSpace(runes[i+1]) {
				isEnd = true
			} else if (runes[i+1] == '"' || runes[i+1] == '\'' || runes[i+1] == '»' || runes[i+1] == '”') &&
				(i+2 >= n || unicode.IsSpace(runes[i+2])) {
				// Include the closing quote
				i++
				current.WriteRune(runes[i])
				isEnd = true
			}

			if isEnd {
				s := strings.TrimSpace(current.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		s := strings.TrimSpace(current.String())
		if s != "" {
			sentences = append(sentences, s)
		}
	}

	// Now group sentences into segments <= MaxChars
	var result []string
	var seg strings.Builder

	for _, s := range sentences {
		if len(s) > c.MaxChars {
			if seg.Len() > 0 {
				result = append(result, strings.TrimSpace(seg.String()))
				seg.Reset()
			}
			// Hard-split extremely long sentence by word boundaries
			result = append(result, c.splitByWords(s)...)
			continue
		}

		if seg.Len()+len(s)+1 <= c.MaxChars {
			if seg.Len() > 0 {
				seg.WriteString(" ")
			}
			seg.WriteString(s)
		} else {
			if seg.Len() > 0 {
				result = append(result, strings.TrimSpace(seg.String()))
				seg.Reset()
			}
			seg.WriteString(s)
		}
	}

	if seg.Len() > 0 {
		result = append(result, strings.TrimSpace(seg.String()))
	}

	return result
}

// splitByWords splits a continuous text without sentence punctuation by spaces.
func (c *Chunker) splitByWords(text string) []string {
	words := strings.Fields(text)
	var res []string
	var b strings.Builder

	for _, w := range words {
		if b.Len()+len(w)+1 <= c.MaxChars {
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(w)
		} else {
			if b.Len() > 0 {
				res = append(res, b.String())
				b.Reset()
			}
			b.WriteString(w)
		}
	}
	if b.Len() > 0 {
		res = append(res, b.String())
	}
	return res
}

