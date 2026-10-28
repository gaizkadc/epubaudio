package main

import (
	"reflect"
	"testing"
)

func TestParseChapterSelection(t *testing.T) {
	tests := []struct {
		input    string
		total    int
		expected []int
		wantErr  bool
	}{
		{"all", 3, []int{0, 1, 2}, false},
		{"*", 2, []int{0, 1}, false},
		{"0", 5, []int{0}, false},
		{"0,2,4", 5, []int{0, 2, 4}, false},
		{"1-3", 5, []int{1, 2, 3}, false},
		{"0, 2-4", 6, []int{0, 2, 3, 4}, false},
		{"3-1", 5, []int{1, 2, 3}, false}, // reversed range
		{"10", 5, nil, true},              // out of bounds
		{"abc", 5, nil, true},             // non-numeric
	}

	for _, tt := range tests {
		got, err := parseChapterSelection(tt.input, tt.total)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseChapterSelection(%q, %d) error = %v, wantErr %v", tt.input, tt.total, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("parseChapterSelection(%q, %d) = %v, expected %v", tt.input, tt.total, got, tt.expected)
		}
	}
}

