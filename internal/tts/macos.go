package tts

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MacOSSay implements the Synthesizer and VoiceProvider interfaces using macOS's built-in `say` command.
type MacOSSay struct{}

// NewMacOSSay creates a new MacOSSay synthesizer.
func NewMacOSSay() *MacOSSay {
	return &MacOSSay{}
}

// Name returns the synthesizer identifier.
func (m *MacOSSay) Name() string {
	return "macos-say"
}

// Synthesize synthesizes speech from text using macOS `say`.
func (m *MacOSSay) Synthesize(ctx context.Context, text string, outPath string, opts Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	voice := opts.Voice
	if voice == "" {
		voice = "Mónica (Spanish (Spain))"
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	// Write text to a temp file to avoid ARG_MAX issues with large text blocks
	tmpTextFile := filepath.Join(os.TempDir(), fmt.Sprintf("say_text_%d.txt", os.Getpid()))
	if err := os.WriteFile(tmpTextFile, []byte(text), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpTextFile)

	// say can generate .m4a or .aiff natively
	ext := strings.ToLower(filepath.Ext(outPath))
	targetOut := outPath
	needsConversion := false
	var intermediateOut string

	if ext == ".mp3" {
		needsConversion = true
		intermediateOut = strings.TrimSuffix(outPath, ext) + "_raw.m4a"
		targetOut = intermediateOut
	}

	cmd := exec.CommandContext(ctx, "say", "-v", voice, "-o", targetOut, "-f", tmpTextFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("say command failed (%w): %s", err, string(out))
	}

	if needsConversion {
		defer os.Remove(intermediateOut)
		// Check if ffmpeg is available for mp3 conversion
		if _, err := exec.LookPath("ffmpeg"); err == nil {
			convCmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", intermediateOut, "-c:a", "libmp3lame", "-q:a", "2", outPath)
			if err := convCmd.Run(); err != nil {
				// Fallback to afconvert
				_ = exec.CommandContext(ctx, "afconvert", "-f", "m4af", "-d", "aac", intermediateOut, outPath).Run()
			}
		} else {
			// If no ffmpeg, keep it as m4a or rename
			if err := os.Rename(intermediateOut, outPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// ListVoices enumerates installed macOS voices using `say -v '?'`.
func (m *MacOSSay) ListVoices(ctx context.Context) ([]Voice, error) {
	cmd := exec.CommandContext(ctx, "say", "-v", "?")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run say -v '?': %w", err)
	}

	var voices []Voice
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "#", 2)
		left := strings.TrimSpace(parts[0])
		desc := ""
		if len(parts) > 1 {
			desc = strings.TrimSpace(parts[1])
		}

		// left looks like: "Mónica (Spanish (Spain)) es_ES" or "Paulina es_MX"
		lastSpace := strings.LastIndex(left, " ")
		if lastSpace == -1 {
			continue
		}
		name := strings.TrimSpace(left[:lastSpace])
		locale := strings.TrimSpace(left[lastSpace+1:])

		voices = append(voices, Voice{
			ID:          name,
			Name:        name,
			Language:    locale,
			Description: desc,
		})
	}

	return voices, nil
}
