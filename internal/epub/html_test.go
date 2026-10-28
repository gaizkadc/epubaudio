package epub

import (
	"strings"
	"testing"
)

func TestCleanHTML(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Doc Title</title>
    <style>body { font-size: 14px; }</style>
    <script>console.log("hello");</script>
</head>
<body>
    <h1>Chapter 1: The Odyssey</h1>
    <p>This is the first paragraph with &ldquo;quotes&rdquo; and an &mdash; em-dash &amp; &nbsp;spaces.</p>
    <div>Another line of <strong>bold</strong> and <em>italic</em> text.</div>
    <nav><a href="#foo">Ignore navigation</a></nav>
</body>
</html>`

	title, text, err := CleanHTML(strings.NewReader(rawHTML))
	if err != nil {
		t.Fatalf("CleanHTML failed: %v", err)
	}

	if title != "Chapter 1: The Odyssey" {
		t.Errorf("expected title 'Chapter 1: The Odyssey', got %q", title)
	}

	if strings.Contains(text, "font-size") {
		t.Errorf("text should not contain style contents: %s", text)
	}
	if strings.Contains(text, "console.log") {
		t.Errorf("text should not contain script contents: %s", text)
	}
	if strings.Contains(text, "Ignore navigation") {
		t.Errorf("text should not contain nav contents: %s", text)
	}

	if !strings.Contains(text, "“quotes”") && !strings.Contains(text, "\"quotes\"") {
		t.Errorf("HTML entities were not unescaped properly: %s", text)
	}
	if !strings.Contains(text, "bold and italic text") {
		t.Errorf("inline formatting not preserved cleanly: %s", text)
	}
}

