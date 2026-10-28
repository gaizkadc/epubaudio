package audio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeBaseName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Chapter 1: The Beginning", "Chapter 1 - The Beginning"},
		{"What? / When? * Yes", "What - When Yes"},
		{"   leading and trailing spaces   ", "leading and trailing spaces"},
	}

	for _, tt := range tests {
		got := SanitizeBaseName(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeBaseName(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConcatFiles(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "part1.mp3")
	f2 := filepath.Join(tmpDir, "part2.mp3")
	out := filepath.Join(tmpDir, "combined.mp3")

	if err := os.WriteFile(f1, []byte("PART1_AUDIO_DATA_"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("PART2_AUDIO_DATA"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewPackager()
	if err := p.ConcatFiles([]string{f1, f2}, out); err != nil {
		t.Fatalf("ConcatFiles failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	expected := "PART1_AUDIO_DATA_PART2_AUDIO_DATA"
	if string(data) != expected {
		t.Fatalf("expected concatenated data %q, got %q", expected, string(data))
	}
}

