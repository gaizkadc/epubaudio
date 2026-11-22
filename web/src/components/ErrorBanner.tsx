import React from 'react';

interface ErrorBannerProps {
  error: string | null;
  onDismiss?: () => void;
}

export const ErrorBanner: React.FC<ErrorBannerProps> = ({ error, onDismiss }) => {
  if (!error) return null;
  return (
    <div className="error-banner">
      <span>⚠️ {error}</span>
      {onDismiss && (
        <button
          onClick={onDismiss}
          style={{
            background: 'none',
            border: 'none',
            color: 'inherit',
            cursor: 'pointer',
            float: 'right',
            fontWeight: 'bold',
          }}
          title="Dismiss"
        >
          ✕
        </button>
      )}
    </div>
  );
};
