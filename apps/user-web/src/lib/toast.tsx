import React, { createContext, useContext, useState, useCallback, useEffect } from 'react';

type ToastVariant = 'default' | 'success' | 'destructive';

interface Toast {
  id: number;
  title?: string;
  description?: string;
  variant?: ToastVariant;
}

interface ToastContextType {
  toast: (opts: { title?: string; description?: string; variant?: ToastVariant }) => void;
}

const ToastContext = createContext<ToastContextType>({ toast: () => {} });

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const toast = useCallback((opts: { title?: string; description?: string; variant?: ToastVariant }) => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, ...opts }]);
  }, []);

  const remove = useCallback((id: number) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div className="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
        {toasts.map(t => (
          <ToastItem key={t.id} toast={t} onClose={() => remove(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

function ToastItem({ toast, onClose }: { toast: Toast; onClose: () => void }) {
  useEffect(() => {
    const timer = setTimeout(onClose, 4000);
    return () => clearTimeout(timer);
  }, [onClose]);

  const borderColor =
    toast.variant === 'success' ? 'var(--success)' :
    toast.variant === 'destructive' ? 'var(--destructive)' :
    'var(--primary)';

  const iconColor =
    toast.variant === 'success' ? 'var(--success)' :
    toast.variant === 'destructive' ? 'var(--destructive)' :
    'var(--primary)';

  return (
    <div
      className="xboard-card p-4 animate-slide-up flex items-start gap-3 min-w-[280px]"
      style={{ borderLeft: `3px solid ${borderColor}` }}
    >
      <div
        className="w-2 h-2 rounded-full mt-1.5 flex-shrink-0"
        style={{ background: iconColor }}
      />
      <div className="flex-1 min-w-0">
        {toast.title && (
          <div className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>{toast.title}</div>
        )}
        {toast.description && (
          <div className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>{toast.description}</div>
        )}
      </div>
      <button
        onClick={onClose}
        className="text-xs flex-shrink-0 transition-colors"
        style={{ color: 'var(--muted-foreground)' }}
        onMouseEnter={e => (e.currentTarget.style.color = 'var(--foreground)')}
        onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
      >
        ×
      </button>
    </div>
  );
}

export function useToast() {
  return useContext(ToastContext);
}
