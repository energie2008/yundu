import * as React from 'react'
import { cn } from '../lib/utils'
import { Inbox } from 'lucide-react'

interface EmptyStateProps extends React.HTMLAttributes<HTMLDivElement> {
  icon?: React.ReactNode
  title: string
  description?: string
  action?: React.ReactNode
}

function EmptyState({
  className,
  icon,
  title,
  description,
  action,
  ...props
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center py-12 text-center',
        className
      )}
      {...props}
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-zinc-800/50 text-zinc-400 mb-4">
        {icon || <Inbox className="h-6 w-6" />}
      </div>
      <h3 className="text-base font-medium text-zinc-100">{title}</h3>
      {description && (
        <p className="mt-1 text-sm text-zinc-500 max-w-sm">{description}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export { EmptyState }
