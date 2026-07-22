import * as React from 'react'
import { Button, Badge } from '@airport/ui'
import { AlertTriangle, CheckCircle2, Copy } from 'lucide-react'
import { load as yamlLoad, dump as yamlDump } from 'js-yaml'

export type EditorMode = 'yaml' | 'json'

interface YamlEditorProps {
  value: string
  onChange: (value: string) => void
  mode?: EditorMode
  onModeChange?: (mode: EditorMode) => void
  error?: string | null
  height?: number
  placeholder?: string
  showModeToggle?: boolean
}

export function tryParseJson(text: string): { valid: boolean; data: unknown; error?: string } {
  try {
    const data = JSON.parse(text)
    return { valid: true, data }
  } catch (e) {
    return { valid: false, data: null, error: (e as Error).message }
  }
}

export function tryParseYamlSimple(text: string): { valid: boolean; data: unknown; error?: string } {
  if (!text.trim()) return { valid: true, data: null }
  const jsonResult = tryParseJson(text)
  if (jsonResult.valid) {
    return jsonResult
  }
  try {
    const data = yamlLoad(text)
    if (data === undefined || data === null) {
      return { valid: true, data: null }
    }
    return { valid: true, data }
  } catch (e) {
    return { valid: false, data: null, error: (e as Error).message }
  }
}

function countLines(text: string): number {
  return text.split('\n').length
}

export function YamlEditor({
  value,
  onChange,
  mode = 'json',
  onModeChange,
  error,
  height = 400,
  placeholder,
  showModeToggle = true,
}: YamlEditorProps) {
  const textareaRef = React.useRef<HTMLTextAreaElement>(null)
  const lineNumbersRef = React.useRef<HTMLDivElement>(null)
  const [internalError, setInternalError] = React.useState<string | null>(null)
  const [copied, setCopied] = React.useState(false)

  const lineCount = React.useMemo(() => countLines(value), [value])

  const parseResult = React.useMemo(() => {
    if (!value.trim()) return { valid: true, data: null, error: null }
    if (mode === 'json') {
      return tryParseJson(value)
    }
    return tryParseYamlSimple(value)
  }, [value, mode])

  const displayError = error || internalError || parseResult.error

  React.useEffect(() => {
    if (parseResult.valid && !error) {
      setInternalError(null)
    } else if (parseResult.error && !error) {
      setInternalError(parseResult.error)
    }
  }, [parseResult, error])

  const handleScroll = React.useCallback(() => {
    if (textareaRef.current && lineNumbersRef.current) {
      lineNumbersRef.current.scrollTop = textareaRef.current.scrollTop
    }
  }, [])

  const handleCopy = React.useCallback(() => {
    navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [value])

  const handleFormat = React.useCallback(() => {
    if (mode === 'json') {
      try {
        const parsed = JSON.parse(value)
        onChange(JSON.stringify(parsed, null, 2))
      } catch {
      }
    }
  }, [value, mode, onChange])

  const switchMode = React.useCallback((newMode: EditorMode) => {
    if (newMode === mode) return

    if (newMode === 'json') {
      const result = mode === 'yaml' ? tryParseYamlSimple(value) : tryParseJson(value)
      if (result.valid && result.data !== null && result.data !== undefined) {
        onChange(JSON.stringify(result.data, null, 2))
        onModeChange?.(newMode)
      }
    } else {
      const yamlResult = tryParseYamlSimple(value)
      if (yamlResult.valid && yamlResult.data !== null && yamlResult.data !== undefined) {
        onChange(yamlDump(yamlResult.data, { indent: 2, lineWidth: -1, noRefs: true }))
        onModeChange?.(newMode)
      } else {
        try {
          const parsed = JSON.parse(value)
          onChange(yamlDump(parsed, { indent: 2, lineWidth: -1, noRefs: true }))
          onModeChange?.(newMode)
        } catch {
        }
      }
    }
  }, [mode, value, onChange, onModeChange])

  return (
    <div className="rounded-lg border border-zinc-700 bg-zinc-900 overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 border-b border-zinc-700 bg-zinc-800/50">
        <div className="flex items-center gap-2">
          {showModeToggle && (
            <div className="flex items-center gap-1">
              <Button
                type="button"
                variant={mode === 'yaml' ? 'secondary' : 'ghost'}
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={() => switchMode('yaml')}
              >
                YAML
              </Button>
              <Button
                type="button"
                variant={mode === 'json' ? 'secondary' : 'ghost'}
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={() => switchMode('json')}
              >
                JSON
              </Button>
            </div>
          )}
          {displayError ? (
            <Badge variant="outline" className="text-red-400 border-red-900 bg-red-950/30 text-xs gap-1">
              <AlertTriangle className="w-3 h-3" />
              语法错误
            </Badge>
          ) : value.trim() ? (
            <Badge variant="outline" className="text-emerald-400 border-emerald-900 bg-emerald-950/30 text-xs gap-1">
              <CheckCircle2 className="w-3 h-3" />
              格式有效
            </Badge>
          ) : null}
        </div>
        <div className="flex items-center gap-1">
          {mode === 'json' && value.trim() && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-zinc-400 hover:text-zinc-200"
              onClick={handleFormat}
            >
              格式化
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs text-zinc-400 hover:text-zinc-200"
            onClick={handleCopy}
          >
            <Copy className="w-3 h-3 mr-1" />
            {copied ? '已复制' : '复制'}
          </Button>
        </div>
      </div>

      <div className="flex" style={{ height }}>
        <div
          ref={lineNumbersRef}
          className="flex-shrink-0 bg-zinc-950/50 border-r border-zinc-800 text-right pr-2 pl-3 py-2 overflow-hidden select-none"
          style={{ width: '3.5rem' }}
        >
          <div className="font-mono text-xs text-zinc-600 leading-6">
            {Array.from({ length: lineCount }, (_, i) => (
              <div key={i + 1} className="h-6">
                {i + 1}
              </div>
            ))}
          </div>
        </div>

        <textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onScroll={handleScroll}
          placeholder={placeholder || (mode === 'yaml' ? '# 输入YAML配置...' : '// 输入JSON配置...')}
          spellCheck={false}
          className={`flex-1 bg-zinc-950 text-zinc-300 font-mono text-xs leading-6 p-2 resize-none focus:outline-none ${
            displayError ? 'text-red-300' : ''
          }`}
          style={{ tabSize: 2 }}
        />
      </div>

      {displayError && (
        <div className="px-3 py-2 bg-red-950/30 border-t border-red-900/50">
          <p className="text-xs text-red-400 font-mono truncate">{displayError}</p>
        </div>
      )}
    </div>
  )
}
