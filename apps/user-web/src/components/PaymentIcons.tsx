// ============================================================
// 官方支付品牌图标
// - 支付宝 / USDT：使用上传的官方图标图片（圆角）
// - 微信支付：官方绿 #07C160 双气泡 SVG
// - TRON/TRX: #FF060A 红底 SVG
// - BNB/BSC:  #F0B90B 金底黑菱形 SVG
// - ETH:      #627EEA 蓝紫底白菱形 SVG
// ============================================================

type IconProps = { size?: number; className?: string }

export function AlipayLogo({ size = 28, className }: IconProps) {
  return (
    <img
      src="/pay/alipay.jpg"
      alt="Alipay"
      width={size}
      height={size}
      className={className}
      style={{
        width: size,
        height: size,
        borderRadius: Math.round(size * 0.22),
        objectFit: 'cover',
        display: 'block',
      }}
    />
  )
}

export function UsdtLogo({ size = 28, className }: IconProps) {
  return (
    <img
      src="/pay/usdt.jpg"
      alt="USDT"
      width={size}
      height={size}
      className={className}
      style={{
        width: size,
        height: size,
        borderRadius: Math.round(size * 0.22),
        objectFit: 'cover',
        display: 'block',
      }}
    />
  )
}

export function WechatLogo({ size = 28, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg" className={className}>
      <rect width="48" height="48" rx="10" fill="#07C160" />
      <path d="M18.5 12C12.2 12 7 16.2 7 21.5c0 2.9 1.6 5.5 4.2 7.3l-1.1 3.3 3.9-2c1.3 0.4 2.8 0.6 4.3 0.7-0.1-0.5-0.2-1-0.2-1.5 0-5.1 4.8-9.2 10.7-9.2 0.4 0 0.9 0 1.3 0.1C28.8 15.2 24 12 18.5 12z" fill="white" />
      <circle cx="14" cy="20" r="1.5" fill="#07C160" />
      <circle cx="21.5" cy="20" r="1.5" fill="#07C160" />
      <path d="M40 29.3c0-4.3-4.3-7.8-9.5-7.8s-9.5 3.5-9.5 7.8 4.3 7.8 9.5 7.8c1.2 0 2.3-0.2 3.4-0.5l3.2 1.7-0.9-2.8c2.4-1.5 3.8-3.8 3.8-6.2z" fill="white" />
      <circle cx="33.7" cy="28.8" r="1.3" fill="#07C160" />
      <circle cx="40.2" cy="28.8" r="1.3" fill="#07C160" />
    </svg>
  )
}

export function TronLogo({ size = 28, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg" className={className}>
      <rect width="48" height="48" rx="24" fill="#FF060A" />
      <path d="M33.4 14.5L21.9 13l-1.4 6.2 12.9-1.7-10.8 6.2 7.6 8.5 5.3-14.4-12.9 1.7" fill="white" />
      <path d="M22.6 19.2l-7.9 1.2 4.8 13.8 5.9-10.4-2.8-4.6z" fill="white" opacity="0.7" />
    </svg>
  )
}

export function BscLogo({ size = 28, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg" className={className}>
      <rect width="48" height="48" rx="24" fill="#F0B90B" />
      <path d="M24 13.5l-4.2 4.2 4.2 4.2 4.2-4.2-4.2-4.2z" fill="#0B0E11" />
      <path d="M14.7 22.8l-4.2 4.2 4.2 4.2 4.2-4.2-4.2-4.2z" fill="#0B0E11" />
      <path d="M24 22.8l-4.2 4.2 4.2 4.2 4.2-4.2-4.2-4.2z" fill="#0B0E11" />
      <path d="M33.3 22.8l-4.2 4.2 4.2 4.2 4.2-4.2-4.2-4.2z" fill="#0B0E11" />
      <path d="M19.4 27.4l-4.2 4.2 4.2 4.2 4.2-4.2-4.2-4.2z" fill="#0B0E11" opacity="0.9" />
      <path d="M28.6 27.4l-4.2 4.2 4.2 4.2 4.2-4.2-4.2-4.2z" fill="#0B0E11" opacity="0.9" />
    </svg>
  )
}

export function EthLogo({ size = 28, className }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg" className={className}>
      <rect width="48" height="48" rx="24" fill="#627EEA" />
      <path d="M24.8 13v9.5l8 3.6-8-13.1z" fill="white" fillOpacity="0.95" />
      <path d="M24.8 13l-8 13.1 8-3.6V13z" fill="white" fillOpacity="0.7" />
      <path d="M24.8 31.6v7.4l8-10.2-8 2.8z" fill="white" fillOpacity="0.95" />
      <path d="M24.8 39v-7.4l-8-2.8 8 10.2z" fill="white" fillOpacity="0.7" />
      <path d="M24.8 29.6l8-3.6-8-3.6v7.2z" fill="white" fillOpacity="0.85" />
      <path d="M16.8 26l8 3.6v-7.2l-8 3.6z" fill="white" fillOpacity="0.55" />
    </svg>
  )
}

function ChainBadge({ chain }: { chain: 'trc' | 'bep' | 'erc' }) {
  const logo = chain === 'trc' ? <TronLogo size={15} /> : chain === 'bep' ? <BscLogo size={15} /> : <EthLogo size={15} />
  return (
    <span
      className="absolute rounded-full"
      style={{
        bottom: -2,
        right: -2,
        width: 19,
        height: 19,
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        border: '2px solid var(--card, #ffffff)',
        background: 'var(--card, #ffffff)',
        borderRadius: '50%',
        lineHeight: 0,
        boxShadow: '0 2px 6px rgba(0,0,0,0.12)',
      }}
    >
      {logo}
    </span>
  )
}

export function UsdtTrcLogo({ size = 28, className }: IconProps) {
  return (
    <span className={`relative inline-block ${className || ''}`} style={{ width: size, height: size }}>
      <UsdtLogo size={size} />
      <ChainBadge chain="trc" />
    </span>
  )
}

export function UsdtBepLogo({ size = 28, className }: IconProps) {
  return (
    <span className={`relative inline-block ${className || ''}`} style={{ width: size, height: size }}>
      <UsdtLogo size={size} />
      <ChainBadge chain="bep" />
    </span>
  )
}

export function UsdtErcLogo({ size = 28, className }: IconProps) {
  return (
    <span className={`relative inline-block ${className || ''}`} style={{ width: size, height: size }}>
      <UsdtLogo size={size} />
      <ChainBadge chain="erc" />
    </span>
  )
}

export function PaymentMethodIcon({ method, size = 28, className }: { method: string; size?: number; className?: string }) {
  switch (method) {
    case 'usdt_trc20':
    case 'usdt-trc20':
    case 'trc20':
      return <UsdtTrcLogo size={size} className={className} />
    case 'usdt_bep20':
    case 'usdt-bep20':
    case 'bep20':
      return <UsdtBepLogo size={size} className={className} />
    case 'usdt_erc20':
    case 'usdt-erc20':
    case 'erc20':
      return <UsdtErcLogo size={size} className={className} />
    case 'alipay':
      return <AlipayLogo size={size} className={className} />
    case 'wechat':
    case 'wxpay':
      return <WechatLogo size={size} className={className} />
    default:
      return <UsdtLogo size={size} className={className} />
  }
}
