import React, { useRef, useState } from 'react';

interface DropzoneProps {
  uploading: boolean;
  onFileSelected: (file: File) => void;
}

type InputMode = 'file' | 'text';

const FilesSVG = () => (
  <svg width="90" height="72" viewBox="0 0 90 72" fill="none" xmlns="http://www.w3.org/2000/svg">
    {/* Blue file */}
    <rect x="0" y="14" width="34" height="44" rx="5" fill="#8ecae6" />
    <rect x="0" y="14" width="34" height="44" rx="5" stroke="#219ebc" strokeWidth="2"/>
    <path d="M8 14V8a2 2 0 0 1 2-2h14l10 10v38a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2V14z" fill="#8ecae6"/>
    <path d="M22 6l10 10H22V6z" fill="#219ebc" opacity="0.6"/>
    <line x1="8" y1="26" x2="26" y2="26" stroke="#219ebc" strokeWidth="2" strokeLinecap="round"/>
    <line x1="8" y1="32" x2="26" y2="32" stroke="#219ebc" strokeWidth="2" strokeLinecap="round"/>
    <line x1="8" y1="38" x2="20" y2="38" stroke="#219ebc" strokeWidth="2" strokeLinecap="round"/>
    {/* Yellow file (middle) */}
    <rect x="28" y="4" width="34" height="46" rx="5" fill="#ffd166" />
    <rect x="28" y="4" width="34" height="46" rx="5" stroke="#ffb703" strokeWidth="2"/>
    <path d="M36 4V0a0 0 0 0 1 0 0h14l10 10v36a2 2 0 0 1-2 2H38a2 2 0 0 1-2-2V4z" fill="#ffd166"/>
    <path d="M50 0l10 10H50V0z" fill="#ffb703" opacity="0.7"/>
    <line x1="36" y1="20" x2="54" y2="20" stroke="#ffb703" strokeWidth="2" strokeLinecap="round"/>
    <line x1="36" y1="27" x2="54" y2="27" stroke="#ffb703" strokeWidth="2" strokeLinecap="round"/>
    <line x1="36" y1="34" x2="48" y2="34" stroke="#ffb703" strokeWidth="2" strokeLinecap="round"/>
    {/* Coral file */}
    <rect x="56" y="14" width="34" height="44" rx="5" fill="#ff9b8a" />
    <rect x="56" y="14" width="34" height="44" rx="5" stroke="#e8632a" strokeWidth="2"/>
    <path d="M64 14V8a2 2 0 0 1 2-2h14l10 10v38a2 2 0 0 1-2 2H66a2 2 0 0 1-2-2V14z" fill="#ff9b8a"/>
    <path d="M78 6l10 10H78V6z" fill="#e8632a" opacity="0.6"/>
    <line x1="64" y1="26" x2="82" y2="26" stroke="#e8632a" strokeWidth="2" strokeLinecap="round"/>
    <line x1="64" y1="32" x2="82" y2="32" stroke="#e8632a" strokeWidth="2" strokeLinecap="round"/>
    <line x1="64" y1="38" x2="76" y2="38" stroke="#e8632a" strokeWidth="2" strokeLinecap="round"/>
  </svg>
);

function buildFilenameFromText(rawText: string): string {
  const firstLine = rawText
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line.length > 0) || 'pasted-text';

  const slug = firstLine
    .slice(0, 40)
    .toLowerCase()
    .replace(/[^a-z0-9\u00C0-\u024F\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-');

  const suffix = Date.now().toString(36).slice(-4);
  return `${slug || 'pasted-text'}-${suffix}.txt`;
}

export const Dropzone: React.FC<DropzoneProps> = ({ uploading, onFileSelected }) => {
  const [mode, setMode] = useState<InputMode>('file');
  const [dragActive, setDragActive] = useState(false);
  const [pastedText, setPastedText] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

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
      onFileSelected(e.dataTransfer.files[0]);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      onFileSelected(e.target.files[0]);
    }
  };

  const handleTextSubmit = () => {
    const trimmed = pastedText.trim();
    if (!trimmed || uploading) return;
    const filename = buildFilenameFromText(trimmed);
    const file = new File([trimmed], filename, { type: 'text/plain' });
    onFileSelected(file);
  };

  return (
    <div className="dropzone-card">
      {/* Background blobs */}
      <div className="blob blob-teal" />
      <div className="blob blob-coral" />
      <div className="blob blob-sun" />
      <div className="blob blob-sky" />

      <div className="dropzone-inner">
        <div className="input-mode-tabs" role="tablist" aria-label="Input method">
          <button
            type="button"
            role="tab"
            aria-selected={mode === 'file'}
            className={`input-mode-tab ${mode === 'file' ? 'active' : ''}`}
            onClick={() => setMode('file')}
          >
            📄 Upload File
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === 'text'}
            className={`input-mode-tab ${mode === 'text' ? 'active' : ''}`}
            onClick={() => setMode('text')}
          >
            ✏️ Paste Text
          </button>
        </div>

        {mode === 'file' ? (
          <>
            <input
              ref={fileInputRef}
              type="file"
              accept=".epub,.txt,.pdf"
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
              <FilesSVG />
              <div className="dropzone-text">
                {uploading ? 'Reading document…' : 'Drag your file here (EPUB, PDF, TXT)'}
              </div>
              <div className="dropzone-hint">or click to select from computer</div>
            </div>

            <div className="dropzone-stars">
              <span>⭐</span>
              <span>✨</span>
              <span>⭐</span>
            </div>
          </>
        ) : (
          <div className="paste-text-container">
            <textarea
              className="paste-text-area"
              value={pastedText}
              onChange={(e) => setPastedText(e.target.value)}
              placeholder="Paste or type your text here..."
              rows={8}
              disabled={uploading}
            />
            <div className="paste-text-footer">
              <span className="char-count">
                {pastedText.trim().length.toLocaleString()} chars
              </span>
              <button
                type="button"
                className="btn btn-primary paste-submit-btn"
                onClick={handleTextSubmit}
                disabled={uploading || pastedText.trim().length === 0}
              >
                {uploading ? 'Processing…' : 'Continue →'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
