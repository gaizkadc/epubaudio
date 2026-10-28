package tts

import (
	"context"
)

// Options holds speech synthesis parameters.
type Options struct {
	Voice  string // Engine-specific voice identifier (e.g., "es-ES-AlvaroNeural")
	Rate   string // Speech rate adjustment (e.g. "+0%", "+15%", "-10%")
	Volume string // Volume adjustment (e.g. "+0%", "-20%")
	Pitch  string // Pitch adjustment (e.g. "+0Hz", "+5Hz")
}

// Voice represents an available voice on the TTS engine.
type Voice struct {
	ID          string // Full voice identifier
	Name        string // Friendly name
	Language    string // Locale code, e.g. "es-ES", "en-US"
	Gender      string // Gender: "Male", "Female", or empty
	Description string // Optional description or status
}

// Synthesizer is the pluggable interface implemented by any TTS backend (Edge-TTS, OpenAI, Kokoro, etc.)
type Synthesizer interface {
	Name() string
	Synthesize(ctx context.Context, text string, outPath string, opts Options) error
}

// VoiceProvider is an optional interface for synthesizers that can enumerate available voices.
type VoiceProvider interface {
	ListVoices(ctx context.Context) ([]Voice, error)
}
