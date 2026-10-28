package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"epubaudio/internal/audio"
	"epubaudio/internal/chunker"
	"epubaudio/internal/model"
	"epubaudio/internal/tts"
)

// OutputFormat defines the target audio format.
type OutputFormat string

const (
	FormatM4B       OutputFormat = "m4b"       // Combined AAC .m4b audiobook with chapters (requires ffmpeg)
	FormatMP3Single OutputFormat = "mp3"       // Combined single .mp3 file
	FormatDirectory OutputFormat = "directory" // Folder with separate chapter MP3 files
)

// ProgressEvent represents progress updates during conversion.
type ProgressEvent struct {
	Stage        string // "start", "chapter_start", "chunk_done", "chapter_done", "packaging", "done"
	ChapterIndex int
	ChapterTitle string
	CurrentChunk int
	TotalChunks  int
	DoneChapters int
	TotalChapters int
	Message      string
}

// Config holds options for the converter service.
type Config struct {
	Synthesizer tts.Synthesizer
	Chunker     *chunker.Chunker
	Packager    *audio.Packager
	TTSOptions  tts.Options
	CacheDir    string
	Workers     int
	KeepTemp    bool
	Format      OutputFormat
	OutputPath  string
	OnProgress  func(ProgressEvent)
}

// Converter coordinates EPUB chapters, text chunking, TTS synthesis, and audio packaging.
type Converter struct {
	cfg Config
}

// New creates a new Converter instance.
func New(cfg Config) *Converter {
	if cfg.Chunker == nil {
		cfg.Chunker = chunker.New(chunker.DefaultMaxChunkSize)
	}
	if cfg.Packager == nil {
		cfg.Packager = audio.NewPackager()
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(os.TempDir(), "epubaudio_cache")
	}
	if cfg.Format == "" {
		if cfg.Packager.HasFFmpeg() {
			cfg.Format = FormatM4B
		} else {
			cfg.Format = FormatMP3Single
		}
	}
	return &Converter{cfg: cfg}
}

// Convert converts the specified chapters of a book to the desired audio format.
func (c *Converter) Convert(ctx context.Context, book *model.Book, chapterIndices []int) (string, error) {
	if len(chapterIndices) == 0 {
		return "", fmt.Errorf("no chapters selected for conversion")
	}

	if err := os.MkdirAll(c.cfg.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}
	if !c.cfg.KeepTemp {
		defer os.RemoveAll(c.cfg.CacheDir)
	}

	// Filter chapters by selected indices
	var selected []model.Chapter
	indexMap := make(map[int]bool)
	for _, idx := range chapterIndices {
		indexMap[idx] = true
	}
	for _, ch := range book.Chapters {
		if indexMap[ch.Index] {
			selected = append(selected, ch)
		}
	}

	if len(selected) == 0 {
		return "", fmt.Errorf("none of the requested chapter indices were found in the book")
	}

	c.emit(ProgressEvent{
		Stage:         "start",
		TotalChapters: len(selected),
		Message:       fmt.Sprintf("Starting conversion of %d chapters using %s voice '%s'", len(selected), c.cfg.Synthesizer.Name(), c.cfg.TTSOptions.Voice),
	})

	var chapterAudios []audio.ChapterAudio
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, c.cfg.Workers)
	errChan := make(chan error, len(selected))

	for i, ch := range selected {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		wg.Add(1)
		go func(chap model.Chapter, displayIdx int) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			c.emit(ProgressEvent{
				Stage:         "chapter_start",
				ChapterIndex: chap.Index,
				ChapterTitle: chap.Title,
				DoneChapters: displayIdx,
				TotalChapters: len(selected),
				Message:       fmt.Sprintf("Synthesizing chapter %d/%d: %s", displayIdx+1, len(selected), chap.Title),
			})

			chAudioPath, err := c.synthesizeChapter(ctx, chap)
			if err != nil {
				errChan <- fmt.Errorf("chapter %d (%s) failed: %w", chap.Index, chap.Title, err)
				return
			}

			dur, _ := c.cfg.Packager.GetAudioDuration(chAudioPath)

			mu.Lock()
			chapterAudios = append(chapterAudios, audio.ChapterAudio{
				Index:     chap.Index,
				Title:     chap.Title,
				AudioPath: chAudioPath,
				Duration:  dur,
			})
			mu.Unlock()

			c.emit(ProgressEvent{
				Stage:         "chapter_done",
				ChapterIndex: chap.Index,
				ChapterTitle: chap.Title,
				DoneChapters: displayIdx + 1,
				TotalChapters: len(selected),
				Message:       fmt.Sprintf("Finished chapter %d/%d: %s", displayIdx+1, len(selected), chap.Title),
			})
		}(ch, i)
	}

	wg.Wait()
	close(errChan)

	if err, ok := <-errChan; ok {
		return "", err
	}

	// Sort chapterAudios by index
	sortedAudios := make([]audio.ChapterAudio, 0, len(chapterAudios))
	for _, ch := range selected {
		for _, ca := range chapterAudios {
			if ca.Index == ch.Index {
				sortedAudios = append(sortedAudios, ca)
				break
			}
		}
	}

	// Output packaging
	finalOutput := c.determineOutputPath(book)
	c.emit(ProgressEvent{
		Stage:   "packaging",
		Message: fmt.Sprintf("Packaging %d chapters into format '%s' at: %s", len(sortedAudios), c.cfg.Format, finalOutput),
	})

	meta := audio.BookMeta{
		Title:    book.Title,
		Author:   book.Author,
		Language: book.Language,
	}

	var pkgErr error
	switch c.cfg.Format {
	case FormatM4B:
		pkgErr = c.cfg.Packager.PackageM4B(ctx, sortedAudios, meta, finalOutput)
	case FormatDirectory:
		pkgErr = c.cfg.Packager.PackageDirectory(sortedAudios, finalOutput)
	case FormatMP3Single:
		fallthrough
	default:
		pkgErr = c.cfg.Packager.PackageMP3Single(ctx, sortedAudios, meta, finalOutput)
	}

	if pkgErr != nil {
		return "", fmt.Errorf("packaging failed: %w", pkgErr)
	}

	c.emit(ProgressEvent{
		Stage:   "done",
		Message: fmt.Sprintf("Successfully generated: %s", finalOutput),
	})

	return finalOutput, nil
}

