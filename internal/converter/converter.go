package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

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
	Stage         string `json:"stage"`         // "start", "chapter_start", "chunk_done", "chapter_done", "packaging", "done"
	ChapterIndex  int    `json:"chapterIndex"`
	ChapterTitle  string `json:"chapterTitle"`
	CurrentChunk  int    `json:"currentChunk"`
	TotalChunks   int    `json:"totalChunks"`
	DoneChapters  int    `json:"doneChapters"`
	TotalChapters int    `json:"totalChapters"`
	Message       string `json:"message"`
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

	for i, ch := range selected {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		chunks := c.cfg.Chunker.Split(ch.Text)
		numChunks := len(chunks)
		if numChunks == 0 {
			numChunks = 1
		}

		c.emit(ProgressEvent{
			Stage:         "chapter_start",
			ChapterIndex:  ch.Index,
			ChapterTitle:  ch.Title,
			DoneChapters:  i,
			TotalChapters: len(selected),
			TotalChunks:   numChunks,
			Message:       fmt.Sprintf("Synthesizing chapter %d/%d: %s", i+1, len(selected), ch.Title),
		})

		chAudioPath, err := c.synthesizeChapter(ctx, ch, chunks, i+1, len(selected))
		if err != nil {
			return "", fmt.Errorf("chapter %d (%s) failed: %w", ch.Index, ch.Title, err)
		}

		dur, _ := c.cfg.Packager.GetAudioDuration(chAudioPath)
		chapterAudios = append(chapterAudios, audio.ChapterAudio{
			Index:     ch.Index,
			Title:     ch.Title,
			AudioPath: chAudioPath,
			Duration:  dur,
		})

		c.emit(ProgressEvent{
			Stage:         "chapter_done",
			ChapterIndex:  ch.Index,
			ChapterTitle:  ch.Title,
			DoneChapters:  i + 1,
			TotalChapters: len(selected),
			TotalChunks:   numChunks,
			Message:       fmt.Sprintf("Finished chapter %d/%d: %s", i+1, len(selected), ch.Title),
		})
	}

	// Output packaging
	finalOutput := c.determineOutputPath(book)
	c.emit(ProgressEvent{
		Stage:   "packaging",
		Message: fmt.Sprintf("Packaging %d chapters into format '%s' at: %s", len(chapterAudios), c.cfg.Format, finalOutput),
	})

	meta := audio.BookMeta{
		Title:    book.Title,
		Author:   book.Author,
		Language: book.Language,
	}

	var pkgErr error
	switch c.cfg.Format {
	case FormatM4B:
		pkgErr = c.cfg.Packager.PackageM4B(ctx, chapterAudios, meta, finalOutput)
	case FormatDirectory:
		pkgErr = c.cfg.Packager.PackageDirectory(chapterAudios, finalOutput)
	case FormatMP3Single:
		fallthrough
	default:
		pkgErr = c.cfg.Packager.PackageMP3Single(ctx, chapterAudios, meta, finalOutput)
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

func (c *Converter) synthesizeChapter(ctx context.Context, ch model.Chapter, chunks []string, chapNum, totalChaps int) (string, error) {
	chapterFinalPath := filepath.Join(c.cfg.CacheDir, fmt.Sprintf("chapter_%04d.mp3", ch.Index))

	// Check if already completely synthesized (resumable)
	if fi, err := os.Stat(chapterFinalPath); err == nil && fi.Size() > 1000 {
		c.emit(ProgressEvent{
			Stage:         "chunk_done",
			ChapterIndex:  ch.Index,
			ChapterTitle:  ch.Title,
			CurrentChunk:  len(chunks),
			TotalChunks:   len(chunks),
			DoneChapters:  chapNum,
			TotalChapters: totalChaps,
		})
		return chapterFinalPath, nil
	}

	if len(chunks) == 0 {
		return "", fmt.Errorf("no text chunks to synthesize in chapter %d", ch.Index)
	}

	if len(chunks) == 1 {
		err := c.cfg.Synthesizer.Synthesize(ctx, chunks[0], chapterFinalPath, c.cfg.TTSOptions)
		if err != nil {
			return "", err
		}
		c.emit(ProgressEvent{
			Stage:         "chunk_done",
			ChapterIndex:  ch.Index,
			ChapterTitle:  ch.Title,
			CurrentChunk:  1,
			TotalChunks:   1,
			DoneChapters:  chapNum,
			TotalChapters: totalChaps,
		})
		return chapterFinalPath, nil
	}

	// Multiple chunks: synthesize concurrently using worker pool!
	chunkPaths := make([]string, len(chunks))
	type chunkJob struct {
		index int
		text  string
		path  string
	}

	jobs := make(chan chunkJob, len(chunks))
	for idx, chunkText := range chunks {
		chunkFile := filepath.Join(c.cfg.CacheDir, fmt.Sprintf("chapter_%04d_chunk_%03d.mp3", ch.Index, idx))
		chunkPaths[idx] = chunkFile
		jobs <- chunkJob{index: idx, text: chunkText, path: chunkFile}
	}
	close(jobs)

	numWorkers := c.cfg.Workers
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > len(chunks) {
		numWorkers = len(chunks)
	}

	var chunkWg sync.WaitGroup
	chunkErrChan := make(chan error, len(chunks))
	var completedChunks atomic.Int32

	for w := 0; w < numWorkers; w++ {
		chunkWg.Add(1)
		go func() {
			defer chunkWg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Check cache
				if fi, err := os.Stat(job.path); err == nil && fi.Size() > 500 {
					done := completedChunks.Add(1)
					c.emit(ProgressEvent{
						Stage:         "chunk_done",
						ChapterIndex:  ch.Index,
						ChapterTitle:  ch.Title,
						CurrentChunk:  int(done),
						TotalChunks:   len(chunks),
						DoneChapters:  chapNum,
						TotalChapters: totalChaps,
					})
					continue
				}

				err := c.cfg.Synthesizer.Synthesize(ctx, job.text, job.path, c.cfg.TTSOptions)
				if err != nil {
					chunkErrChan <- fmt.Errorf("chunk %d of chapter %d failed: %w", job.index+1, ch.Index, err)
					return
				}

				done := completedChunks.Add(1)
				c.emit(ProgressEvent{
					Stage:         "chunk_done",
					ChapterIndex:  ch.Index,
					ChapterTitle:  ch.Title,
					CurrentChunk:  int(done),
					TotalChunks:   len(chunks),
					DoneChapters:  chapNum,
					TotalChapters: totalChaps,
				})
			}
		}()
	}

	chunkWg.Wait()
	close(chunkErrChan)

	if err, ok := <-chunkErrChan; ok {
		return "", err
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
