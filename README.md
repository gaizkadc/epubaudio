# epubaudio 🎧📖

A fast, modular command-line tool written in Go that converts EPUB eBooks into audiobooks (`.m4b` with chapter markers, single `.mp3`, or a directory of chapter files).

Powered by free, high-quality neural synthesizers (Edge-TTS, Kokoro, Piper, and macOS `say`) with a pluggable architecture.

> [!NOTE]
> This project is a **work in progress** (WIP) created and actively developed alongside **Antigravity** (Google DeepMind). Features, optimizations, and synthesis engines are continuously evolving.

---

## ✨ Features

- **Standard-Compliant EPUB Parsing**: Respects the reading order defined in the EPUB `<spine>` (`container.xml` $\rightarrow$ `.opf`), avoiding out-of-order chapters, indices, or footnotes.
- **Table of Contents (TOC) Extraction**: Reads chapter titles from `toc.ncx` / `nav.xhtml` or document headings (`<h1>`, `<h2>`).
- **Clean Text Normalization**: Automatically strips scripts, styles, and markup while unescaping HTML entities and preserving natural sentence and paragraph pauses.
- **Smart Text Chunking**: Automatically splits long chapters into manageable chunks along sentence and paragraph boundaries, avoiding synthesis timeouts or character limit errors.
- **Resumable Cache**: Saves audio chunks into a cache directory. If synthesis is interrupted or cancelled, restarting resumes where it left off without re-synthesizing completed parts.
- **Multiple Output Formats**:
  - **`.m4b`**: AAC audiobook file with real chapter navigation markers (requires `ffmpeg`).
  - **`.mp3`**: Single consolidated MP3 file.
  - **`dir`**: Directory containing numbered MP3 files per chapter (`01 - Chapter Title.mp3`).
- **Concurrent Workers**: Converts chapters concurrently with a configurable worker pool.
- **Interactive & Headless**: Offers an interactive chapter selector by default, or runs completely headless via CLI flags.

---

## 📋 Prerequisites

