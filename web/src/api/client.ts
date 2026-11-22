import { BookMeta, ConversionPayload, Voice } from '../types';

export async function uploadDocument(file: File): Promise<BookMeta> {
  const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.'));
  if (!['.epub', '.txt', '.pdf'].includes(ext)) {
    throw new Error('Please upload a valid .epub, .txt, or .pdf document.');
  }

  const formData = new FormData();
  formData.append('file', file);

  const res = await fetch('/api/upload', {
    method: 'POST',
    body: formData,
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || 'Failed to upload and parse document');
  }

  return res.json();
}

export async function fetchVoices(synth: string, lang: string): Promise<Voice[]> {
  const queryLang = lang || 'es';
  const res = await fetch(`/api/voices?synth=${encodeURIComponent(synth)}&lang=${encodeURIComponent(queryLang)}`);
  if (!res.ok) {
    return [];
  }
  return res.json();
}

export async function startConversion(payload: ConversionPayload): Promise<string> {
  const res = await fetch('/api/convert', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || 'Failed to start conversion');
  }

  const data = await res.json();
  return data.jobId;
}
