package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticUI(t *testing.T) {
	srv := New(Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "root") {
		t.Errorf("expected index.html to contain 'root' element, got: %s", string(body))
	}
}

func TestUploadInvalidFormat(t *testing.T) {
	srv := New(Config{})
	handler := srv.Handler()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.docx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("not a supported format"))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for unsupported file, got %d", w.Result().StatusCode)
	}
}

func TestUploadValidTXT(t *testing.T) {
	t.Cleanup(func() { _ = os.RemoveAll("input") })
	srv := New(Config{})
	handler := srv.Handler()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sample_document.txt")
	if err != nil {
		t.Fatal(err)
	}
	txtContent := "Chapter 1: The New Horizon\nThis is a sample document for testing the upload endpoint."
	_, _ = part.Write([]byte(txtContent))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for txt file, got %d", w.Result().StatusCode)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if resp["filename"] != "sample_document.txt" {
		t.Errorf("expected filename sample_document.txt, got %v", resp["filename"])
	}
	if resp["totalChapters"].(float64) < 1 {
		t.Errorf("expected at least 1 chapter, got %v", resp["totalChapters"])
	}
}

func TestDownloadNotFound(t *testing.T) {
	srv := New(Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/download?file=does_not_exist_xyz.mp3", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing download, got %d", w.Result().StatusCode)
	}
}

func TestConvertMissingFile(t *testing.T) {
	srv := New(Config{})
	handler := srv.Handler()

	payload := `{"filename": "missing.epub", "synth": "edge"}`
	req := httptest.NewRequest("POST", "/api/convert", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing book file, got %d", w.Result().StatusCode)
	}
}

func TestVoicesEndpoint(t *testing.T) {
	srv := New(Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/voices?synth=macos&lang=es", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Mónica") {
		t.Errorf("expected voices to contain Mónica for macOS Spanish, got: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "Bubbles") || strings.Contains(bodyStr, "Jester") {
		t.Errorf("expected false-positive English voices Bubbles/Jester to be filtered out, got: %s", bodyStr)
	}
}

func TestDownloadHeaders(t *testing.T) {
	_ = os.MkdirAll("output", 0755)
	dummyFile := filepath.Join("output", "test_audio.m4b")
	_ = os.WriteFile(dummyFile, []byte("dummy m4b content"), 0644)
	t.Cleanup(func() { _ = os.Remove(dummyFile) })

	srv := New(Config{})
	handler := srv.Handler()

	// 1. Streaming request (audio player)
	req := httptest.NewRequest("GET", "/api/download?file=test_audio.m4b", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "inline") {
		t.Errorf("expected inline Content-Disposition for player, got %s", resp.Header.Get("Content-Disposition"))
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "audio/mp4") {
		t.Errorf("expected audio/mp4 Content-Type, got %s", resp.Header.Get("Content-Type"))
	}

	// 2. Download request
	reqDl := httptest.NewRequest("GET", "/api/download?file=test_audio.m4b&download=true", nil)
	wDl := httptest.NewRecorder()
	handler.ServeHTTP(wDl, reqDl)

	respDl := wDl.Result()
	if !strings.Contains(respDl.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("expected attachment Content-Disposition, got %s", respDl.Header.Get("Content-Disposition"))
	}
}

