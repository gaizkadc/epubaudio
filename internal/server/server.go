package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"epubaudio/internal/audio"
	"epubaudio/internal/chunker"
	"epubaudio/internal/converter"
	"epubaudio/internal/epub"
	"epubaudio/internal/tts"
	"epubaudio/web"
)

type Config struct {
	Port       string
	Proxy      string
	KokoroURL  string
	PiperModel string
	Workers    int
	CacheDir   string
}

type Server struct {
	cfg      Config
	packager *audio.Packager
	jobs     sync.Map // map[string]*Job
}

type Job struct {
	ID        string
	Events    chan WebProgressEvent
	Done      chan struct{}
	CreatedAt time.Time
}

type WebProgressEvent struct {
	converter.ProgressEvent
	OutputFile  string `json:"outputFile,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	Error       string `json:"error,omitempty"`
}

func New(cfg Config) *Server {
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	return &Server{
		cfg:      cfg,
		packager: audio.NewPackager(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/voices", s.handleVoices)
	mux.HandleFunc("POST /api/convert", s.handleConvert)
	mux.HandleFunc("GET /api/progress", s.handleProgress)
	mux.HandleFunc("GET /api/download", s.handleDownload)

	// Embedded static UI
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Web UI assets missing", http.StatusInternalServerError)
		})
	} else {
		fileServer := http.FileServer(http.FS(distFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				fileServer.ServeHTTP(w, r)
				return
			}

			// Check if file exists in embedded assets
			f, err := distFS.Open(path)
			if err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}

			// SPA Fallback: serve index.html
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	return mux
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	_ = os.MkdirAll("input", 0755)

	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		http.Error(w, "File too large or invalid multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read uploaded file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".epub") {
		http.Error(w, "File must be an .epub file", http.StatusBadRequest)
		return
	}

	savePath := filepath.Join("input", header.Filename)
	dst, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Failed to write file to disk", http.StatusInternalServerError)
		return
	}

	book, err := epub.Open(savePath)
	if err != nil {
		http.Error(w, "Failed to parse EPUB: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp := map[string]interface{}{
		"filename":        header.Filename,
		"title":           book.Title,
		"author":          book.Author,
		"language":        book.Language,
		"totalChapters":   len(book.Chapters),
		"totalCharacters": book.TotalCharacters(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleVoices(w http.ResponseWriter, r *http.Request) {
	synthName := r.URL.Query().Get("synth")
	if synthName == "" {
		synthName = "edge"
	}
	filterLang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))

	engine, err := s.getSynthesizer(synthName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	vp, ok := engine.(tts.VoiceProvider)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tts.Voice{})
		return
	}

	allVoices, err := vp.ListVoices(r.Context())
	if err != nil {
		http.Error(w, "Failed to list voices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var matched []tts.Voice
	baseLang := filterLang
	if idx := strings.IndexAny(filterLang, "-_"); idx != -1 {
		baseLang = filterLang[:idx]
	}

	for _, v := range allVoices {
		vLang := strings.ToLower(v.Language)
		vID := strings.ToLower(v.ID)
		vName := strings.ToLower(v.Name)

		match := filterLang == ""
		if !match {
			// First try exact or prefix match against dialect (e.g. "en-us")
			match = vLang == filterLang ||
				strings.HasPrefix(vLang, filterLang+"-") ||
				strings.HasPrefix(vLang, filterLang+"_") ||
				strings.HasPrefix(vID, filterLang+"-") ||
				strings.Contains(vLang, filterLang) ||
				strings.Contains(vID, filterLang)

			// If no match and base language exists (e.g. "en"), match against base language
			if !match && baseLang != "" {
				match = vLang == baseLang ||
					strings.HasPrefix(vLang, baseLang+"-") ||
					strings.HasPrefix(vLang, baseLang+"_") ||
					strings.HasPrefix(vID, baseLang+"-") ||
					strings.Contains(vName, "("+baseLang)
			}
		}

		if match {
			matched = append(matched, v)
		}
	}

	// Fallback: if no voices matched the language filter, return all voices
	// so the user is never left with an empty list
	if len(matched) == 0 && len(allVoices) > 0 {
		matched = allVoices
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(matched)
}

type convertRequest struct {
	Filename string `json:"filename"`
	Synth    string `json:"synth"`
	Voice    string `json:"voice"`
	Format   string `json:"format"`
	Chapters string `json:"chapters"`
}

func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	var req convertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	cleanFile := filepath.Base(req.Filename)
	filePath := filepath.Join("input", cleanFile)
	if _, err := os.Stat(filePath); err != nil {
		http.Error(w, "Book file not found: "+cleanFile, http.StatusNotFound)
		return
	}

	book, err := epub.Open(filePath)
	if err != nil {
		http.Error(w, "Failed to read EPUB: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Synth == "" {
		req.Synth = "edge"
	}
	engine, err := s.getSynthesizer(req.Synth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	voice := req.Voice
	if voice == "" {
		voice, _ = s.resolveDefaultVoice(engine, req.Synth, book.Language)
	}

	chapterIndices, err := s.parseChapters(req.Chapters, len(book.Chapters))
	if err != nil {
		http.Error(w, "Invalid chapter selection: "+err.Error(), http.StatusBadRequest)
		return
	}

	outputFormat := converter.FormatMP3Single
	if strings.ToLower(req.Format) == "m4b" {
		if s.packager.HasFFmpeg() {
			outputFormat = converter.FormatM4B
		} else {
			outputFormat = converter.FormatMP3Single
		}
	}

	jobID := generateID()
	job := &Job{
		ID:        jobID,
		Events:    make(chan WebProgressEvent, 100),
		Done:      make(chan struct{}),
		CreatedAt: time.Now(),
	}
	s.jobs.Store(jobID, job)

	// Launch background conversion
	go func() {
		defer close(job.Done)

		conv := converter.New(converter.Config{
			Synthesizer: engine,
			Chunker:     chunker.New(chunker.DefaultMaxChunkSize),
			Packager:    s.packager,
			TTSOptions: tts.Options{
				Voice: voice,
				Rate:  "+0%",
			},
			CacheDir: s.cfg.CacheDir,
			Workers:  s.cfg.Workers,
			Format:   outputFormat,
			OnProgress: func(ev converter.ProgressEvent) {
				if ev.Stage == "done" {
					// Suppress bare internal done event;
					// the enriched event with OutputFile and DownloadURL will be sent right after Convert() returns.
					return
				}
				select {
				case job.Events <- WebProgressEvent{ProgressEvent: ev}:
				default:
				}
			},
		})

		outPath, err := conv.Convert(context.Background(), book, chapterIndices)
		if err != nil {
			job.Events <- WebProgressEvent{
				ProgressEvent: converter.ProgressEvent{Stage: "error"},
				Error:         err.Error(),
			}
			return
		}

		baseName := filepath.Base(outPath)
		job.Events <- WebProgressEvent{
			ProgressEvent: converter.ProgressEvent{
				Stage:   "done",
				Message: fmt.Sprintf("Finished: %s", baseName),
			},
			OutputFile:  baseName,
			DownloadURL: "/api/download?file=" + url.QueryEscape(baseName),
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"jobId": jobID,
	})
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("jobId")
	val, ok := s.jobs.Load(jobID)
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	job := val.(*Job)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-job.Events:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			if ev.Stage == "done" || ev.Stage == "error" {
				return
			}
		}
	}
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.URL.Query().Get("file"))
	if filename == "" || filename == "." || filename == "/" {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join("output", filename)
	if _, err := os.Stat(fullPath); err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, fullPath)
}

func (s *Server) getSynthesizer(name string) (tts.Synthesizer, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "edge", "edge-tts":
		return tts.NewEdgeTTS(s.cfg.Proxy), nil
	case "macos", "say", "macos-say":
		return tts.NewMacOSSay(), nil
	case "piper":
		return tts.NewPiper(s.cfg.PiperModel), nil
	case "kokoro":
		return tts.NewKokoro(s.cfg.KokoroURL), nil
	default:
		return nil, fmt.Errorf("unknown synthesizer: %s", name)
	}
}

func (s *Server) resolveDefaultVoice(engine tts.Synthesizer, synthName, bookLang string) (string, bool) {
	lang := strings.ToLower(strings.TrimSpace(bookLang))
	switch strings.ToLower(synthName) {
	case "macos", "say", "macos-say":
		if strings.HasPrefix(lang, "es") {
			return "Mónica (Spanish (Spain))", true
		}
		return "Samantha (English (US))", true
	case "piper":
		if p, ok := engine.(*tts.Piper); ok && p.ModelPath != "" {
			return strings.TrimSuffix(filepath.Base(p.ModelPath), ".onnx"), true
		}
		return "default", true
	case "kokoro":
		if strings.HasPrefix(lang, "es") {
			return "ef_dora", true
		}
		return "af_heart", true
	default: // edge-tts
		switch {
		case strings.HasPrefix(lang, "es"):
			return "es-ES-AlvaroNeural", true
		case strings.HasPrefix(lang, "en"):
			return "en-US-ChristopherNeural", true
		case strings.HasPrefix(lang, "fr"):
			return "fr-FR-HenriNeural", true
		case strings.HasPrefix(lang, "de"):
			return "de-DE-ConradNeural", true
		case strings.HasPrefix(lang, "it"):
			return "it-IT-DiegoNeural", true
		case strings.HasPrefix(lang, "pt"):
			return "pt-BR-AntonioNeural", true
		default:
			return "es-ES-AlvaroNeural", true
		}
	}
}

func (s *Server) parseChapters(input string, total int) ([]int, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" || input == "all" || input == "*" {
		all := make([]int, total)
		for i := 0; i < total; i++ {
			all[i] = i
		}
		return all, nil
	}

	seen := make(map[int]bool)
	parts := strings.Split(input, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if strings.Contains(p, "-") {
			rangeParts := strings.SplitN(p, "-", 2)
			start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range: %s", p)
			}
			if start > end {
				start, end = end, start
			}
			for i := start; i <= end; i++ {
				if i >= 0 && i < total {
					seen[i] = true
				}
			}
		} else {
			idx, err := strconv.Atoi(p)
			if err != nil {
				return nil, fmt.Errorf("invalid chapter index: %s", p)
			}
			if idx < 0 || idx >= total {
				return nil, fmt.Errorf("chapter index out of range [0..%d]: %d", total-1, idx)
			}
			seen[idx] = true
		}
	}

	result := make([]int, 0, len(seen))
	for idx := range seen {
		result = append(result, idx)
	}
	sort.Ints(result)
	return result, nil
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

