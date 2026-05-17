// Reusable presentational components styled after MongoDB's LeafyGreen system.

import type { ReactNode } from 'react'

type BadgeColor = 'gray' | 'green' | 'blue' | 'yellow' | 'red'

export function Badge({
  color = 'gray',
  children,
  dot,
}: {
  color?: BadgeColor
  children: ReactNode
  dot?: boolean
}) {
  return (
    <span className={`badge ${color}`}>
      {dot && <span className="dot" />}
      {children}
    </span>
  )
}

export function Card({
  title,
  desc,
  action,
  children,
}: {
  title?: string
  desc?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="card">
      {(title || action) && (
        <div className="card-head">
          <div>
            {title && <h2>{title}</h2>}
            {desc && <p className="card-desc" style={{ marginBottom: 0 }}>{desc}</p>}
          </div>
          {action}
        </div>
      )}
      <div style={{ marginTop: title || action ? 16 : 0 }}>{children}</div>
    </section>
  )
}

type ButtonVariant = 'default' | 'primary' | 'danger'

export function Button({
  children,
  variant = 'default',
  small,
  loading,
  ...rest
}: {
  children: ReactNode
  variant?: ButtonVariant
  small?: boolean
  loading?: boolean
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const cls = ['btn', variant !== 'default' ? variant : '', small ? 'small' : '']
    .filter(Boolean)
    .join(' ')
  return (
    <button className={cls} {...rest} disabled={rest.disabled || loading}>
      {loading && (
        <span className={`spinner ${variant === 'primary' ? 'light' : ''}`} />
      )}
      {children}
    </button>
  )
}

export function Banner({
  variant,
  children,
}: {
  variant: 'info' | 'success' | 'warning' | 'danger'
  children: ReactNode
}) {
  const icon =
    variant === 'danger'
      ? '✕'
      : variant === 'warning'
        ? '!'
        : variant === 'success'
          ? '✓'
          : 'i'
  return (
    <div className={`banner ${variant}`}>
      <span className="ico">{icon}</span>
      <div>{children}</div>
    </div>
  )
}

export function ProgressBar({
  value,
  indeterminate,
}: {
  value?: number
  indeterminate?: boolean
}) {
  const pct = Math.max(0, Math.min(100, value ?? 0))
  return (
    <div className={`progress ${indeterminate ? 'indeterminate' : ''}`}>
      <div className="fill" style={{ width: indeterminate ? undefined : `${pct}%` }} />
    </div>
  )
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="field">
      <label>{label}</label>
      {children}
      {hint && <span className="hint">{hint}</span>}
    </div>
  )
}

export function Checkbox({
  checked,
  onChange,
  title,
  description,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  title: string
  description?: string
}) {
  return (
    <label className="checkbox">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="ctext">
        <b>{title}</b>
        {description && <span>{description}</span>}
      </span>
    </label>
  )
}

export function Metric({
  label,
  value,
  small,
}: {
  label: string
  value: ReactNode
  small?: boolean
}) {
  return (
    <div className="metric">
      <div className="mlabel">{label}</div>
      <div className={`mvalue ${small ? 'small' : ''}`}>{value}</div>
    </div>
  )
}

export function Spinner({ light }: { light?: boolean }) {
  return <span className={`spinner ${light ? 'light' : ''}`} />
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel = 'Confirm',
  danger,
  busy,
  error,
  onConfirm,
  onCancel,
}: {
  title: string
  message: ReactNode
  confirmLabel?: string
  danger?: boolean
  busy?: boolean
  error?: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div className="modal-overlay" onClick={busy ? undefined : onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>{title}</h2>
        <p>{message}</p>
        {error && (
          <div style={{ marginBottom: 16 }}>
            <Banner variant="danger">{error}</Banner>
          </div>
        )}
        <div className="btn-row" style={{ justifyContent: 'flex-end' }}>
          <Button onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
          <Button
            variant={danger ? 'danger' : 'primary'}
            onClick={onConfirm}
            loading={busy}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}
