import * as React from 'react'
import { cn } from '@airport/ui'

interface DropdownContextValue {
  open: boolean
  setOpen: (open: boolean) => void
}

const DropdownContext = React.createContext<DropdownContextValue | undefined>(undefined)

function useDropdown() {
  const ctx = React.useContext(DropdownContext)
  if (!ctx) throw new Error('Dropdown components must be used within DropdownMenu')
  return ctx
}

interface DropdownMenuProps {
  children: React.ReactNode
}

export function DropdownMenu({ children }: DropdownMenuProps) {
  const [open, setOpen] = React.useState(false)
  const ref = React.useRef<HTMLDivElement>(null)

  React.useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    function handleEscape(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    if (open) {
      document.addEventListener('mousedown', handleClickOutside)
      document.addEventListener('keydown', handleEscape)
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [open])

  return (
    <DropdownContext.Provider value={{ open, setOpen }}>
      <div ref={ref} className="relative inline-block">
        {children}
      </div>
    </DropdownContext.Provider>
  )
}

interface DropdownMenuTriggerProps {
  children: React.ReactNode
  asChild?: boolean
}

export function DropdownMenuTrigger({ children }: DropdownMenuTriggerProps) {
  const { open, setOpen } = useDropdown()
  return (
    <div onClick={() => setOpen(!open)} className="cursor-pointer">
      {children}
    </div>
  )
}

interface DropdownMenuContentProps extends React.HTMLAttributes<HTMLDivElement> {
  align?: 'left' | 'right'
}

export function DropdownMenuContent({
  className,
  align = 'right',
  children,
  ...props
}: DropdownMenuContentProps) {
  const { open } = useDropdown()
  if (!open) return null
  return (
    <div
      className={cn(
        'absolute z-50 mt-1 min-w-[160px] rounded-lg border border-zinc-800 bg-zinc-900 py-1 shadow-lg',
        align === 'right' ? 'right-0' : 'left-0',
        className
      )}
      onClick={(e) => e.stopPropagation()}
      {...props}
    >
      {children}
    </div>
  )
}

interface DropdownMenuItemProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'default' | 'danger'
}

export const DropdownMenuItem = React.forwardRef<HTMLButtonElement, DropdownMenuItemProps>(
  ({ className, variant = 'default', onClick, children, ...props }, ref) => {
    const { setOpen } = useDropdown()
    return (
      <button
        ref={ref}
        className={cn(
          'w-full px-3 py-2 text-left text-sm transition-colors flex items-center gap-2',
          variant === 'danger'
            ? 'text-red-400 hover:bg-red-950/50 hover:text-red-300'
            : 'text-zinc-300 hover:bg-zinc-800 hover:text-zinc-100',
          className
        )}
        onClick={(e) => {
          onClick?.(e)
          setOpen(false)
        }}
        {...props}
      >
        {children}
      </button>
    )
  }
)
DropdownMenuItem.displayName = 'DropdownMenuItem'

export function DropdownMenuSeparator() {
  return <div className="my-1 h-px bg-zinc-800" />
}

export function DropdownMenuLabel({ children, className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn('px-3 py-1.5 text-xs font-semibold text-zinc-500', className)}
      {...props}
    >
      {children}
    </div>
  )
}
