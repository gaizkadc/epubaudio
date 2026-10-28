package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"epubaudio/internal/audio"
	"epubaudio/internal/chunker"
	"epubaudio/internal/converter"
	"epubaudio/internal/epub"
	"epubaudio/internal/model"
	"epubaudio/internal/tts"
)

type voiceListFlag struct {
	set    bool
	filter string
}

func (v *voiceListFlag) String() string {
	return v.filter
}

func (v *voiceListFlag) Set(val string) error {
	v.set = true
	if val != "true" && val != "false" {
		v.filter = val
	}
	return nil
}

func (v *voiceListFlag) IsBoolFlag() bool {
	return true
}

var (
	flagSynth      = flag.String("synth", "edge", "TTS engine: 'edge' (Edge-TTS), 'macos' (Apple say), 'piper' (Piper offline), or 'kokoro'")
	flagVoice      = flag.String("voice", "", "TTS voice (e.g. 'es-ES-AlvaroNeural', 'Mónica (Spanish (Spain))', 'es_ES-davefx-medium')")
	flagRate       = flag.String("rate", "+0%", "Speech rate adjustment (e.g. '+10%', '+20%', '-10%')")
	flagFormat     = flag.String("format", "", "Output format: 'm4b', 'mp3', or 'dir'")
	flagOutput     = flag.String("output", "", "Output file path or directory (shorthand -o)")
	flagChapters   = flag.String("chapters", "", "Chapters to convert (e.g. 'all', '0,1,2', '1-5')")
	flagList       = flag.Bool("list", false, "List detected chapters and exit")
	flagVoices     voiceListFlag
	flagWorkers    = flag.Int("workers", 2, "Number of concurrent TTS synthesis workers")
	flagCacheDir   = flag.String("cache-dir", "", "Custom cache directory for audio chunks (enables resuming)")
	flagKeepTemp   = flag.Bool("keep-temp", false, "Keep cached intermediate chunk files after completion")
	flagProxy      = flag.String("proxy", "", "HTTP proxy for TTS requests (Edge-TTS)")
	flagKokoroURL  = flag.String("kokoro-url", "", "Custom Kokoro API endpoint (default: http://localhost:8880/v1/audio/speech)")
	flagPiperModel = flag.String("piper-model", "", "Custom path to Piper .onnx model file")
)

