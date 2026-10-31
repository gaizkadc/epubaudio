import React, { useState, useEffect, useRef } from 'react';

interface BookMeta {
  filename: string;
  title: string;
  author: string;
  language: string;
  totalChapters: number;
  totalCharacters: number;
}

interface Voice {
  id?: string;
  name?: string;
  language?: string;
  gender?: string;
  // Fallbacks in case of PascalCase serialization
  ID?: string;
  Name?: string;
  Language?: string;
  Gender?: string;
}

interface ProgressEvent {
  stage?: string;
  Stage?: string;
  chapterIndex?: number;
  ChapterIndex?: number;
  chapterTitle?: string;
  ChapterTitle?: string;
  currentChunk?: number;
  CurrentChunk?: number;
  totalChunks?: number;
  TotalChunks?: number;
  doneChapters?: number;
  DoneChapters?: number;
  totalChapters?: number;
  TotalChapters?: number;
  message?: string;
  Message?: string;
  outputFile?: string;
  downloadUrl?: string;
  error?: string;
}

function parseChapterCount(input: string, total: number): number {
  const trimmed = input.trim().toLowerCase();
  if (!trimmed || trimmed === 'all' || trimmed === '*') {
    return total;
  }
  const seen = new Set<number>();
  const parts = trimmed.split(',');
  for (const part of parts) {
    const p = part.trim();
    if (!p) continue;
    if (p.includes('-')) {
      const [startStr, endStr] = p.split('-');
      const start = parseInt(startStr, 10);
      const end = parseInt(endStr, 10);
      if (!isNaN(start) && !isNaN(end)) {
        const [low, high] = start <= end ? [start, end] : [end, start];
        for (let i = low; i <= high; i++) {
          if (i >= 0 && i < total) seen.add(i);
        }
      }
    } else {
      const idx = parseInt(p, 10);
      if (!isNaN(idx) && idx >= 0 && idx < total) {
        seen.add(idx);
      }
    }
  }
  return seen.size || total;
}

function cleanVoiceName(rawName: string, id: string): string {
  if (!rawName) return id;
  return rawName
    .replace(/^Microsoft\s+/i, '')
    .replace(/\s+Online\s+\(Natural\)\s*-\s*/i, ' - ')
    .replace(/\s+Online\s+\(Natural\)/i, '')
    .trim();
}

