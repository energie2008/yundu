export function UsdtLogo({ size = 26 }: { size?: number }) {
  return (
    <span
      className="inline-flex items-center justify-center rounded-lg font-bold text-white flex-shrink-0"
      style={{
        width: size,
        height: size,
        background: '#26a17b',
        fontSize: Math.max(8, Math.round(size * 0.38)),
        letterSpacing: 0,
        lineHeight: 1,
      }}
      aria-label="USDT"
    >
      USDT
    </span>
  )
}

export function UsdtBadge({ size = 30 }: { size?: number }) {
  return (
    <span
      className="inline-flex items-center justify-center rounded-full text-white font-bold flex-shrink-0"
      style={{
        width: size,
        height: size,
        background: '#26a17b',
        fontSize: Math.max(10, Math.round(size * 0.5)),
        letterSpacing: 0,
        lineHeight: 1,
      }}
      aria-label="₮"
    >
      ₮
    </span>
  )
}