func init() {
	flag.StringVar(flagOutput, "o", "", "Output file path or directory")
	flag.Var(&flagVoices, "list-voices", "List available voices for the chosen synthesizer (optional language filter: e.g. -list-voices es or -list-voices book.epub)")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: epubaudio [options] <book.epub>\n\n")
		fmt.Fprintf(os.Stderr, "A modular tool to convert EPUB ebooks into audiobooks (M4B, MP3, or chapter directory).\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nSupported Synthesizers (-synth):\n")
		fmt.Fprintf(os.Stderr, "  edge     Microsoft Edge Neural TTS (Free, high-quality, cloud, 400+ voices)\n")
		fmt.Fprintf(os.Stderr, "  macos    Native macOS 'say' engine (Offline, zero dependencies, uses Mac voices)\n")
		fmt.Fprintf(os.Stderr, "  piper    Piper Neural TTS (Offline, high-speed open-source engine)\n")
		fmt.Fprintf(os.Stderr, "  kokoro   Kokoro 82M TTS (Offline local ONNX inference or local HTTP server)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  epubaudio book.epub                                     # Interactive mode\n")
		fmt.Fprintf(os.Stderr, "  epubaudio -synth macos -chapters 0-2 book.epub          # Use native Mac voices\n")
		fmt.Fprintf(os.Stderr, "  epubaudio -synth piper -chapters all book.epub          # Use Piper offline\n")
		fmt.Fprintf(os.Stderr, "  epubaudio -synth edge -voice es-ES-ElviraNeural book.epub\n")
		fmt.Fprintf(os.Stderr, "  epubaudio -list-voices es                               # List Spanish voices\n")
		fmt.Fprintf(os.Stderr, "  epubaudio -list-voices book.epub                        # Auto-detect language & list voices\n")
	}

	flag.Parse()

	// 1. Initialize synthesizer engine
	ttsEngine, err := selectSynthesizer(*flagSynth, *flagProxy, *flagKokoroURL, *flagPiperModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Synthesizer error: %v\n", err)
		os.Exit(1)
	}

	// 2. Voice listing command
	if flagVoices.set || containsFlag("-list-voices") {
		filterLang := flagVoices.filter

		// If no filter was given, check positional arguments
		if filterLang == "" && len(flag.Args()) > 0 {
			target := flag.Args()[0]
			if strings.HasSuffix(strings.ToLower(target), ".epub") {
				// User passed an EPUB file: detect its language!
				if b, err := epub.Open(target); err == nil && b.Language != "" {
					fmt.Printf("📖 Book: \"%s\" (Detected language: %s)\n", defaultStr(b.Title, target), b.Language)
					filterLang = b.Language
				}
			} else {
				// User passed language code as argument, e.g. -list-voices es
				filterLang = target
			}
		}

		listVoices(ttsEngine, filterLang)
		return
	}

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	epubPath := args[0]

	// 3. Open and parse EPUB
	fmt.Printf("📖 Reading EPUB: %s\n", epubPath)
	book, err := epub.Open(epubPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error parsing EPUB: %v\n", err)
		os.Exit(1)
	}

	// 4. Resolve voice
	voice, isDefaultVoice := resolveVoice(ttsEngine, *flagVoice, *flagSynth, book.Language)

	fmt.Printf("📚 Title:       %s\n", defaultStr(book.Title, "Unknown Title"))
	if book.Author != "" {
		fmt.Printf("✍️  Author:      %s\n", book.Author)
	}
	if book.Language != "" {
		fmt.Printf("🌐 Language:    %s\n", book.Language)
	}
	fmt.Printf("🔊 Synthesizer: %s\n", ttsEngine.Name())
	if isDefaultVoice {
		if book.Language != "" {
			fmt.Printf("🎙️  Voice:       %s (default for '%s')\n", voice, book.Language)
		} else {
			fmt.Printf("🎙️  Voice:       %s (default)\n", voice)
		}
	} else {
		fmt.Printf("🎙️  Voice:       %s\n", voice)
	}
	fmt.Printf("📑 Chapters:    %d detected (total ~%d characters)\n\n", len(book.Chapters), book.TotalCharacters())

	// 5. Just list chapters if requested
	if *flagList {
		printChapters(book.Chapters)
		return
	}

	// 6. Select chapters
	var selectedIndices []int
	if *flagChapters != "" {
		indices, err := parseChapterSelection(*flagChapters, len(book.Chapters))
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid chapter selection: %v\n", err)
			os.Exit(1)
		}
		selectedIndices = indices
	} else {
		// Interactive chapter selection
		printChapters(book.Chapters)
		fmt.Printf("\nEnter chapters to convert ('all', '0,1,2', or '1-5') [default: all]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			input = "all"
		}
		indices, err := parseChapterSelection(input, len(book.Chapters))
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid selection: %v\n", err)
			os.Exit(1)
		}
		selectedIndices = indices
	}

	if len(selectedIndices) == 0 {
		fmt.Println("No chapters selected. Exiting.")
		return
	}

	// 7. Setup packager
	packager := audio.NewPackager()

	var outputFormat converter.OutputFormat
	switch strings.ToLower(*flagFormat) {
	case "m4b":
		outputFormat = converter.FormatM4B
	case "dir", "directory":
		outputFormat = converter.FormatDirectory
	case "mp3":
		outputFormat = converter.FormatMP3Single
	default:
		if packager.HasFFmpeg() {
			outputFormat = converter.FormatM4B
		} else {
			outputFormat = converter.FormatMP3Single
		}
	}

	if outputFormat == converter.FormatM4B && !packager.HasFFmpeg() {
		fmt.Println("⚠️  ffmpeg not found on PATH. Falling back to MP3 output.")
		fmt.Println("   (Tip: install ffmpeg via 'brew install ffmpeg' to generate M4B audiobooks with chapters)")
		outputFormat = converter.FormatMP3Single
	}

	// Context for graceful cancellation on Ctrl+C
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	conv := converter.New(converter.Config{
		Synthesizer: ttsEngine,
		Chunker:     chunker.New(chunker.DefaultMaxChunkSize),
		Packager:    packager,
		TTSOptions: tts.Options{
			Voice: voice,
			Rate:  *flagRate,
		},
		CacheDir:   *flagCacheDir,
		Workers:    *flagWorkers,
		KeepTemp:   *flagKeepTemp,
		Format:     outputFormat,
		OutputPath: *flagOutput,
		OnProgress: func(ev converter.ProgressEvent) {
			fi, err := os.Stdout.Stat()
			isTerminal := err == nil && (fi.Mode()&os.ModeCharDevice) != 0

			switch ev.Stage {
			case "start":
				fmt.Printf("🚀 %s\n", ev.Message)
			case "chapter_start":
				if ev.TotalChunks > 1 {
					fmt.Printf("\n🎙️  [%d/%d] Synthesizing: %q (%d chunks)...\n", ev.DoneChapters+1, ev.TotalChapters, ev.ChapterTitle, ev.TotalChunks)
				} else {
					fmt.Printf("\n🎙️  [%d/%d] Synthesizing: %q...\n", ev.DoneChapters+1, ev.TotalChapters, ev.ChapterTitle)
				}
			case "chunk_done":
				percent := 0
				if ev.TotalChunks > 0 {
					percent = (ev.CurrentChunk * 100) / ev.TotalChunks
				}
				if isTerminal {
					barWidth := 20
					filled := (percent * barWidth) / 100
					bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
					fmt.Printf("\r   ↳ Progress: [%s] %3d%% (%d/%d chunks)", bar, percent, ev.CurrentChunk, ev.TotalChunks)
					if ev.CurrentChunk >= ev.TotalChunks {
						fmt.Println()
					}
				} else {
					fmt.Printf("   ↳ Chunk %d/%d completed (%d%%)\n", ev.CurrentChunk, ev.TotalChunks, percent)
				}
			case "chapter_done":
				fmt.Printf("   ✅ Chapter [%d/%d] finished\n", ev.DoneChapters, ev.TotalChapters)
			case "packaging":
				fmt.Printf("\n📦 %s\n", ev.Message)
			case "done":
				fmt.Printf("\n🎉 %s\n", ev.Message)
			}
		},
	})

	outputFile, err := conv.Convert(ctx, book, selectedIndices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Conversion error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✨ Audio generated successfully: %s\n", outputFile)
}

