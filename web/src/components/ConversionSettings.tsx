import React from 'react';
import { Voice } from '../types';
import { cleanVoiceName, parseChapterCount } from '../utils/formatters';

interface ConversionSettingsProps {
  synth: string;
  voices: Voice[];
  selectedVoice: string;
  loadingVoices: boolean;
  format: string;
  chapters: string;
  totalChapters: number;
  onSynthChange: (synth: string) => void;
  onVoiceChange: (voice: string) => void;
  onFormatChange: (format: string) => void;
  onChaptersChange: (chapters: string) => void;
  onSubmit: () => void;
  onReset: () => void;
}

export const ConversionSettings: React.FC<ConversionSettingsProps> = ({
  synth,
  voices,
  selectedVoice,
  loadingVoices,
  format,
  chapters,
  totalChapters,
  onSynthChange,
  onVoiceChange,
  onFormatChange,
  onChaptersChange,
  onSubmit,
  onReset,
}) => {
  const selectedCount = parseChapterCount(chapters, totalChapters);

  return (
    <>
      <div className="form-grid">
        <div className="form-group">
          <label htmlFor="synth">Synthesizer Engine</label>
          <select
            id="synth"
            value={synth}
            onChange={(e) => onSynthChange(e.target.value)}
          >
            <option value="edge">Microsoft Edge-TTS (Cloud, Natural)</option>
            <option value="macos">macOS 'say' (Offline, Built-in)</option>
            <option value="kokoro">Kokoro-82M (Offline Neural)</option>
            <option value="piper">Piper (Offline VITS)</option>
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="voice">
            <span>Voice</span>
            {loadingVoices && <span className="label-hint">Updating...</span>}
          </label>
          <select
            id="voice"
            value={selectedVoice}
            disabled={loadingVoices}
            onChange={(e) => onVoiceChange(e.target.value)}
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
              <option value="">{loadingVoices ? 'Loading voices...' : 'Default Voice'}</option>
            )}
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="format">Output Format</label>
          <select
            id="format"
            value={format}
            onChange={(e) => onFormatChange(e.target.value)}
          >
            <option value="m4b">.m4b (Audiobook with chapters)</option>
            <option value="mp3">.mp3 (Single MP3)</option>
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="chapters">
            <span>Chapters to Convert</span>
            <span className="label-hint">
              {selectedCount} of {totalChapters} chapters
            </span>
          </label>
          <input
            id="chapters"
            type="text"
            value={chapters}
            placeholder="e.g. all or 0-1 or 1,2,5"
            onChange={(e) => onChaptersChange(e.target.value)}
          />
        </div>
      </div>

      <button className="btn btn-primary" onClick={onSubmit} style={{ marginTop: '24px' }}>
        🚀 Start Conversion
      </button>

      <div style={{ marginTop: '14px', textAlign: 'center' }}>
        <button className="btn btn-secondary" onClick={onReset}>
          Choose another book
        </button>
      </div>
    </>
  );
};