export default function App() {
  const [dragActive, setDragActive] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [book, setBook] = useState<BookMeta | null>(null);
  const [synth, setSynth] = useState('edge');
  const [voices, setVoices] = useState<Voice[]>([]);
  const [selectedVoice, setSelectedVoice] = useState('');
  const [format, setFormat] = useState('m4b');
  const [chapters, setChapters] = useState('all');

  const [stage, setStage] = useState<'idle' | 'ready' | 'converting' | 'done'>('idle');
  const [progress, setProgress] = useState<ProgressEvent | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [downloadInfo, setDownloadInfo] = useState<{ url: string; filename: string } | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Load voices whenever synth or book language changes
  useEffect(() => {
    if (!book) return;
    const fetchVoices = async () => {
      try {
        const lang = book.language || 'es';
        const res = await fetch(`/api/voices?synth=${synth}&lang=${encodeURIComponent(lang)}`);
        if (res.ok) {
          const list: Voice[] = await res.json();
          setVoices(list);
          if (list.length > 0) {
            const firstId = list[0].id || list[0].ID || '';
            setSelectedVoice(firstId);
          } else {
            setSelectedVoice('');
          }
        }
      } catch (err) {
        console.error('Failed to fetch voices:', err);
      }
    };
    fetchVoices();
  }, [synth, book?.language]);

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      uploadFile(e.dataTransfer.files[0]);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      uploadFile(e.target.files[0]);
    }
  };

  const uploadFile = async (file: File) => {
    if (!file.name.toLowerCase().endsWith('.epub')) {
      setError('Please upload a valid .epub eBook file.');
      return;
    }

    setError(null);
    setUploading(true);

    const formData = new FormData();
    formData.append('file', file);

    try {
      const res = await fetch('/api/upload', {
        method: 'POST',
        body: formData,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to upload and parse EPUB');
      }

      const data: BookMeta = await res.json();
      setBook(data);
      setStage('ready');
    } catch (err: any) {
      setError(err.message || 'Error uploading file');
    } finally {
      setUploading(false);
    }
  };

  const startConversion = async () => {
    if (!book) return;

    setError(null);
    setStage('converting');
    setLogs([]);
    setProgress(null);

    try {
      const res = await fetch('/api/convert', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filename: book.filename,
          synth,
          voice: selectedVoice,
          format,
          chapters,
        }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to start conversion');
      }

      const { jobId } = await res.json();

      // Connect to SSE for real-time progress
      const sse = new EventSource(`/api/progress?jobId=${jobId}`);
      eventSourceRef.current = sse;

      sse.onmessage = (event) => {
        try {
          const ev: ProgressEvent = JSON.parse(event.data);
          setProgress(ev);

          if (ev.message) {
            setLogs((prev) => [...prev.slice(-30), ev.message]);
          }

          const stageName = (ev.stage || (ev as any).Stage || '').toLowerCase();
          if (stageName === 'done') {
            sse.close();
            setStage('done');
            const url = ev.downloadUrl || (ev as any).DownloadURL || '';
            const file = ev.outputFile || (ev as any).OutputFile || '';
            if (url && file) {
              setDownloadInfo({
                url,
                filename: file,
              });
            } else if (book) {
              const fallbackName = `${book.title || 'audiobook'} [${synth}].${format}`;
              setDownloadInfo({
                url: `/api/download?file=${encodeURIComponent(fallbackName)}`,
                filename: fallbackName,
              });
            }
          } else if (stageName === 'error') {
            sse.close();
            setError(ev.error || (ev as any).Error || 'Conversion failed');
            setStage('ready');
          }
        } catch (e) {
          console.error('SSE parse error:', e);
        }
      };

      sse.onerror = (err) => {
        console.error('SSE connection error:', err);
        sse.close();
      };
    } catch (err: any) {
      setError(err.message || 'Error starting conversion');
      setStage('ready');
    }
  };

  const resetAll = () => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }
    setBook(null);
    setStage('idle');
    setProgress(null);
    setLogs([]);
    setDownloadInfo(null);
    setError(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  // Calculations for progress bars
  const selectedChapterCount = parseChapterCount(chapters, book?.totalChapters || 1);
  const totalChapters = progress?.totalChapters || (progress as any)?.TotalChapters || selectedChapterCount;
  const doneChapters = progress?.doneChapters || (progress as any)?.DoneChapters || 0;
  const chapterPercent = Math.min(100, Math.round((doneChapters / totalChapters) * 100));

  const totalChunks = progress?.totalChunks || 1;
  const currentChunk = progress?.currentChunk || 0;
  const chunkPercent = Math.min(100, Math.round((currentChunk / totalChunks) * 100));

  return (
    <div className="container">
      <header>
        <h1>epubaudio 🎧📖</h1>
        <p>Turn your EPUB eBooks into audiobooks with natural neural TTS</p>
      </header>

      {error && <div className="error-banner">⚠️ {error}</div>}

      {stage === 'idle' && (
        <div className="card">
          <input
            ref={fileInputRef}
            type="file"
            accept=".epub"
            style={{ display: 'none' }}
            onChange={handleFileChange}
          />
          <div
            className={`dropzone ${dragActive ? 'active' : ''}`}
            onDragEnter={handleDrag}
            onDragLeave={handleDrag}
            onDragOver={handleDrag}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
          >
            <div className="dropzone-icon">📖</div>
            <div className="dropzone-text">
              {uploading ? 'Reading EPUB metadata...' : 'Drag & drop an .epub eBook here'}
            </div>
            <div className="dropzone-hint">or click to browse from your computer</div>
          </div>
        </div>
      )}

      {stage === 'ready' && book && (
        <div className="card">
          <div className="book-header">
            <div className="book-title">{book.title || 'Untitled Book'}</div>
            <div className="book-author">{book.author ? `by ${book.author}` : 'Unknown Author'}</div>

            <div className="book-meta-grid">
              <div className="meta-item">
                <div className="meta-label">Language</div>
                <div className="meta-value">{book.language ? book.language.toUpperCase() : 'ES'}</div>
              </div>
              <div className="meta-item">
                <div className="meta-label">Chapters</div>
                <div className="meta-value">{book.totalChapters}</div>
              </div>
              <div className="meta-item">
                <div className="meta-label">Total Size</div>
                <div className="meta-value">~{(book.totalCharacters / 1000).toFixed(0)}k chars</div>
              </div>
            </div>
          </div>

          <div className="form-grid">
            <div className="form-group">
              <label htmlFor="synth">Synthesizer Engine</label>
              <select id="synth" value={synth} onChange={(e) => setSynth(e.target.value)}>
                <option value="edge">Microsoft Edge-TTS (Cloud, Natural)</option>
                <option value="macos">macOS 'say' (Offline, Built-in)</option>
                <option value="kokoro">Kokoro-82M (Offline Neural)</option>
                <option value="piper">Piper (Offline VITS)</option>
              </select>
            </div>

            <div className="form-group">
              <label htmlFor="voice">Voice</label>
              <select
                id="voice"
                value={selectedVoice}
                onChange={(e) => setSelectedVoice(e.target.value)}
              >
                {voices.length > 0 ? (
                  voices.map((v, i) => {
                    const id = v.id || v.ID || '';
                    const rawName = v.name || v.Name || id;
                    const clean = cleanVoiceName(rawName, id);
                    const lang = v.language || v.Language || '';
                    const gender = v.gender || v.Gender || '';
                    const label = `${clean}${lang ? ` (${lang})` : ''}${gender ? ` • ${gender}` : ''}`;
                    return (
                      <option key={id || i} value={id} title={label}>
                        {label}
                      </option>
                    );
                  })
                ) : (
                  <option value="">Default Voice</option>
                )}
              </select>
            </div>

            <div className="form-group">
              <label htmlFor="format">Output Format</label>
              <select id="format" value={format} onChange={(e) => setFormat(e.target.value)}>
                <option value="m4b">.m4b (Audiobook with chapters)</option>
                <option value="mp3">.mp3 (Single MP3)</option>
              </select>
            </div>

            <div className="form-group">
              <label htmlFor="chapters">
                <span>Chapters to Convert</span>
                <span className="label-hint">
                  {selectedChapterCount} of {book.totalChapters} chapters
                </span>
              </label>
              <input
                id="chapters"
                type="text"
                value={chapters}
                placeholder="e.g. all or 0-1 or 1,2,5"
                onChange={(e) => setChapters(e.target.value)}
              />
            </div>
          </div>

          <button className="btn btn-primary" onClick={startConversion}>
            🚀 Start Conversion
          </button>

          <div style={{ marginTop: '14px', textAlign: 'center' }}>
            <button className="btn btn-secondary" onClick={resetAll}>
              Choose another book
            </button>
          </div>
        </div>
      )}

      {stage === 'converting' && (
        <div className="card progress-card">
          <div className="progress-header">
            <div>
              <div style={{ fontSize: '1.1rem', fontWeight: 600 }}>Converting Audiobook</div>
              <div style={{ fontSize: '0.85rem', color: '#94a3b8' }}>
                {progress?.chapterTitle
                  ? `Active: "${progress.chapterTitle}"`
                  : 'Synthesizing speech...'}
              </div>
            </div>
            <span className="status-badge">
              <span>●</span> {progress?.stage || 'Synthesizing'}
            </span>
          </div>

          {/* Overall Chapter Progress Bar */}
          <div className="progress-bar-container">
            <div className="progress-labels">
              <span className="progress-title">
                Chapter Progress: {doneChapters} of {totalChapters} chapters
              </span>
              <span className="progress-percent">{chapterPercent}%</span>
            </div>
            <div className="progress-track">
              <div className="progress-fill" style={{ width: `${chapterPercent}%` }} />
            </div>
          </div>

          {/* Intra-Chapter Chunk Progress Bar */}
          {totalChunks > 1 && (
            <div className="progress-bar-container">
              <div className="progress-labels">
                <span className="progress-title">
                  Current Chapter Chunks: {currentChunk} of {totalChunks}
                </span>
                <span className="progress-percent">{chunkPercent}%</span>
              </div>
              <div className="progress-track">
                <div className="progress-fill chunk" style={{ width: `${chunkPercent}%` }} />
              </div>
            </div>
          )}

          {/* Terminal log messages */}
          <div className="log-box">
            {logs.length > 0 ? (
              logs.map((line, idx) => <div key={idx}>{line}</div>)
            ) : (
              <div>Initializing audio synthesis pipeline...</div>
            )}
          </div>
        </div>
      )}

      {stage === 'done' && (
        <div className="card done-card">
          <div className="done-icon">🎉</div>
          <div className="done-title">Audiobook Ready!</div>
          {downloadInfo ? (
            <>
              <div className="done-filename">{downloadInfo.filename}</div>
              <a
                href={downloadInfo.url}
                download={downloadInfo.filename}
                className="btn btn-success"
              >
                ⬇️ Download Audiobook ({format.toUpperCase()})
              </a>
            </>
          ) : (
            <div style={{ color: '#94a3b8' }}>Generating download link...</div>
          )}
          <button className="btn btn-secondary" onClick={resetAll} style={{ marginTop: '10px' }}>
            Convert Another Book
          </button>
        </div>
      )}
    </div>
  );
}

