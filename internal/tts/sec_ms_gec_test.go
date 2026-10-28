package tts

import (
	"strings"
	"testing"
)

func TestGenerateSecMSGecV2(t *testing.T) {
	tok := GenerateSecMSGecV2()
	parts := strings.Split(tok, "-")
	if len(parts) != 2 {
		t.Fatalf("expected token format <16-hex>-<64-hex>, got %s", tok)
	}
	if len(parts[0]) != 16 {
		t.Errorf("expected 16-char cs64, got %d chars: %s", len(parts[0]), parts[0])
	}
	if len(parts[1]) != 64 {
		t.Errorf("expected 64-char sha256, got %d chars: %s", len(parts[1]), parts[1])
	}
}

