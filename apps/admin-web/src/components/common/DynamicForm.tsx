import * as React from 'react'
import { Input, Label, Textarea, Switch, Button, Badge } from '@airport/ui'
import { RefreshCw } from 'lucide-react'

export interface SchemaField {
  type: 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array'
  title?: string
  description?: string
  default?: unknown
  enum?: (string | number)[]
  enumNames?: Record<string, string>
  format?: string
  pattern?: string
  minLength?: number
  maxLength?: number
  minimum?: number
  maximum?: number
  required?: string[]
  properties?: Record<string, SchemaField>
  items?: SchemaField
  secret?: boolean
  sensitive?: boolean
  autoGenerate?: 'uuid' | 'password' | 'path' | 'base64key'
  placeholder?: string
}

interface DynamicFormProps {
  schema: SchemaField
  value: Record<string, unknown>
  onChange: (value: Record<string, unknown>) => void
  required?: string[]
  depth?: number
}

function generateRandomString(length: number): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

function generateBase64Key(): string {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  let binary = ''
  bytes.forEach((b) => (binary += String.fromCharCode(b)))
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function generateUUID(): string {
  if (typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

function getFieldLabel(field: SchemaField, key: string): string {
  return field.title || key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

function renderFieldInput(
  key: string,
  field: SchemaField,
  fieldValue: unknown,
  onFieldChange: (val: unknown) => void,
  isRequired: boolean
) {
  const inputClass = 'bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500'

  if (field.enum && field.enum.length > 0) {
    return (
      <select
        className={`flex h-9 w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500 appearance-none bg-[url("data:image/svg+xml;charset=utf-8,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' fill=\'none\' viewBox=\'0 0 20 20\'%3E%3Cpath stroke=\'%2371717a\' stroke-linecap=\'round\' stroke-linejoin=\'round\' stroke-width=\'1.5\' d=\'M6 8l4 4 4-4\'/%3E%3C/svg%3E")] bg-[length:1.25rem_1.25rem] bg-[right_0.5rem_center] bg-no-repeat pr-8`}
        value={String(fieldValue ?? field.default ?? '')}
        onChange={(e) => onFieldChange(e.target.value)}
      >
        {field.enum.map((opt) => (
          <option key={String(opt)} value={String(opt)}>
            {field.enumNames?.[String(opt)] || String(opt)}
          </option>
        ))}
      </select>
    )
  }

  if (field.type === 'boolean') {
    return (
      <Switch
        checked={Boolean(fieldValue ?? field.default ?? false)}
        onChange={(e) => onFieldChange(e.target.checked)}
      />
    )
  }

  if (field.type === 'number' || field.type === 'integer') {
    const numVal = fieldValue ?? field.default
    return (
      <Input
        type="number"
        className={inputClass}
        value={numVal !== undefined && numVal !== null ? String(numVal) : ''}
        placeholder={field.placeholder}
        onChange={(e) => {
          const val = e.target.value
          onFieldChange(val === '' ? undefined : Number(val))
        }}
      />
    )
  }

  if (field.format === 'textarea' || field.type === 'string' && field.maxLength && field.maxLength > 200) {
    return (
      <Textarea
        className={`${inputClass} min-h-[60px]`}
        value={String(fieldValue ?? field.default ?? '')}
        placeholder={field.placeholder}
        onChange={(e) => onFieldChange(e.target.value)}
      />
    )
  }

  const isSensitive = field.sensitive || field.secret || key.toLowerCase().includes('password') ||
    key.toLowerCase().includes('secret') || key.toLowerCase().includes('private_key')

  const autoBtn = field.autoGenerate ? (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="absolute right-1 top-1/2 -translate-y-1/2 h-7 px-2 text-zinc-400 hover:text-indigo-400"
      onClick={() => {
        let generated = ''
        switch (field.autoGenerate) {
          case 'uuid':
            generated = generateUUID()
            break
          case 'password':
            generated = generateUUID()
            break
          case 'path':
            generated = '/' + generateRandomString(16 + Math.floor(Math.random() * 16))
            break
          case 'base64key':
            generated = generateBase64Key()
            break
        }
        onFieldChange(generated)
      }}
    >
      <RefreshCw className="w-3 h-3 mr-1" />
      生成
    </Button>
  ) : null

  return (
    <div className="relative">
      <Input
        type={isSensitive ? 'password' : 'text'}
        className={`${inputClass} ${autoBtn ? 'pr-16' : ''}`}
        value={String(fieldValue ?? field.default ?? '')}
        placeholder={field.placeholder}
        onChange={(e) => onFieldChange(e.target.value)}
      />
      {autoBtn}
    </div>
  )
}

export function DynamicForm({ schema, value, onChange, required, depth = 0 }: DynamicFormProps) {
  if (!schema.properties) return null

  const requiredFields = required || schema.required || []

  const handleFieldChange = (key: string, val: unknown) => {
    onChange({ ...value, [key]: val })
  }

  return (
    <div className={depth === 0 ? 'space-y-4' : 'space-y-3'}>
      {Object.entries(schema.properties).map(([key, field]) => {
        const isRequired = requiredFields.includes(key)
        const fieldValue = value[key]

        if (field.type === 'object' && field.properties) {
          return (
            <div key={key} className="p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
              <div className="text-sm font-medium text-zinc-300 mb-2 flex items-center gap-2">
                {getFieldLabel(field, key)}
                {isRequired && <span className="text-red-400">*</span>}
                {field.description && (
                  <span className="text-xs text-zinc-500 font-normal">{field.description}</span>
                )}
              </div>
              <DynamicForm
                schema={field}
                value={(value[key] as Record<string, unknown>) || {}}
                onChange={(val) => handleFieldChange(key, val)}
                required={field.required}
                depth={depth + 1}
              />
            </div>
          )
        }

        return (
          <div key={key} className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-zinc-300 text-sm flex items-center gap-1">
                {getFieldLabel(field, key)}
                {isRequired && <span className="text-red-400">*</span>}
                {(field.sensitive || field.secret) && (
                  <Badge variant="outline" className="text-[10px] px-1 py-0 border-amber-800 text-amber-400">
                    敏感
                  </Badge>
                )}
              </Label>
              {field.autoGenerate && (
                <span className="text-xs text-indigo-400">可自动生成</span>
              )}
            </div>
            {field.description && (
              <p className="text-xs text-zinc-500">{field.description}</p>
            )}
            {renderFieldInput(key, field, fieldValue, (val) => handleFieldChange(key, val), isRequired)}
          </div>
        )
      })}
    </div>
  )
}
