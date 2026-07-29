import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '../lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-[var(--ring)] focus:ring-offset-2 focus:ring-offset-[var(--background)]',
  {
    variants: {
      variant: {
        default:
          'bg-[var(--primary)] text-[var(--primary-foreground)] hover:bg-[var(--primary-soft)]',
        secondary:
          'bg-[var(--muted)] text-[var(--secondary-foreground)] hover:bg-[var(--secondary)]',
        destructive:
          'bg-[var(--accent-danger)] text-[var(--accent-danger-foreground)]',
        outline:
          'border border-[var(--border)] text-[var(--secondary-foreground)]',
        success:
          'bg-[var(--accent-success)] text-[var(--accent-success-foreground)]',
        warning:
          'bg-[var(--accent-warning)] text-[var(--accent-warning-foreground)]',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  )
}

export { Badge, badgeVariants }
