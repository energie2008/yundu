// CSS variable-based theme constants for YunDu Admin
// All colors reference CSS variables defined in index.css
// This allows automatic light/dark mode switching

export const ADMIN_BG = 'var(--background)'
export const ADMIN_CARD = 'var(--card)'
export const ADMIN_CARD_HOVER = 'var(--card-hover)'
export const ADMIN_BORDER = 'var(--border)'
export const ADMIN_BORDER_HOVER = 'var(--border-hover)'
export const ADMIN_INPUT_BG = 'var(--card)'
export const ADMIN_INPUT_BORDER = 'var(--border)'
export const ADMIN_TEXT = 'var(--foreground)'
export const ADMIN_TEXT_SECONDARY = 'var(--secondary-foreground)'
export const ADMIN_TEXT_MUTED = 'var(--muted-foreground)'
export const ADMIN_GRADIENT = 'var(--gradient-primary)'
export const ADMIN_GRADIENT_SOFT = 'var(--gradient-soft)'
export const ADMIN_ACCENT = 'var(--primary)'
export const ADMIN_ACCENT_GLOW = 'var(--primary-glow)'
export const ADMIN_SUCCESS = 'var(--success)'
export const ADMIN_WARNING = 'var(--warning)'
export const ADMIN_DANGER = 'var(--destructive)'
export const ADMIN_INFO = 'var(--info)'

// Sidebar specific
export const ADMIN_SIDEBAR_BG = 'var(--sidebar-bg)'
export const ADMIN_SIDEBAR_BORDER = 'var(--sidebar-border)'
export const ADMIN_SIDEBAR_TEXT = 'var(--sidebar-text)'
export const ADMIN_SIDEBAR_ACTIVE = 'var(--sidebar-active)'
export const ADMIN_SIDEBAR_ACTIVE_TEXT = 'var(--sidebar-active-text)'
export const ADMIN_SIDEBAR_HEADER = 'var(--sidebar-header)'

// Header specific
export const ADMIN_HEADER_BG = 'var(--header-bg)'
export const ADMIN_HEADER_BORDER = 'var(--header-border)'

// Accent colors for rich variety
export const ADMIN_ACCENT_PINK = 'var(--accent-pink-foreground)'
export const ADMIN_ACCENT_SKY = 'var(--accent-sky-foreground)'
export const ADMIN_ACCENT_EMERALD = 'var(--accent-emerald-foreground)'
export const ADMIN_ACCENT_AMBER = 'var(--accent-amber-foreground)'
export const ADMIN_ACCENT_ROSE = 'var(--accent-rose-foreground)'

// Gradients
export const ADMIN_GRADIENT_SUCCESS = 'var(--gradient-success)'
export const ADMIN_GRADIENT_WARNING = 'var(--gradient-warning)'
export const ADMIN_GRADIENT_DANGER = 'var(--gradient-danger)'
export const ADMIN_GRADIENT_INFO = 'var(--gradient-info)'

// Shadows
export const ADMIN_SHADOW_SM = 'var(--shadow-sm)'
export const ADMIN_SHADOW_MD = 'var(--shadow-md)'
export const ADMIN_SHADOW_LG = 'var(--shadow-lg)'
export const ADMIN_SHADOW_GLOW = 'var(--shadow-glow)'

export const statCardStyle = {
  backgroundColor: ADMIN_CARD,
  border: `1px solid ${ADMIN_BORDER}`,
  borderRadius: '16px',
  transition: 'all 0.2s ease',
}

export const cardStyle = {
  backgroundColor: ADMIN_CARD,
  border: `1px solid ${ADMIN_BORDER}`,
  borderRadius: '16px',
}

export const inputStyle = {
  backgroundColor: ADMIN_INPUT_BG,
  borderColor: ADMIN_INPUT_BORDER,
  color: ADMIN_TEXT,
}

export const badgeStyle = {
  backgroundColor: ADMIN_ACCENT_GLOW,
  color: ADMIN_ACCENT,
  border: `1px solid ${ADMIN_BORDER}`,
}
