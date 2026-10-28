package tts

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Piper implements the Synthesizer and VoiceProvider interfaces using Piper TTS.
type Piper struct {
	BinaryPath string
	ModelPath  string
}

// NewPiper creates a Piper synthesizer, auto-locating the piper binary and default models.
func NewPiper(modelPath string) *Piper {
	bin := findPiperBinary()
	if modelPath == "" {
		modelPath = findDefaultPiperModel()
	}
	return &Piper{
		BinaryPath: bin,
		ModelPath:  modelPath,
	}
}

// Name returns the synthesizer identifier.
func (p *Piper) Name() string {
	return "piper"
}

// Synthesize synthesizes speech from text using Piper.
func (p *Piper) Synthesize(ctx context.Context, text string, outPath string, opts Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	bin := p.BinaryPath
	if bin == "" {
		return fmt.Errorf("piper binary not found (install via 'pip install piper-tts')")
	}

	model := p.ModelPath
	if opts.Voice != "" && strings.HasSuffix(opts.Voice, ".onnx") {
		model = opts.Voice
	}
	if model == "" {
		return fmt.Errorf("no piper model (.onnx) specified or found in ~/.local/share/piper/voices/")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	tmpWav := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "_raw.wav"
	defer os.Remove(tmpWav)

	args := []string{
		"--model", model,
		"--output_file", tmpWav,
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		_, _ = io.WriteString(stdin, text)
	}()

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("piper failed (%w): %s", err, string(out))
	}

	// Convert WAV to requested format
	ext := strings.ToLower(filepath.Ext(outPath))
	if ext == ".wav" {
		return os.Rename(tmpWav, outPath)
	}

	if _, err := exec.LookPath("ffmpeg"); err == nil {
		convCmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpWav, "-c:a", "libmp3lame", "-b:a", "128k", outPath)
		if err := convCmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback to macOS afconvert (produces AAC audio in .m4a/.mp3)
	afCmd := exec.CommandContext(ctx, "afconvert", "-f", "m4af", "-d", "aac", tmpWav, outPath)
	if err := afCmd.Run(); err == nil {
		return nil
	}

	// If conversion fails, copy wav directly
	return copyFileDirect(tmpWav, outPath)
}

// ListVoices enumerates available Piper models in standard voice directories.
func (p *Piper) ListVoices(ctx context.Context) ([]Voice, error) {
	home, _ := os.UserHomeDir()
	voiceDir := filepath.Join(home, ".local", "share", "piper", "voices")
	entries, err := os.ReadDir(voiceDir)
	if err != nil {
		return nil, fmt.Errorf("no piper voices found in %s: %w", voiceDir, err)
	}

	var voices []Voice
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".onnx") {
			name := strings.TrimSuffix(entry.Name(), ".onnx")
			voices = append(voices, Voice{
				ID:          filepath.Join(voiceDir, entry.Name()),
				Name:        name,
				Language:    extractPiperLang(name),
				Description: "Local Piper Neural Voice",
			})
		}
	}
	return voices, nil
}

func findPiperBinary() string {
	if path, err := exec.LookPath("piper"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "Library", "Python", "3.9", "bin", "piper"),
		filepath.Join(home, "Library", "Python", "3.10", "bin", "piper"),
		filepath.Join(home, "Library", "Python", "3.11", "bin", "piper"),
		filepath.Join(home, "Library", "Python", "3.12", "bin", "piper"),
		filepath.Join(home, ".local", "bin", "piper"),
		"/usr/local/bin/piper",
		"/opt/homebrew/bin/piper",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func findDefaultPiperModel() string {
	home, _ := os.UserHomeDir()
	voiceDir := filepath.Join(home, ".local", "share", "piper", "voices")
	entries, err := os.ReadDir(voiceDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".onnx") {
			return filepath.Join(voiceDir, e.Name())
		}
	}
	return ""
}

func extractPiperLang(modelName string) string {
	parts := strings.Split(modelName, "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

func copyFileDirect(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
