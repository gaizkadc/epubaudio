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
  const downloadLink = downloadUrl.includes('?')
    ? `${downloadUrl}&download=true`
    : `${downloadUrl}?download=true`;

  return (
    <div className="card done-card">
      <div className="done-title">Here's your audiobook!</div>
      <div className="done-filename">{filename}</div>
      <div className="done-meta">
        {synth.toUpperCase()} · {format.toUpperCase()}
      </div>

      <div className="audio-player-container">
        <audio controls preload="metadata" className="audio-preview" key={downloadUrl}>
          <source
            src={downloadUrl}
            type={format === 'mp3' ? 'audio/mpeg' : 'audio/mp4'}
          />
          Your browser does not support the audio element.
        </audio>
      </div>

      <div className="done-actions">
        <a
          href={downloadLink}
          download={filename}
          className="btn btn-success"
        >
          ⬇ Download {format.toUpperCase()}
        </a>
        <button className="btn btn-secondary" onClick={onReset}>
          Convert another book
        </button>
      </div>
    </div>
  );
};
