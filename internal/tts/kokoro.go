package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Kokoro implements the Synthesizer and VoiceProvider interfaces for the Kokoro TTS engine.
// It supports both local embedded ONNX execution and an OpenAI-compatible HTTP server endpoint.
type Kokoro struct {
	Endpoint   string
	PythonPath string
	ModelPath  string
	VoicesPath string
}

// NewKokoro creates a Kokoro synthesizer.
func NewKokoro(endpoint string) *Kokoro {
	if endpoint == "" {
		endpoint = os.Getenv("KOKORO_ENDPOINT")
	}
	if endpoint == "" {
		endpoint = "http://localhost:8880/v1/audio/speech"
	}

	home, _ := os.UserHomeDir()
	modelPath := filepath.Join(home, ".local", "share", "kokoro", "kokoro-v1.0.onnx")
	voicesPath := filepath.Join(home, ".local", "share", "kokoro", "voices-v1.0.bin")

	pyPath := findKokoroPython()

	return &Kokoro{
		Endpoint:   endpoint,
		PythonPath: pyPath,
		ModelPath:  modelPath,
		VoicesPath: voicesPath,
	}
}

// Name returns the synthesizer identifier.
func (k *Kokoro) Name() string {
	return "kokoro"
}

// Synthesize synthesizes speech using local Kokoro ONNX model, or falls back to HTTP endpoint.
func (k *Kokoro) Synthesize(ctx context.Context, text string, outPath string, opts Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	voice := opts.Voice
	if voice == "" {
		voice = "ef_dora" // default Spanish Kokoro voice
	}

	// 1. If local ONNX model and python runtime are available, run locally!
	if k.hasLocalModel() && k.PythonPath != "" {
		return k.synthesizeLocal(ctx, text, outPath, voice, opts)
	}

	// 2. Fallback to HTTP endpoint
	return k.synthesizeHTTP(ctx, text, outPath, voice)
}

func (k *Kokoro) hasLocalModel() bool {
	if _, err := os.Stat(k.ModelPath); err != nil {
		return false
	}
	if _, err := os.Stat(k.VoicesPath); err != nil {
		return false
	}
	return true
}

func (k *Kokoro) synthesizeLocal(ctx context.Context, text, outPath, voice string, opts Options) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	tmpText := filepath.Join(os.TempDir(), fmt.Sprintf("kokoro_in_%d.txt", os.Getpid()))
	if err := os.WriteFile(tmpText, []byte(text), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpText)

	tmpWav := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "_kokoro_raw.wav"
	defer os.Remove(tmpWav)

	lang := "es"
	if strings.HasPrefix(voice, "a") {
		lang = "en-us"
	} else if strings.HasPrefix(voice, "b") {
		lang = "en-gb"
	} else if strings.HasPrefix(voice, "f") {
		lang = "fr-fr"
	}

	script := `
import sys, os, soundfile as sf
from kokoro_onnx import Kokoro

text_file, out_file, voice, lang = sys.argv[1:5]
with open(text_file, 'r', encoding='utf-8') as f:
    text = f.read()

home = os.path.expanduser('~')
model_path = os.path.join(home, '.local/share/kokoro/kokoro-v1.0.onnx')
voices_path = os.path.join(home, '.local/share/kokoro/voices-v1.0.bin')

kokoro = Kokoro(model_path, voices_path)
samples, sr = kokoro.create(text, voice=voice, speed=1.0, lang=lang)
sf.write(out_file, samples, sr)
`

	cmd := exec.CommandContext(ctx, k.PythonPath, "-c", script, tmpText, tmpWav, voice, lang)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("local kokoro failed (%w): %s", err, string(out))
	}

	// Convert WAV to target format (e.g. MP3)
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

	// Fallback to afconvert
	afCmd := exec.CommandContext(ctx, "afconvert", "-f", "m4af", "-d", "aac", tmpWav, outPath)
	if err := afCmd.Run(); err == nil {
		return nil
	}

	return copyFileDirect(tmpWav, outPath)
}

func (k *Kokoro) synthesizeHTTP(ctx context.Context, text, outPath, voice string) error {
	payload := map[string]interface{}{
		"model":           "kokoro",
		"input":           text,
		"voice":           voice,
		"response_format": "mp3",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", k.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Kokoro server at %s: %w\n(Hint: start Kokoro with 'docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu')", k.Endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kokoro synthesis failed (%s): %s", resp.Status, string(respBody))
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	return err
}

// ListVoices returns standard available Kokoro voice presets.
func (k *Kokoro) ListVoices(ctx context.Context) ([]Voice, error) {
	return []Voice{
		{ID: "ef_dora", Name: "Dora (Female, Spanish)", Language: "es-ES", Gender: "Female", Description: "Spanish female natural voice"},
		{ID: "em_alex", Name: "Alex (Male, Spanish)", Language: "es-ES", Gender: "Male", Description: "Spanish male natural voice"},
		{ID: "af_heart", Name: "Heart (Female, US English)", Language: "en-US", Gender: "Female", Description: "Highly realistic natural voice"},
		{ID: "af_bella", Name: "Bella (Female, US English)", Language: "en-US", Gender: "Female", Description: "Expressive female voice"},
		{ID: "af_sarah", Name: "Sarah (Female, US English)", Language: "en-US", Gender: "Female", Description: "Warm female narrator"},
		{ID: "am_adam", Name: "Adam (Male, US English)", Language: "en-US", Gender: "Male", Description: "Deep male narrator"},
		{ID: "am_michael", Name: "Michael (Male, US English)", Language: "en-US", Gender: "Male", Description: "Clear articulate male voice"},
		{ID: "bf_emma", Name: "Emma (Female, British)", Language: "en-GB", Gender: "Female", Description: "British English female"},
		{ID: "bm_george", Name: "George (Male, British)", Language: "en-GB", Gender: "Male", Description: "British English male"},
		{ID: "ff_siwis", Name: "Siwis (Female, French)", Language: "fr-FR", Gender: "Female", Description: "French female voice"},
	}, nil
}

func findKokoroPython() string {
	candidates := []string{
		"/opt/local/bin/python3",
		"/opt/local/bin/python3.12",
		"/usr/local/bin/python3",
		"/usr/bin/python3",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			// Quick test if kokoro_onnx is importable
			checkCmd := exec.Command(c, "-c", "import kokoro_onnx")
			if err := checkCmd.Run(); err == nil {
				return c
			}
		}
	}
	return ""
}
