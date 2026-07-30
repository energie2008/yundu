import { useEffect, useRef, useState } from 'react';
import { LOCALES, useI18n, type LangCode } from '../lib/i18n';

// Top-right language dropdown. Matches both light/dark themes via CSS vars.
export function LanguageSelector() {
  const { lang, setLang } = useI18n();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const current = LOCALES.find(l => l.code === lang) || LOCALES[0];

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [open]);

  const pick = (code: LangCode) => {
    setLang(code);
    setOpen(false);
  };

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className="h-8 px-2.5 rounded-lg flex items-center gap-1.5 text-xs transition-colors"
        style={{ color: 'var(--muted-foreground)' }}
        onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10" />
          <line x1="2" y1="12" x2="22" y2="12" />
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
        <span>{current.short} {current.label}</span>
        <span style={{ fontSize: 9 }}>▾</span>
      </button>
      {open && (
        <div
          className="absolute right-0 top-full mt-1 py-1 rounded-lg border shadow-lg z-50 min-w-[150px]"
          style={{ background: 'var(--card)', borderColor: 'var(--border)' }}
          role="listbox"
        >
          {LOCALES.map(l => (
            <button
              key={l.code}
              onClick={() => pick(l.code)}
              className="w-full text-left px-3 py-2 text-xs flex items-center justify-between transition-colors"
              style={{
                color: l.code === lang ? 'var(--primary)' : 'var(--foreground)',
                fontWeight: l.code === lang ? 600 : 400,
              }}
              onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
              role="option"
              aria-selected={l.code === lang}
            >
              <span>
                <span className="inline-block w-7 font-semibold">{l.short}</span>
                {l.label}
              </span>
              {l.code === lang && <span>✓</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// Floating variant for pages without a header (login / register / etc.)
export function FloatingLanguageSelector() {
  return (
    <div className="fixed top-4 right-4 z-50">
      <div
        className="rounded-lg border shadow-sm"
        style={{ background: 'var(--card)', borderColor: 'var(--border)' }}
      >
        <LanguageSelector />
      </div>
    </div>
  );
}
