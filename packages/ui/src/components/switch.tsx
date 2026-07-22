import * as React from 'react'
import { cn } from '../lib/utils'

export interface SwitchProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {}

const Switch = React.forwardRef<HTMLInputElement, SwitchProps>(
  ({ className, checked, defaultChecked, onChange, disabled, ...props }, ref) => {
    return (
      <label
        className={cn(
          'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer items-center rounded-full transition-colors focus-within:outline-none focus-within:ring-2 focus-within:ring-indigo-500 focus-within:ring-offset-2 focus-within:ring-offset-zinc-950',
          checked ? 'bg-indigo-600' : 'bg-zinc-700',
          disabled && 'cursor-not-allowed opacity-50',
          className
        )}
      >
        <input
          type="checkbox"
          className="sr-only"
          ref={ref}
          checked={checked}
          defaultChecked={defaultChecked}
          onChange={onChange}
          disabled={disabled}
          {...props}
        />
        <span
          className={cn(
            'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition-transform',
            checked ? 'translate-x-4' : 'translate-x-0.5'
          )}
        />
      </label>
    )
  }
)
Switch.displayName = 'Switch'

export { Switch }
