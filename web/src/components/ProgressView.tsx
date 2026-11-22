import React, { useEffect, useRef } from 'react';
import { ProgressEvent } from '../types';

interface ProgressViewProps {
  progress: ProgressEvent | null;
  logs: string[];
  totalSelectedChapters: number;
}

export const ProgressView: React.FC<ProgressViewProps> = ({
  progress,
  logs,
  totalSelectedChapters,
}) => {
  const logBoxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (logBoxRef.current) {
      logBoxRef.current.scrollTop = logBoxRef.current.scrollHeight;
    }
  }, [logs]);

  const totalChapters = progress?.totalChapters || (progress as any)?.TotalChapters || totalSelectedChapters || 1;
  const doneChapters = progress?.doneChapters || (progress as any)?.DoneChapters || 0;
  const chapterPercent = Math.min(100, Math.round((doneChapters / Math.max(1, totalChapters)) * 100));

  const totalChunks = progress?.totalChunks || 1;
  const currentChunk = progress?.currentChunk || 0;
  const chunkPercent = Math.min(100, Math.round((currentChunk / Math.max(1, totalChunks)) * 100));

  const currentChapterTitle = progress?.chapterTitle || (progress as any)?.ChapterTitle || 'Synthesizing speech...';
  const stageName = progress?.stage || (progress as any)?.Stage || 'Synthesizing';

  return (
    <div className="card progress-card">
      <div className="progress-header">
        <div>
          <div className="progress-heading">
            Synthesizing Audiobook...
          </div>
          <div className="progress-subheading">
            Active: <strong>"{currentChapterTitle}"</strong>
          </div>
        </div>
        <span className="status-badge pulse">
          <span className="badge-dot">●</span> {stageName}
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
      <div className="logs-section">
        <div className="logs-header">Activity Log</div>
        <div className="log-box" ref={logBoxRef}>
          {logs.length > 0 ? (
            logs.map((line, idx) => (
              <div key={idx} className="log-line">
                <span className="log-prompt">›</span>
                <span className="log-text">{line}</span>
              </div>
            ))
          ) : (
            <div className="log-placeholder">Initializing audio synthesis pipeline...</div>
          )}
        </div>
      </div>
    </div>
  );
};
