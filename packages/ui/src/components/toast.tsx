import * as React from 'react'
import * as ReactDOM from 'react-dom'
import { X, CheckCircle, AlertCircle, Info } from 'lucide-react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '../lib/utils'

const toastVariants = cva(
  'pointer-events-auto relative flex w-full items-center justify-between space-x-4 overflow-hidden rounded-lg border p-4 pr-8 shadow-lg transition-all',
  {
    variants: {
      variant: {
        default: 'border-zinc-800 bg-zinc-900 text-zinc-100',
        destructive: 'border-red-900/50 bg-red-950/80 text-red-200',
        success: 'border-emerald-900/50 bg-emerald-950/80 text-emerald-200',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
)

export interface ToastProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof toastVariants> {
  id?: string
  title?: string
  description?: string
  duration?: number
  onClose?: () => void
}

const ToastIcon = ({ variant }: { variant: ToastProps['variant'] }) => {
  switch (variant) {
    case 'success':
      return <CheckCircle className="h-5 w-5 text-emerald-400" />
    case 'destructive':
      return <AlertCircle className="h-5 w-5 text-red-400" />
    default:
      return <Info className="h-5 w-5 text-indigo-400" />
  }
}

const Toast = React.forwardRef<HTMLDivElement, ToastProps>(
  ({ className, variant, title, description, onClose, id: _id, duration: _duration, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn(toastVariants({ variant }), className)}
        role="alert"
        {...props}
      >
        <div className="flex items-start gap-3">
          <ToastIcon variant={variant} />
          <div className="flex-1">
            {title && <div className="text-sm font-medium">{title}</div>}
            {description && (
              <div className={cn('text-sm mt-0.5', variant === 'destructive' ? 'text-red-300' : variant === 'success' ? 'text-emerald-300' : 'text-zinc-400')}>
                {description}
              </div>
            )}
          </div>
        </div>
        {onClose && (
          <button
            onClick={onClose}
            className="absolute right-2 top-2 rounded-md p-1 opacity-70 transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
    )
  }
)
Toast.displayName = 'Toast'

interface ToastItem {
  id: string
  title?: string
  description?: string
  variant?: ToastProps['variant']
  duration?: number
}

interface ToastContextValue {
  toasts: ToastItem[]
  addToast: (toast: Omit<ToastItem, 'id'>) => string
  removeToast: (id: string) => void
}

const ToastContext = React.createContext<ToastContextValue | undefined>(undefined)

let toastIdCounter = 0

function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = React.useState<ToastItem[]>([])

  const removeToast = React.useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const addToast = React.useCallback((toast: Omit<ToastItem, 'id'>) => {
    const id = `toast-${++toastIdCounter}`
    const duration = toast.duration ?? 5000

    setToasts((prev) => [...prev, { ...toast, id }])

    if (duration > 0) {
      setTimeout(() => {
        removeToast(id)
      }, duration)
    }

    return id
  }, [removeToast])

  return (
    <ToastContext.Provider value={{ toasts, addToast, removeToast }}>
      {children}
      <ToastViewport />
    </ToastContext.Provider>
  )
}

function ToastViewport() {
  const context = React.useContext(ToastContext)
  const [mounted, setMounted] = React.useState(false)

  React.useEffect(() => {
    setMounted(true)
    return () => setMounted(false)
  }, [])

  if (!context || !mounted) return null

  return ReactDOM.createPortal(
    <div className="fixed top-4 left-1/2 z-[100] flex -translate-x-1/2 flex-col gap-2 w-full max-w-sm px-4">
      {context.toasts.map((toast) => (
        <div
          key={toast.id}
          className="animate-[slideIn_0.3s_ease-out]"
        >
          <Toast
            variant={toast.variant}
            title={toast.title}
            description={toast.description}
            onClose={() => context.removeToast(toast.id)}
          />
        </div>
      ))}
    </div>,
    document.body
  )
}

function useToast() {
  const context = React.useContext(ToastContext)

  if (!context) {
    throw new Error('useToast must be used within a ToastProvider')
  }

  const toast = React.useMemo(
    () => (props: Omit<ToastItem, 'id'>) => context.addToast(props),
    [context]
  )

  return {
    toast,
    dismiss: context.removeToast,
    toasts: context.toasts,
  }
}

export { Toast, ToastProvider, ToastViewport, useToast, toastVariants }