func (c *Converter) synthesizeChapter(ctx context.Context, ch model.Chapter) (string, error) {
	chapterFinalPath := filepath.Join(c.cfg.CacheDir, fmt.Sprintf("chapter_%04d.mp3", ch.Index))

	// Check if already completely synthesized (resumable)
	if fi, err := os.Stat(chapterFinalPath); err == nil && fi.Size() > 1000 {
		return chapterFinalPath, nil
	}

	chunks := c.cfg.Chunker.Split(ch.Text)
	if len(chunks) == 0 {
		return "", fmt.Errorf("no text chunks to synthesize in chapter %d", ch.Index)
	}

	if len(chunks) == 1 {
		err := c.cfg.Synthesizer.Synthesize(ctx, chunks[0], chapterFinalPath, c.cfg.TTSOptions)
		if err != nil {
			return "", err
		}
		return chapterFinalPath, nil
	}

	// Multiple chunks: synthesize each and stitch
	var chunkPaths []string
	for idx, chunkText := range chunks {
		chunkFile := filepath.Join(c.cfg.CacheDir, fmt.Sprintf("chapter_%04d_chunk_%03d.mp3", ch.Index, idx))

		// Check if chunk exists and has size
		if fi, err := os.Stat(chunkFile); err == nil && fi.Size() > 500 {
			chunkPaths = append(chunkPaths, chunkFile)
			continue
		}

		err := c.cfg.Synthesizer.Synthesize(ctx, chunkText, chunkFile, c.cfg.TTSOptions)
		if err != nil {
			return "", fmt.Errorf("chunk %d of chapter %d failed: %w", idx, ch.Index, err)
		}

		chunkPaths = append(chunkPaths, chunkFile)
		c.emit(ProgressEvent{
			Stage:        "chunk_done",
			ChapterIndex: ch.Index,
			ChapterTitle: ch.Title,
			CurrentChunk: idx + 1,
			TotalChunks:  len(chunks),
		})
	}

	// Concat all chunks into chapter audio
	if err := c.cfg.Packager.ConcatFiles(chunkPaths, chapterFinalPath); err != nil {
		return "", fmt.Errorf("failed to stitch chunks for chapter %d: %w", ch.Index, err)
	}

	return chapterFinalPath, nil
}

func (c *Converter) determineOutputPath(book *model.Book) string {
	_ = os.MkdirAll("output", 0755)

	if c.cfg.OutputPath != "" {
		// If user specified just a base file name (e.g. "my_book.m4b"), route to output folder
		if filepath.Dir(c.cfg.OutputPath) == "." && !strings.HasPrefix(c.cfg.OutputPath, ".") {
			return filepath.Join("output", c.cfg.OutputPath)
		}
		return c.cfg.OutputPath
	}

	base := "audiobook"
	if book.Title != "" {
		base = book.Title
	} else if book.FilePath != "" {
		base = strings.TrimSuffix(filepath.Base(book.FilePath), filepath.Ext(book.FilePath))
	}

	synthName := "edge-tts"
	if c.cfg.Synthesizer != nil {
		synthName = c.cfg.Synthesizer.Name()
	}

	base = fmt.Sprintf("%s [%s]", audio.SanitizeBaseName(base), synthName)

	switch c.cfg.Format {
	case FormatM4B:
		return filepath.Join("output", base+".m4b")
	case FormatDirectory:
		return filepath.Join("output", base+"_audio")
	default:
		return filepath.Join("output", base+".mp3")
	}
}

func (c *Converter) emit(ev ProgressEvent) {
	if c.cfg.OnProgress != nil {
		c.cfg.OnProgress(ev)
	}
}
