import { useEffect, useState } from 'react'
import QRCodeLib from 'qrcode'
import { PaymentMethodIcon } from './PaymentIcons'

interface QRCodeProps {
  value: string
  size?: number
  /** 支付方式标识（alipay/wechat/usdt_xxx），用于二维码中央 logo */
  logo?: string
  className?: string
}

/**
 * 二维码组件：本地生成（不依赖外部 API），经典白底黑格样式，
 * 错误校正级别 H，支持中央叠加支付方式 logo。
 */
export function QRCode({ value, size = 180, logo, className = '' }: QRCodeProps) {
  const [dataUrl, setDataUrl] = useState('')

  useEffect(() => {
    let cancelled = false
    QRCodeLib.toDataURL(value, {
      width: size * 2,
      margin: 2,
      errorCorrectionLevel: 'H',
      color: { dark: '#000000', light: '#ffffff' },
    })
      .then(url => { if (!cancelled) setDataUrl(url) })
      .catch(() => { if (!cancelled) setDataUrl('') })
    return () => { cancelled = true }
  }, [value, size])

  return (
    <div className={`relative flex items-center justify-center ${className}`} style={{ width: size, height: size }}>
      {dataUrl ? (
        <img src={dataUrl} alt="二维码" width={size} height={size} className="rounded-lg bg-white" style={{ imageRendering: 'pixelated' }} />
      ) : (
        <div style={{ width: size, height: size }} />
      )}
      {logo && (
        <div
          className="absolute flex items-center justify-center rounded-md bg-white"
          style={{ width: Math.round(size * 0.24), height: Math.round(size * 0.24), boxShadow: '0 0 4px rgba(0,0,0,0.15)' }}
        >
          <PaymentMethodIcon method={logo} size={Math.round(size * 0.2)} />
        </div>
      )}
    </div>
  )
}
