package epub

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"

	"epubaudio/internal/model"
)

// ContainerXML represents META-INF/container.xml
type containerXML struct {
	Rootfiles struct {
		Rootfile []struct {
			FullPath  string `xml:"full-path,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

// PackageOPF represents the OPF package document
type packageOPF struct {
	Metadata struct {
		Title    []string `xml:"title"`
		Creator  []string `xml:"creator"`
		Language []string `xml:"language"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID        string `xml:"id,attr"`
			Href      string `xml:"href,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Itemrefs []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// NCX represents toc.ncx for EPUB 2 chapter titles
type ncxXML struct {
	NavMap struct {
		NavPoints []navPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

type navPoint struct {
	NavLabel struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	NavPoints []navPoint `xml:"navPoint"`
}

// Open reads an EPUB file and parses its metadata, spine, and chapters in reading order.
func Open(epubPath string) (*model.Book, error) {
	zr, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open epub archive: %w", err)
	}
	defer zr.Close()

	// Map ZIP files for fast lookup
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	book := &model.Book{
		FilePath: epubPath,
	}

	// 1. Locate rootfile from META-INF/container.xml
	rootfilePath, err := findRootfile(files)
	if err != nil {
		// Fallback to searching for .opf file
		rootfilePath = findOPFFallback(files)
	}

	if rootfilePath != "" && files[rootfilePath] != nil {
		if err := parseFromOPF(files, rootfilePath, book); err == nil && len(book.Chapters) > 0 {
			return book, nil
		}
	}

	// 2. Fallback: Parse all HTML files if OPF is missing or corrupt
	if err := parseFallbackHTML(files, book); err != nil {
		return nil, err
	}

	if len(book.Chapters) == 0 {
		return nil, errors.New("no readable text chapters found in epub")
	}

	return book, nil
}

func findRootfile(files map[string]*zip.File) (string, error) {
	f, ok := files["META-INF/container.xml"]
	if !ok {
		return "", errors.New("container.xml not found")
	}
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var c containerXML
	if err := xml.NewDecoder(rc).Decode(&c); err != nil {
		return "", err
	}

	for _, rf := range c.Rootfiles.Rootfile {
		if rf.FullPath != "" {
			return rf.FullPath, nil
		}
	}
	return "", errors.New("no rootfile specified in container.xml")
}

func findOPFFallback(files map[string]*zip.File) string {
	for name := range files {
		if strings.HasSuffix(strings.ToLower(name), ".opf") {
			return name
		}
	}
	return ""
}

func parseFromOPF(files map[string]*zip.File, opfPath string, book *model.Book) error {
	rc, err := files[opfPath].Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	var opf packageOPF
	if err := xml.NewDecoder(rc).Decode(&opf); err != nil {
		return err
	}

	if len(opf.Metadata.Title) > 0 {
		book.Title = opf.Metadata.Title[0]
	}
	if len(opf.Metadata.Creator) > 0 {
		book.Author = opf.Metadata.Creator[0]
	}
	if len(opf.Metadata.Language) > 0 {
		book.Language = opf.Metadata.Language[0]
	}

	opfDir := path.Dir(opfPath)
	if opfDir == "." {
		opfDir = ""
	}

	// Build manifest item map (ID -> Href, MediaType)
	type manifestItem struct {
		Href      string
		MediaType string
	}
	manifest := make(map[string]manifestItem, len(opf.Manifest.Items))
	var ncxHref string
	for _, it := range opf.Manifest.Items {
		decodedHref, err := url.PathUnescape(it.Href)
		if err != nil {
			decodedHref = it.Href
		}
		manifest[it.ID] = manifestItem{
			Href:      decodedHref,
			MediaType: it.MediaType,
		}
		if it.MediaType == "application/x-dtbncx+xml" || strings.HasSuffix(it.Href, ".ncx") {
			ncxHref = decodedHref
		}
	}

	// Optional: read NCX table of contents for clean chapter titles
	tocTitles := make(map[string]string)
	if ncxHref != "" {
		ncxZipPath := resolvePath(opfDir, ncxHref)
		if ncxFile, ok := files[ncxZipPath]; ok {
			if ncxRC, err := ncxFile.Open(); err == nil {
				parseNCX(ncxRC, tocTitles)
				ncxRC.Close()
			}
		}
	}

	// Read chapters in spine order
	chapterIndex := 0
	for _, itemref := range opf.Spine.Itemrefs {
		item, ok := manifest[itemref.IDRef]
		if !ok {
			continue
		}

		// Only HTML/XHTML documents
		isHTML := strings.Contains(item.MediaType, "html") ||
			strings.HasSuffix(strings.ToLower(item.Href), ".xhtml") ||
			strings.HasSuffix(strings.ToLower(item.Href), ".html")
		if !isHTML {
			continue
		}

		zipEntryPath := resolvePath(opfDir, item.Href)
		zf, ok := files[zipEntryPath]
		if !ok {
			continue
		}

		zrc, err := zf.Open()
		if err != nil {
			continue
		}

		htmlTitle, text, err := CleanHTML(zrc)
		zrc.Close()
		if err != nil {
			continue
		}

		// Skip covers or blank spacer pages (< 80 characters of text)
		if len(text) < 80 {
			continue
		}

		// Determine best title
		title := htmlTitle
		// Check NCX title mapping
		cleanHref := strings.Split(item.Href, "#")[0]
		if tocT, ok := tocTitles[cleanHref]; ok && tocT != "" {
			title = tocT
		}
		if title == "" {
			base := path.Base(item.Href)
			title = strings.TrimSuffix(base, path.Ext(base))
		}

		words := len(strings.Fields(text))

		book.Chapters = append(book.Chapters, model.Chapter{
			Index:     chapterIndex,
			ID:        itemref.IDRef,
			Href:      item.Href,
			Title:     title,
			Text:      text,
			CharCount: len(text),
			WordCount: words,
		})
		chapterIndex++
	}

	return nil
}

func resolvePath(baseDir, href string) string {
	if baseDir == "" {
		return href
	}
	return path.Clean(path.Join(baseDir, href))
}

func parseNCX(r io.Reader, out map[string]string) {
	var ncx ncxXML
	if err := xml.NewDecoder(r).Decode(&ncx); err != nil {
		return
	}

	var walkNP func(np navPoint)
	walkNP = func(np navPoint) {
		cleanSrc := strings.TrimSpace(np.Content.Src)
		if idx := strings.Index(cleanSrc, "#"); idx != -1 {
			cleanSrc = cleanSrc[:idx]
		}
		title := strings.TrimSpace(np.NavLabel.Text)
		if cleanSrc != "" && title != "" {
			if _, exists := out[cleanSrc]; !exists {
				out[cleanSrc] = title
			}
		}
		for _, child := range np.NavPoints {
			walkNP(child)
		}
	}

	for _, np := range ncx.NavMap.NavPoints {
		walkNP(np)
	}
}

func parseFallbackHTML(files map[string]*zip.File, book *model.Book) error {
	var names []string
	for name := range files {
		lower := strings.ToLower(name)
		if (strings.HasSuffix(lower, ".xhtml") || strings.HasSuffix(lower, ".html")) &&
			!strings.Contains(lower, "nav") && !strings.Contains(lower, "toc") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	idx := 0
	for _, name := range names {
		zf := files[name]
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		htmlTitle, text, err := CleanHTML(rc)
		rc.Close()
		if err != nil || len(text) < 80 {
			continue
		}

		title := htmlTitle
		if title == "" {
			base := path.Base(name)
			title = strings.TrimSuffix(base, path.Ext(base))
		}

		book.Chapters = append(book.Chapters, model.Chapter{
			Index:     idx,
			ID:        name,
			Href:      name,
			Title:     title,
			Text:      text,
			CharCount: len(text),
			WordCount: len(strings.Fields(text)),
		})
		idx++
	}

	if book.Title == "" && len(book.Chapters) > 0 {
		base := path.Base(book.FilePath)
		book.Title = strings.TrimSuffix(base, path.Ext(base))
	}

	return nil
}

