interface QRCodeProps {
  value: string
  size?: number
  className?: string
}

export function QRCode({ value, size = 180, className = '' }: QRCodeProps) {
  const qrUrl = `https://api.qrserver.com/v1/create-qr-code/?size=${size}x${size}&data=${encodeURIComponent(value)}&margin=10&bgcolor=18181b&color=fafafa`

  return (
    <div className={`flex items-center justify-center ${className}`} style={{ width: size, height: size }}>
      <img
        src={qrUrl}
        alt="QR Code"
        width={size}
        height={size}
        className="rounded-lg"
        crossOrigin="anonymous"
      />
    </div>
  )
}
