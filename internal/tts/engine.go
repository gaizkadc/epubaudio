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
	ID          string `json:"id"`          // Full voice identifier
	Name        string `json:"name"`        // Friendly name
	Language    string `json:"language"`    // Locale code, e.g. "es-ES", "en-US"
	Gender      string `json:"gender"`      // Gender: "Male", "Female", or empty
	Description string `json:"description,omitempty"` // Optional description or status
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
