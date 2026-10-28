package audio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ChapterAudio holds info about a completed chapter audio file.
type ChapterAudio struct {
	Index     int
	Title     string
	AudioPath string
	Duration  time.Duration
}

// BookMeta holds metadata for the final audio file tagging.
type BookMeta struct {
	Title    string
	Author   string
	Language string
}

// Packager handles stitching chunk files and packaging chapters into M4B, MP3, or folders.
type Packager struct {
	hasFFmpeg  bool
	hasFFprobe bool
}

// NewPackager initializes an audio packager and detects ffmpeg tools.
func NewPackager() *Packager {
	_, ffmpegErr := exec.LookPath("ffmpeg")
	_, ffprobeErr := exec.LookPath("ffprobe")
	return &Packager{
		hasFFmpeg:  ffmpegErr == nil,
		hasFFprobe: ffprobeErr == nil,
	}
}

// HasFFmpeg returns true if ffmpeg executable is available on PATH.
func (p *Packager) HasFFmpeg() bool {
	return p.hasFFmpeg
}

// ConcatFiles concatenates multiple audio files into a single destination file.
// For MP3s with matching bitrate, raw binary concatenation is valid and lossless.
func (p *Packager) ConcatFiles(inputs []string, destPath string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no input files to concatenate")
	}
	if len(inputs) == 1 {
		return copyFile(inputs[0], destPath)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()

	bufDest := bufio.NewWriter(dest)
	for _, in := range inputs {
		f, err := os.Open(in)
		if err != nil {
			return err
		}
		if _, err := io.Copy(bufDest, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	return bufDest.Flush()
}

// GetAudioDuration returns the duration of an audio file using ffprobe, or approximates it.
func (p *Packager) GetAudioDuration(path string) (time.Duration, error) {
	if p.hasFFprobe {
		cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
		out, err := cmd.Output()
		if err == nil {
			secs, parseErr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
			if parseErr == nil {
				return time.Duration(secs * float64(time.Second)), nil
			}
		}
	}

	// Fallback approximation for 128kbps MP3 if ffprobe is absent
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	// 128 kbps = 16,000 bytes/sec
	bytes := fi.Size()
	secs := float64(bytes) / 16000.0
	return time.Duration(secs * float64(time.Second)), nil
}

// PackageM4B packages chapter audio files into an M4B audiobook with chapter markers.
func (p *Packager) PackageM4B(ctx context.Context, chapters []ChapterAudio, meta BookMeta, outPath string) error {
	if !p.hasFFmpeg {
		return fmt.Errorf("ffmpeg is required to generate .m4b audiobooks (run 'brew install ffmpeg')")
	}

	tempDir := filepath.Dir(outPath)
	concatFile := filepath.Join(tempDir, "ffmpeg_concat.txt")
	metaFile := filepath.Join(tempDir, "ffmpeg_metadata.txt")

	// 1. Create concat list
	var concatContent strings.Builder
	for _, ch := range chapters {
		absPath, err := filepath.Abs(ch.AudioPath)
		if err != nil {
			absPath = ch.AudioPath
		}
		concatContent.WriteString(fmt.Sprintf("file '%s'\n", strings.ReplaceAll(absPath, "'", "'\\''")))
	}
	if err := os.WriteFile(concatFile, []byte(concatContent.String()), 0644); err != nil {
		return err
	}
	defer os.Remove(concatFile)

	// 2. Measure chapter durations and build FFMETADATA
	var metaContent strings.Builder
	metaContent.WriteString(";FFMETADATA1\n")
	if meta.Title != "" {
		metaContent.WriteString(fmt.Sprintf("title=%s\n", escapeFFMeta(meta.Title)))
		metaContent.WriteString(fmt.Sprintf("album=%s\n", escapeFFMeta(meta.Title)))
	}
	if meta.Author != "" {
		metaContent.WriteString(fmt.Sprintf("artist=%s\n", escapeFFMeta(meta.Author)))
		metaContent.WriteString(fmt.Sprintf("album_artist=%s\n", escapeFFMeta(meta.Author)))
	}
	metaContent.WriteString("genre=Audiobook\n\n")

	var currentMs int64 = 0
	for _, ch := range chapters {
		dur, _ := p.GetAudioDuration(ch.AudioPath)
		ms := dur.Milliseconds()
		if ms <= 0 {
			ms = 1000 // minimum 1s placeholder
		}

		endMs := currentMs + ms
		metaContent.WriteString("[CHAPTER]\n")
		metaContent.WriteString("TIMEBASE=1/1000\n")
		metaContent.WriteString(fmt.Sprintf("START=%d\n", currentMs))
		metaContent.WriteString(fmt.Sprintf("END=%d\n", endMs))
		metaContent.WriteString(fmt.Sprintf("title=%s\n\n", escapeFFMeta(ch.Title)))

		currentMs = endMs
	}

	if err := os.WriteFile(metaFile, []byte(metaContent.String()), 0644); err != nil {
		return err
	}
	defer os.Remove(metaFile)

	// 3. Run ffmpeg command
	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile,
		"-i", metaFile,
		"-map_metadata", "1",
		"-c:a", "aac",
		"-b:a", "128k",
		outPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg failed (%w): %s", err, string(out))
	}

	return nil
}

// PackageMP3Single concatenates all chapters into a single MP3 file.
func (p *Packager) PackageMP3Single(ctx context.Context, chapters []ChapterAudio, meta BookMeta, outPath string) error {
	var inputPaths []string
	for _, ch := range chapters {
		inputPaths = append(inputPaths, ch.AudioPath)
	}

	if p.hasFFmpeg {
		tempDir := filepath.Dir(outPath)
		concatFile := filepath.Join(tempDir, "ffmpeg_concat_mp3.txt")

		var concatContent strings.Builder
		for _, in := range inputPaths {
			abs, _ := filepath.Abs(in)
			concatContent.WriteString(fmt.Sprintf("file '%s'\n", strings.ReplaceAll(abs, "'", "'\\''")))
		}
		_ = os.WriteFile(concatFile, []byte(concatContent.String()), 0644)
		defer os.Remove(concatFile)

		args := []string{
			"-y",
			"-f", "concat",
			"-safe", "0",
			"-i", concatFile,
			"-c", "copy",
		}
		if meta.Title != "" {
			args = append(args, "-metadata", fmt.Sprintf("title=%s", meta.Title))
			args = append(args, "-metadata", fmt.Sprintf("album=%s", meta.Title))
		}
		if meta.Author != "" {
			args = append(args, "-metadata", fmt.Sprintf("artist=%s", meta.Author))
		}
		args = append(args, outPath)

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if err := cmd.Run(); err == nil {
			return nil
		}
		// If ffmpeg fails, fall back to pure Go concat
	}

	return p.ConcatFiles(inputPaths, outPath)
}

// PackageDirectory exports each chapter as a distinct numbered MP3 file in a directory.
func (p *Packager) PackageDirectory(chapters []ChapterAudio, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	for _, ch := range chapters {
		safeTitle := SanitizeBaseName(ch.Title)
		filename := fmt.Sprintf("%02d - %s.mp3", ch.Index+1, safeTitle)
		dest := filepath.Join(outDir, filename)
		if err := copyFile(ch.AudioPath, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// SanitizeBaseName strips illegal characters for file systems across Windows, Mac, and Linux.
func SanitizeBaseName(s string) string {
	s = strings.TrimSpace(s)
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", " -",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
		"\n", " ",
		"\r", " ",
	)
	clean := replacer.Replace(s)
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 80 {
		clean = clean[:80]
	}
	return strings.TrimSpace(clean)
}

func escapeFFMeta(val string) string {
	val = strings.ReplaceAll(val, "\\", "\\\\")
	val = strings.ReplaceAll(val, "=", "\\=")
	val = strings.ReplaceAll(val, ";", "\\;")
	val = strings.ReplaceAll(val, "#", "\\#")
	val = strings.ReplaceAll(val, "\n", "\\\n")
	return val
}
