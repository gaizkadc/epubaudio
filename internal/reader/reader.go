package reader

import (
	"fmt"
	"path/filepath"
	"strings"

	"epubaudio/internal/epub"
	"epubaudio/internal/model"
)

// SupportedExtensions lists all supported input document formats.
var SupportedExtensions = []string{".epub", ".txt", ".pdf"}

// IsSupported returns true if the file path has a supported extension.
func IsSupported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, supported := range SupportedExtensions {
		if ext == supported {
			return true
		}
	}
	return false
}

// Open loads and parses an eBook or document from path (.epub, .txt, or .pdf).
func Open(path string) (*model.Book, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".epub":
		return epub.Open(path)
	case ".txt":
		return OpenTXT(path)
	case ".pdf":
		return OpenPDF(path)
	default:
		return nil, fmt.Errorf("unsupported document format %q (supported formats: %s)", ext, strings.Join(SupportedExtensions, ", "))
	}
}

// CleanTitle derives a clean title from a file path.
func CleanTitle(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	title := strings.TrimSuffix(base, ext)
	// Replace underscores and excessive spaces
	title = strings.ReplaceAll(title, "_", " ")
	return strings.TrimSpace(title)
}

// DetectLanguage estimates the primary language of text using high-frequency stopword analysis.
func DetectLanguage(text string) string {
	lower := " " + strings.ToLower(text) + " "

	// Common short words distinctive to languages
	scores := map[string]int{
		"es": countWords(lower, []string{" de ", " la ", " el ", " en ", " que ", " los ", " del ", " las ", " por ", " un ", " para ", " con ", " no ", " una "}),
		"en": countWords(lower, []string{" the ", " of ", " and ", " to ", " in ", " is ", " that ", " for ", " it ", " with ", " as ", " was ", " on ", " are "}),
		"fr": countWords(lower, []string{" de ", " le ", " la ", " et ", " les ", " des ", " en ", " un ", " du ", " une ", " que ", " est ", " pour ", " dans "}),
		"de": countWords(lower, []string{" der ", " die ", " das ", " und ", " in ", " den ", " von ", " zu ", " mit ", " sich ", " des ", " ist ", " auf ", " für "}),
		"it": countWords(lower, []string{" di ", " che ", " e ", " la ", " il ", " un ", " a ", " per ", " in ", " una ", " del ", " sono ", " con ", " non "}),
		"pt": countWords(lower, []string{" de ", " que ", " o ", " a ", " e ", " do ", " da ", " em ", " um ", " para ", " com ", " não ", " uma ", " os "}),
	}

	bestLang := "en"
	bestScore := 0
	for lang, score := range scores {
		if score > bestScore {
			bestScore = score
			bestLang = lang
		}
	}

	if bestScore < 3 {
		return "es" // Default fallback if too short to determine
	}
	return bestLang
}

func countWords(text string, words []string) int {
	count := 0
	for _, w := range words {
		count += strings.Count(text, w)
	}
	return count
}
