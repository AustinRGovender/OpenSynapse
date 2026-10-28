import { useState, type FormEvent } from 'react'
import { useAuthStore } from '../../stores/auth-store'
import { Button, Field, TextInput } from '../ui/primitives'

function Frame({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-full items-center justify-center bg-slate-950 px-4">
      <div className="w-full max-w-sm rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-xl">
        <p className="text-xs font-semibold uppercase tracking-wider text-teal-400">OpenSynapse</p>
        <h1 className="mt-1 text-lg font-semibold text-slate-100">{title}</h1>
        <p className="mt-1 text-sm text-slate-400">{subtitle}</p>
        <div className="mt-5">{children}</div>
      </div>
    </div>
  )
}

/** Sign-in for team mode. */
export function LoginScreen() {
  const login = useAuthStore((s) => s.login)
  const notice = useAuthStore((s) => s.error)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await login(email, password)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign-in failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Frame title="Sign in" subtitle="This installation is shared by a team. Use the account your admin created for you.">
      <form onSubmit={submit} className="space-y-3">
        {notice && !error && <p className="rounded bg-amber-500/10 px-3 py-2 text-xs text-amber-300">{notice}</p>}
        <Field label="Email">
          <TextInput type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Password">
          <TextInput type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        {error && <p role="alert" className="text-xs text-red-400">{error}</p>}
        <Button type="submit" variant="primary" className="w-full" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </Button>
      </form>
    </Frame>
  )
}

/** One-time creation of the first admin in team mode. */
export function SetupScreen() {
  const setup = useAuthStore((s) => s.setup)
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await setup(email, password, name)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Frame title="Create the admin account" subtitle="Team mode is on and no accounts exist yet. The first account is an administrator who can invite everyone else.">
      <form onSubmit={submit} className="space-y-3">
        <Field label="Your name">
          <TextInput required value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Email">
          <TextInput type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Password" hint="At least 8 characters.">
          <TextInput type="password" autoComplete="new-password" minLength={8} required value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        {error && <p role="alert" className="text-xs text-red-400">{error}</p>}
        <Button type="submit" variant="primary" className="w-full" disabled={busy}>
          {busy ? 'Creating…' : 'Create account'}
        </Button>
      </form>
    </Frame>
  )
}

export function ConnectionError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <Frame title="Can't connect" subtitle={message}>
      <p className="mb-4 text-xs text-slate-500">
        Start it with <code className="text-slate-300">pnpm dev</code> or <code className="text-slate-300">docker compose up</code>, then retry.
      </p>
      <Button variant="primary" onClick={onRetry} className="w-full">Retry</Button>
    </Frame>
  )
}
