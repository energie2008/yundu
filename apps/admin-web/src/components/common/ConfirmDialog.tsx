import { AlertTriangle } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Button,
} from '@airport/ui'

interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  warning?: string
  confirmText?: string
  cancelText?: string
  confirmVariant?: 'danger' | 'primary'
  onConfirm: () => void
  loading?: boolean
  children?: React.ReactNode
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  warning,
  confirmText = '确认',
  cancelText = '取消',
  confirmVariant = 'danger',
  onConfirm,
  loading = false,
  children,
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className={`w-5 h-5 ${confirmVariant === 'danger' ? 'text-red-400' : 'text-amber-400'}`} />
            {title}
          </DialogTitle>
          {description && <DialogDescription className="text-zinc-400">{description}</DialogDescription>}
        </DialogHeader>

        {warning && (
          <div className="bg-red-950/30 border border-red-900/50 rounded-lg p-3 text-sm text-red-300">
            {warning}
          </div>
        )}

        {children}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
            className="border-zinc-700 text-zinc-300"
          >
            {cancelText}
          </Button>
          <Button
            type="button"
            isLoading={loading}
            onClick={onConfirm}
            className={confirmVariant === 'danger' ? 'bg-red-600 hover:bg-red-500' : 'bg-indigo-600 hover:bg-indigo-500'}
          >
            {confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
