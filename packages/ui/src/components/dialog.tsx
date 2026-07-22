import * as React from 'react'
import * as ReactDOM from 'react-dom'
import { X } from 'lucide-react'
import { cn } from '../lib/utils'

interface DialogContextValue {
  open: boolean
  setOpen: (open: boolean) => void
}

const DialogContext = React.createContext<DialogContextValue | undefined>(undefined)

function useDialogContext() {
  const context = React.useContext(DialogContext)
  if (!context) {
    throw new Error('Dialog components must be used within a Dialog provider')
  }
  return context
}

interface DialogProps {
  open?: boolean
  defaultOpen?: boolean
  onOpenChange?: (open: boolean) => void
  children: React.ReactNode
}

function Dialog({ open: controlledOpen, defaultOpen = false, onOpenChange, children }: DialogProps) {
  const [internalOpen, setInternalOpen] = React.useState(defaultOpen)
  const isControlled = controlledOpen !== undefined
  const open = isControlled ? controlledOpen : internalOpen

  const setOpen = React.useCallback(
    (value: boolean) => {
      if (!isControlled) {
        setInternalOpen(value)
      }
      onOpenChange?.(value)
    },
    [isControlled, onOpenChange]
  )

  React.useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
    return () => {
      document.body.style.overflow = ''
    }
  }, [open])

  return (
    <DialogContext.Provider value={{ open, setOpen }}>
      {children}
    </DialogContext.Provider>
  )
}

interface DialogTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {}

const DialogTrigger = React.forwardRef<HTMLButtonElement, DialogTriggerProps>(
  ({ ...props }, ref) => {
    const { setOpen } = useDialogContext()
    return (
      <button
        ref={ref}
        onClick={() => setOpen(true)}
        {...props}
      />
    )
  }
)
DialogTrigger.displayName = 'DialogTrigger'

interface DialogContentProps extends React.HTMLAttributes<HTMLDivElement> {
  showCloseButton?: boolean
}

const DialogContent = React.forwardRef<HTMLDivElement, DialogContentProps>(
  ({ className, children, showCloseButton = true, ...props }, ref) => {
    const { open, setOpen } = useDialogContext()
    const [mounted, setMounted] = React.useState(false)
    const [isAnimating, setIsAnimating] = React.useState(false)

    React.useEffect(() => {
      if (open) {
        setMounted(true)
        requestAnimationFrame(() => {
          requestAnimationFrame(() => setIsAnimating(true))
        })
      } else if (mounted) {
        setIsAnimating(false)
        const timer = setTimeout(() => setMounted(false), 200)
        return () => clearTimeout(timer)
      }
    }, [open, mounted])

    React.useEffect(() => {
      const handleEscape = (e: KeyboardEvent) => {
        if (e.key === 'Escape' && open) {
          setOpen(false)
        }
      }
      document.addEventListener('keydown', handleEscape)
      return () => document.removeEventListener('keydown', handleEscape)
    }, [open, setOpen])

    if (!mounted) return null

    return ReactDOM.createPortal(
      <div className="fixed inset-0 z-50 flex items-center justify-center">
        <div
          className={cn(
            'absolute inset-0 bg-black/80 transition-opacity duration-200',
            isAnimating ? 'opacity-100' : 'opacity-0'
          )}
          onClick={() => setOpen(false)}
        />
        <div
          ref={ref}
          className={cn(
            'relative z-50 w-full max-w-lg rounded-xl border border-zinc-800 bg-zinc-900 p-6 shadow-lg transition-all duration-200',
            isAnimating
              ? 'opacity-100 scale-100 translate-y-0'
              : 'opacity-0 scale-95 translate-y-2',
            className
          )}
          role="dialog"
          aria-modal="true"
          onClick={(e) => e.stopPropagation()}
          {...props}
        >
          {children}
          {showCloseButton && (
            <button
              className="absolute right-4 top-4 rounded-sm opacity-70 ring-offset-zinc-950 transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:pointer-events-none"
              onClick={() => setOpen(false)}
            >
              <X className="h-4 w-4 text-zinc-400" />
              <span className="sr-only">Close</span>
            </button>
          )}
        </div>
      </div>,
      document.body
    )
  }
)
DialogContent.displayName = 'DialogContent'

const DialogHeader = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn('flex flex-col space-y-1.5 text-center sm:text-left pr-6', className)}
    {...props}
  />
)
DialogHeader.displayName = 'DialogHeader'

const DialogFooter = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2 mt-4', className)}
    {...props}
  />
)
DialogFooter.displayName = 'DialogFooter'

const DialogTitle = React.forwardRef<
  HTMLHeadingElement,
  React.HTMLAttributes<HTMLHeadingElement>
>(({ className, ...props }, ref) => (
  <h2
    ref={ref}
    className={cn('text-lg font-semibold leading-none tracking-tight text-zinc-100', className)}
    {...props}
  />
))
DialogTitle.displayName = 'DialogTitle'

const DialogDescription = React.forwardRef<
  HTMLParagraphElement,
  React.HTMLAttributes<HTMLParagraphElement>
>(({ className, ...props }, ref) => (
  <p
    ref={ref}
    className={cn('text-sm text-zinc-400', className)}
    {...props}
  />
))
DialogDescription.displayName = 'DialogDescription'

export { Dialog, DialogTrigger, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter }
