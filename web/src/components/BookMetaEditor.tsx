import React from 'react';
import { BookMeta } from '../types';
import { normalizeLangCode } from '../utils/formatters';

interface BookMetaEditorProps {
  book: BookMeta;
  title: string;
  author: string;
  language: string;
  onTitleChange: (val: string) => void;
  onAuthorChange: (val: string) => void;
  onLanguageChange: (val: string) => void;
}

export const BookMetaEditor: React.FC<BookMetaEditorProps> = ({
  book,
  title,
  author,
  language,
  onTitleChange,
  onAuthorChange,
  onLanguageChange,
}) => {
  const normLang = normalizeLangCode(language);

  return (
    <div className="book-header">
      <div className="book-info">
        <input
          type="text"
          className="book-title-input"
          value={title}
          onChange={(e) => onTitleChange(e.target.value)}
          placeholder="Document Title"
          title="Click to edit document title"
        />
        <div className="book-author-row">
          <span className="author-prefix">by</span>
          <input
            type="text"
            className="book-author-input"
            value={author}
            onChange={(e) => onAuthorChange(e.target.value)}
            placeholder="Author name (optional)"
            title="Click to edit author"
          />
        </div>
      </div>

      <div className="book-meta-grid">
        <div className="meta-item">
          <div className="meta-label">Language</div>
          <div className="meta-select-wrapper">
            <select
              id="meta-language"
              value={normLang}
              onChange={(e) => onLanguageChange(e.target.value)}
              className="meta-stat-select"
              title="Click to change language and filter voices"
            >
              <option value="es">ES (Spanish)</option>
              <option value="en">EN (English)</option>
              <option value="fr">FR (French)</option>
              <option value="de">DE (German)</option>
              <option value="it">IT (Italian)</option>
              <option value="pt">PT (Portuguese)</option>
              <option value="ca">CA (Catalan)</option>
              <option value="gl">GL (Galician)</option>
              <option value="eu">EU (Basque)</option>
              <option value="nl">NL (Dutch)</option>
              <option value="pl">PL (Polish)</option>
              <option value="ru">RU (Russian)</option>
              <option value="ja">JA (Japanese)</option>
              <option value="zh">ZH (Chinese)</option>
              <option value="all">ALL (All Voices)</option>
            </select>
          </div>
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
  );
};