func selectSynthesizer(name, proxy, kokoroURL, piperModel string) (tts.Synthesizer, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "edge", "edge-tts":
		return tts.NewEdgeTTS(proxy), nil
	case "macos", "say", "macos-say":
		return tts.NewMacOSSay(), nil
	case "piper":
		return tts.NewPiper(piperModel), nil
	case "kokoro":
		return tts.NewKokoro(kokoroURL), nil
	default:
		return nil, fmt.Errorf("unknown synthesizer engine: %q (supported: 'edge', 'macos', 'piper', 'kokoro')", name)
	}
}

func printChapters(chapters []model.Chapter) {
	fmt.Println("--- Detected Chapters ---")
	for _, ch := range chapters {
		fmt.Printf("[%2d] %-40s (%6d chars, ~%5d words)\n", ch.Index, truncate(ch.Title, 40), ch.CharCount, ch.WordCount)
	}
}

func parseChapterSelection(input string, total int) ([]int, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "all" || input == "*" {
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

func resolveVoice(engine tts.Synthesizer, requestedVoice, synthName, bookLang string) (string, bool) {
	if strings.TrimSpace(requestedVoice) != "" {
		return requestedVoice, false
	}
	if p, ok := engine.(*tts.Piper); ok && p.ModelPath != "" {
		base := filepath.Base(p.ModelPath)
		return strings.TrimSuffix(base, ".onnx"), true
	}
	v := pickDefaultVoice(synthName, bookLang)
	if v == "" {
		v = "default"
	}
	return v, true
}

func pickDefaultVoice(synthName, lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch strings.ToLower(synthName) {
	case "macos", "say", "macos-say":
		if strings.HasPrefix(lang, "es") {
			return "Mónica (Spanish (Spain))"
		}
		return "Samantha (English (US))"
	case "piper":
		return "" // Uses piper auto-detected default model
	case "kokoro":
		if strings.HasPrefix(lang, "es") {
			return "ef_dora"
		}
		return "af_heart"
	default: // edge-tts
		switch {
		case strings.HasPrefix(lang, "es"):
			return "es-ES-AlvaroNeural"
		case strings.HasPrefix(lang, "en"):
			return "en-US-ChristopherNeural"
		case strings.HasPrefix(lang, "fr"):
			return "fr-FR-HenriNeural"
		case strings.HasPrefix(lang, "de"):
			return "de-DE-ConradNeural"
		case strings.HasPrefix(lang, "it"):
			return "it-IT-DiegoNeural"
		case strings.HasPrefix(lang, "pt"):
			return "pt-BR-AntonioNeural"
		default:
			return "es-ES-AlvaroNeural"
		}
	}
}

func listVoices(engine tts.Synthesizer, filterLang string) {
	fmt.Printf("🔍 Fetching voices for %s...\n", engine.Name())
	vp, ok := engine.(tts.VoiceProvider)
	if !ok {
		fmt.Printf("Synthesizer %s does not support enumerating voices.\n", engine.Name())
		return
	}

	voices, err := vp.ListVoices(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching voices: %v\n", err)
		return
	}

	filter := strings.ToLower(strings.TrimSpace(filterLang))
	count := 0
	for _, v := range voices {
		vLang := strings.ToLower(v.Language)
		vID := strings.ToLower(v.ID)
		vName := strings.ToLower(v.Name)

		match := filter == ""
		if !match {
			if len(filter) == 2 {
				// Exact 2-letter language prefix match (e.g. 'es' matches 'es-ES', 'es_MX')
				match = strings.HasPrefix(vLang, filter+"-") ||
					strings.HasPrefix(vLang, filter+"_") ||
					vLang == filter ||
					strings.HasPrefix(vID, filter+"-")
			} else {
				match = strings.Contains(vLang, filter) ||
					strings.Contains(vID, filter) ||
					strings.Contains(vName, filter)
			}
		}
		if !match {
			continue
		}
		fmt.Printf("%-32s | %-6s | %-6s | %s\n", v.ID, v.Language, v.Gender, v.Name)
		count++
	}

	fmt.Printf("\nTotal voices listed: %d\n", count)
	if filter != "" {
		fmt.Printf("(Filtered by '%s')\n", filter)
	}
}

func containsFlag(flagName string) bool {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, flagName) {
			return true
		}
	}
	return false
}

func defaultStr(val, def string) string {
	if strings.TrimSpace(val) == "" {
		return def
	}
	return val
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
