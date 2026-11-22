import React, { useState, useEffect, useRef } from 'react';
import { BookMeta, Voice, ProgressEvent, AppStage } from './types';
import { uploadDocument, fetchVoices as apiFetchVoices, startConversion as apiStartConversion } from './api/client';
import { parseChapterCount, normalizeLangCode } from './utils/formatters';
import { Header } from './components/Header';
import { ErrorBanner } from './components/ErrorBanner';
import { Dropzone } from './components/Dropzone';
import { BookMetaEditor } from './components/BookMetaEditor';
import { ConversionSettings } from './components/ConversionSettings';
import { ProgressView } from './components/ProgressView';
import { DownloadCard } from './components/DownloadCard';

export default function App() {
  const [stage, setStage] = useState<AppStage>('idle');
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Book & Metadata
  const [book, setBook] = useState<BookMeta | null>(null);
  const [editableTitle, setEditableTitle] = useState('');
  const [editableAuthor, setEditableAuthor] = useState('');
  const [editableLanguage, setEditableLanguage] = useState('es');

  // Synthesis configuration
  const [synth, setSynth] = useState('edge');
  const [voices, setVoices] = useState<Voice[]>([]);
  const [selectedVoice, setSelectedVoice] = useState('');
  const [loadingVoices, setLoadingVoices] = useState(false);
  const [format, setFormat] = useState('m4b');
  const [chapters, setChapters] = useState('all');

  // Conversion Progress & Output
  const [progress, setProgress] = useState<ProgressEvent | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [downloadInfo, setDownloadInfo] = useState<{ url: string; filename: string } | null>(null);

  const eventSourceRef = useRef<EventSource | null>(null);

  // Fetch voices whenever synth engine, language, or active book changes
  useEffect(() => {
    if (!book) return;
    let isCancelled = false;

    const loadVoices = async () => {
      setLoadingVoices(true);
      try {
        const list = await apiFetchVoices(synth, editableLanguage);
        if (!isCancelled) {
          setVoices(list);
          if (list.length > 0) {
            setSelectedVoice((prev) => {
              const exists = list.some((v) => (v.id || v.ID) === prev);
              return exists ? prev : (list[0].id || list[0].ID || '');
            });
          } else {
            setSelectedVoice('');
          }
        }
      } catch (err: any) {
        if (!isCancelled) {
          console.error('Failed to load voices:', err);
        }
      } finally {
        if (!isCancelled) {
          setLoadingVoices(false);
        }
      }
    };

    loadVoices();
    return () => {
      isCancelled = true;
    };
  }, [synth, editableLanguage, book?.filename]);

  const handleFileUpload = async (file: File) => {
    setError(null);
    setUploading(true);

    try {
      const data = await uploadDocument(file);
      setBook(data);
      setEditableTitle(data.title || '');
      setEditableAuthor(data.author || '');
      const initialLang = normalizeLangCode(data.language || 'es');
      setEditableLanguage(initialLang);
      setStage('ready');
    } catch (err: any) {
      setError(err.message || 'Error uploading file');
    } finally {
      setUploading(false);
    }
  };

  const handleStartConversion = async () => {
    if (!book) return;

    setError(null);
    setStage('converting');
    setLogs([]);
    setProgress(null);

    try {
      const jobId = await apiStartConversion({
        filename: book.filename,
        title: editableTitle,
        author: editableAuthor,
        language: editableLanguage,
        synth,
        voice: selectedVoice,
        format,
        chapters,
      });

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
              setDownloadInfo({ url, filename: file });
            } else if (book) {
              const bookTitle = editableTitle || book.title || 'audiobook';
              const fallbackName = `${bookTitle} [${synth}].${format}`;
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

  const handleReset = () => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }
    setBook(null);
    setEditableTitle('');
    setEditableAuthor('');
    setEditableLanguage('es');
    setStage('idle');
    setProgress(null);
    setLogs([]);
    setDownloadInfo(null);
    setError(null);
  };

  const selectedChapterCount = parseChapterCount(chapters, book?.totalChapters || 1);

  return (
    <div className="container">
      <Header />
      <ErrorBanner error={error} onDismiss={() => setError(null)} />

      {stage === 'idle' && (
        <Dropzone uploading={uploading} onFileSelected={handleFileUpload} />
      )}

      {stage === 'ready' && book && (
        <div className="card">
          <BookMetaEditor
            book={book}
            title={editableTitle}
            author={editableAuthor}
            language={editableLanguage}
            onTitleChange={setEditableTitle}
            onAuthorChange={setEditableAuthor}
            onLanguageChange={setEditableLanguage}
          />
          <ConversionSettings
            synth={synth}
            voices={voices}
            selectedVoice={selectedVoice}
            loadingVoices={loadingVoices}
            format={format}
            chapters={chapters}
            totalChapters={book.totalChapters}
            onSynthChange={setSynth}
            onVoiceChange={setSelectedVoice}
            onFormatChange={setFormat}
            onChaptersChange={setChapters}
            onSubmit={handleStartConversion}
            onReset={handleReset}
          />
        </div>
      )}

      {stage === 'converting' && (
        <ProgressView
          progress={progress}
          logs={logs}
          totalSelectedChapters={selectedChapterCount}
        />
      )}

      {stage === 'done' && downloadInfo && (
        <DownloadCard
          downloadUrl={downloadInfo.url}
          filename={downloadInfo.filename}
          synth={synth}
          format={format}
          onReset={handleReset}
        />
      )}
    </div>
  );
}
