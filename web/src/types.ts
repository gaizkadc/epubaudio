export interface BookMeta {
  filename: string;
  title: string;
  author: string;
  language: string;
  totalChapters: number;
  totalCharacters: number;
}

export interface Voice {
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

export interface ProgressEvent {
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

export interface ConversionPayload {
  filename: string;
  title?: string;
  author?: string;
  language?: string;
  synth: string;
  voice: string;
  format: string;
  chapters: string;
}

export type AppStage = 'idle' | 'ready' | 'converting' | 'done';
