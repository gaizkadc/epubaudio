import React, { useRef, useState } from 'react';

interface DropzoneProps {
  uploading: boolean;
  onFileSelected: (file: File) => void;
}

export const Dropzone: React.FC<DropzoneProps> = ({ uploading, onFileSelected }) => {
  const [dragActive, setDragActive] = useState(false);
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

  return (
    <div className="card">
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
        <div className="dropzone-icon">📚</div>
        <div className="dropzone-text">
          {uploading ? 'Reading document metadata...' : 'Drag & drop an .epub, .txt, or .pdf document here'}
        </div>
        <div className="dropzone-hint">or click to browse from your computer</div>
      </div>
    </div>
  );
};
