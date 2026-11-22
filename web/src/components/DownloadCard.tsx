import React from 'react';

interface DownloadCardProps {
  downloadUrl: string;
  filename: string;
  synth: string;
  format: string;
  onReset: () => void;
}

export const DownloadCard: React.FC<DownloadCardProps> = ({
  downloadUrl,
  filename,
  synth,
  format,
  onReset,
}) => {
  return (
    <div className="card done-card">
      <div className="done-icon">🎉</div>
      <div className="done-title">Audiobook Ready!</div>
      <div className="done-filename">{filename}</div>
      <div className="done-meta">
        Synthesized with {synth.toUpperCase()} • {format.toUpperCase()} format
      </div>

      <div className="audio-player-container">
        <audio controls src={downloadUrl} className="audio-preview">
          Your browser does not support the audio element.
        </audio>
      </div>

      <div className="done-actions">
        <a
          href={downloadUrl}
          download={filename}
          className="btn btn-success"
        >
          ⬇️ Download Audiobook ({format.toUpperCase()})
        </a>
        <button className="btn btn-secondary" onClick={onReset}>
          Choose another book
        </button>
      </div>
    </div>
  );
};
