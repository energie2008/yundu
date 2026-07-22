import * as React from 'react'
import { cn } from '@airport/ui'
import { Check } from 'lucide-react'

export interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {}

export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, checked, defaultChecked, onChange, disabled, ...props }, ref) => {
    return (
      <label
        className={cn(
          'relative inline-flex h-4 w-4 flex-shrink-0 cursor-pointer items-center justify-center rounded border transition-colors',
          checked ? 'bg-indigo-600 border-indigo-600' : 'bg-zinc-800 border-zinc-600 hover:border-zinc-500',
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
        {checked && <Check className="h-3 w-3 text-white" />}
      </label>
    )
  }
)
Checkbox.displayName = 'Checkbox'
