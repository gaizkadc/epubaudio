package epub

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var (
	spaceRegex    = regexp.MustCompile(`[ \t\r\f]+`)
	multipleNL    = regexp.MustCompile(`\n{3,}`)
)

// CleanHTML extracts readable plain text and detects a title from an HTML/XHTML document.
func CleanHTML(r io.Reader) (title string, text string, err error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", "", err
	}

	var buf bytes.Buffer
	var headingTitle string
	var headTitle string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			// Skip non-body and invisible elements
			switch tag {
			case "style", "script", "noscript", "svg", "nav":
				return
			case "head":
				// Scan head for <title>, then skip the rest
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && strings.ToLower(c.Data) == "title" {
						headTitle = extractNodeText(c)
					}
				}
				return
			case "h1", "h2", "h3":
				if headingTitle == "" {
					hText := strings.TrimSpace(extractNodeText(n))
					if len(hText) > 0 && len(hText) < 120 {
						headingTitle = hText
					}
				}
				buf.WriteString("\n\n")
			case "p", "div", "blockquote", "section", "article":
				buf.WriteString("\n\n")
			case "br", "hr":
				buf.WriteString("\n")
			case "li":
				buf.WriteString("\n• ")
			}
		} else if n.Type == html.TextNode {
			// Replace non-breaking spaces with standard space
			normalized := strings.ReplaceAll(n.Data, "\u00a0", " ")
			normalized = strings.ReplaceAll(normalized, "\u200b", "") // zero-width space
			buf.WriteString(normalized)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}

		// Add newline closing block elements
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			switch tag {
			case "p", "div", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
				buf.WriteString("\n\n")
			case "li":
				buf.WriteString("\n")
			}
		}
	}

	walk(doc)

	// Clean up title
	headTitle = strings.TrimSpace(headTitle)
	headingTitle = strings.TrimSpace(headingTitle)
	if headingTitle != "" {
		title = headingTitle
	} else {
		title = headTitle
	}

	raw := buf.String()

	// Normalize whitespace
	lines := strings.Split(raw, "\n")
	var cleanLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(spaceRegex.ReplaceAllString(l, " "))
		cleanLines = append(cleanLines, trimmed)
	}

	cleaned := strings.Join(cleanLines, "\n")
	cleaned = multipleNL.ReplaceAllString(cleaned, "\n\n")
	cleaned = strings.TrimSpace(cleaned)

	return title, cleaned, nil
}

// extractNodeText extracts all text under a node recursively.
func extractNodeText(n *html.Node) string {
	var b strings.Builder
	var fn func(*html.Node)
	fn = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			fn(c)
		}
	}
	fn(n)
	return b.String()
}