1. **Go 1.22+**: [Install Go](https://go.dev/dl/)
2. **FFmpeg** *(Optional, recommended)*: Required if you want to export `.m4b` audiobooks with embedded chapter markers.
   - **macOS**: `brew install ffmpeg`
   - **Ubuntu/Debian**: `sudo apt install ffmpeg`
   - **Windows**: `winget install Gyan.FFmpeg` or `choco install ffmpeg`

---

## 🚀 Building & Installation

Clone or download the project and build using `make` or standard `go build`:

```bash
git clone https://github.com/gaizkadc/epubaudio.git
cd epubaudio

# Using Make:
make build

# Or using Go directly:
go build -o epubaudio ./cmd/epubaudio
```

You can optionally install it into your `GOBIN` (`~/go/bin`):

```bash
make install
# or: go install ./cmd/epubaudio
```

### Handy Makefile Commands

| Command | Description |
|---|---|
| `make help` | Display all available targets |
| `make build` | Compile the `epubaudio` binary |
| `make test` | Run all unit tests |
| `make test-race` | Run unit tests with Go's race detector |
| `make coverage` | Generate and show test coverage report |
| `make lint` | Run static checks (`go vet` & `golangci-lint`) |
| `make fmt` | Format source code with `gofmt` |
| `make clean` | Clean built binaries and temporary audio artifacts |

---

## 💡 Quick Start & Usage

### 1. Interactive Mode
Run with just the path to your EPUB:
```bash
./epubaudio my_book.epub
```
The tool will:
1. Scan and display book metadata (title, author, detected language).
2. List all detected chapters with character counts.
3. Prompt you to choose which chapters to convert (e.g. `all`, `0,1,2`, or `1-5`).
4. Synthesize the audio and package the final audiobook.

---

### 2. Command-Line Options

| Flag | Shorthand | Default | Description |
|---|---|---|---|
| `-synth` | | `edge` | Synthesizer engine: `edge`, `macos`, `piper`, or `kokoro` |
| `-voice` | | *Auto from language* | TTS voice identifier (e.g. `es-ES-AlvaroNeural`, `Mónica (Spanish (Spain))`) |
| `-rate` | | `+0%` | Speech rate adjustment (e.g. `+10%`, `+20%`, `-10%`) |
| `-format` | | `m4b` (or `mp3`) | Output format: `m4b`, `mp3`, or `dir` |
| `-output` | `-o` | `output/<Title> [<synth>].<ext>` | Output file path or directory (routes to `output/` by default) |
| `-chapters`| `-c` | *Interactive* | Chapters to convert (`all`, `0,1,2`, or `1-5`) |
| `-list` | | `false` | List detected chapters and character counts, then exit |
| `-list-voices` | | | List available TTS voices for the chosen `-synth` (optional filter: `-list-voices es`) |
| `-workers` | | `2` | Number of concurrent synthesis workers |
| `-cache-dir` | | *System temp* | Custom directory for temporary audio files (enables resuming) |
| `-keep-temp` | | `false` | Keep intermediate chunk files after completion |
| `-piper-model`| | *Auto-detected* | Custom path to Piper `.onnx` model |
| `-kokoro-url` | | `http://localhost:8880/v1/audio/speech` | Custom Kokoro API endpoint |
| `-proxy` | | | HTTP proxy URL for Edge-TTS requests |

---

### 3. Usage Examples

#### List Chapters Without Converting
```bash
./epubaudio -list my_book.epub
```

#### Convert All Chapters to M4B Audiobook (with chapters)
```bash
./epubaudio -chapters all -format m4b -o "Dune.m4b" dune.epub
```

#### Convert Specific Chapters (e.g. Chapters 0 to 4)
```bash
./epubaudio -chapters 0-4 dune.epub
```

#### Change Voice and Speed
```bash
./epubaudio -voice en-US-ChristopherNeural -rate "+15%" -chapters all book.epub
```

#### Export as a Folder of Individual MP3 Files
```bash
./epubaudio -format dir -o ./my_audiobook book.epub
```

#### Using Different Synthesizers (Offline & Cloud)
```bash
# 1. macOS Native Offline (Uses Mac's built-in voices)
./epubaudio -synth macos -voice "Mónica (Spanish (Spain))" book.epub

# 2. Piper Neural TTS (100% Offline, ultra-fast)
./epubaudio -synth piper book.epub

# 3. Kokoro TTS (Local OpenAI-compatible server)
./epubaudio -synth kokoro book.epub

# 4. Microsoft Edge-TTS (Default cloud engine, 400+ voices)
./epubaudio -synth edge -voice es-ES-AlvaroNeural book.epub
```

#### Discover Available Voices per Engine
```bash
# List macOS installed voices
./epubaudio -synth macos -list-voices es

# List Piper local models
./epubaudio -synth piper -list-voices

# List Edge-TTS cloud voices
./epubaudio -synth edge -list-voices es
```

---

## 🏗️ Architecture & Modularity

The project is structured with modularity and separation of concerns:

```
epubaudio/
├── cmd/
│   └── epubaudio/
│       ├── main.go             # CLI interface, interactive prompts & flags
│       └── main_test.go        # Unit tests for CLI parsing
├── internal/
│   ├── model/                  # Data structures (Book, Chapter)
│   │   └── book.go
│   ├── epub/                   # EPUB archive reader, spine parser & HTML cleaner
│   │   ├── reader.go
│   │   ├── html.go
│   │   ├── reader_test.go
│   │   └── html_test.go
│   ├── chunker/                # Intelligent sentence/paragraph text chunker
│   │   ├── chunker.go
│   │   └── chunker_test.go
│   ├── tts/                    # Pluggable TTS backend abstraction
│   │   ├── engine.go           # Synthesizer and VoiceProvider interfaces
│   │   └── edge.go             # Microsoft Edge-TTS implementation
│   ├── audio/                  # Audio concat, duration calculation & packaging
│   │   ├── packager.go
│   │   └── packager_test.go
│   └── converter/              # Pipeline orchestrator (workers, cache, progress)
│       └── converter.go
├── go.mod
└── go.sum
```

### Adding a New TTS Provider
To add a new TTS engine (such as OpenAI, ElevenLabs, Kokoro, or local Piper), simply implement the `Synthesizer` interface defined in `internal/tts/engine.go`:

```go
type Synthesizer interface {
    Name() string
    Synthesize(ctx context.Context, text string, outPath string, opts Options) error
}
```

Then plug it into the `converter.New(converter.Config{ Synthesizer: myEngine, ... })`.

---

## 🧪 Running Tests

All modules include unit tests:

```bash
go test ./... -v
```

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

