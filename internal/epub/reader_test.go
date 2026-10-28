package epub

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenEPUB(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// 1. mimetype
	f, _ := zw.Create("mimetype")
	f.Write([]byte("application/epub+zip"))

	// 2. META-INF/container.xml
	f, _ = zw.Create("META-INF/container.xml")
	f.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// 3. OEBPS/content.opf
	f, _ = zw.Create("OEBPS/content.opf")
	f.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Novel</dc:title>
    <dc:creator>Jane Doe</dc:creator>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="c1" href="chap1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="chap2.xhtml" media-type="application/xhtml+xml"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="c2"/>
    <itemref idref="c1"/>
  </spine>
</package>`))

	// 4. OEBPS/toc.ncx
	f, _ = zw.Create("OEBPS/toc.ncx")
	f.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="np-1">
      <navLabel><text>Official Chapter Two</text></navLabel>
      <content src="chap2.xhtml"/>
    </navPoint>
    <navPoint id="np-2">
      <navLabel><text>Official Chapter One</text></navLabel>
      <content src="chap1.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`))

	// 5. OEBPS/chap1.xhtml
	f, _ = zw.Create("OEBPS/chap1.xhtml")
	f.Write([]byte(`<!DOCTYPE html><html><body><h1>Chapter One</h1><p>` +
		`This is the full text of chapter one. It has plenty of words and content to exceed the minimum threshold.` +
		`</p></body></html>`))

	// 6. OEBPS/chap2.xhtml
	f, _ = zw.Create("OEBPS/chap2.xhtml")
	f.Write([]byte(`<!DOCTYPE html><html><body><h1>Chapter Two</h1><p>` +
		`This is chapter two, which appears first in the spine. It also has sufficient character length to be considered a chapter.` +
		`</p></body></html>`))

	zw.Close()

	// Write to temp file
	tmpFile := filepath.Join(t.TempDir(), "test.epub")
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	book, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if book.Title != "Test Novel" {
		t.Errorf("expected book title 'Test Novel', got %q", book.Title)
	}
	if book.Author != "Jane Doe" {
		t.Errorf("expected author 'Jane Doe', got %q", book.Author)
	}
	if book.Language != "en" {
		t.Errorf("expected language 'en', got %q", book.Language)
	}

	if len(book.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(book.Chapters))
	}

	// Verify spine order: chap2 came first in spine!
	if book.Chapters[0].Title != "Official Chapter Two" {
		t.Errorf("expected first chapter to be 'Official Chapter Two' (spine order), got %q", book.Chapters[0].Title)
	}
	if book.Chapters[1].Title != "Official Chapter One" {
		t.Errorf("expected second chapter to be 'Official Chapter One', got %q", book.Chapters[1].Title)
	}
}

