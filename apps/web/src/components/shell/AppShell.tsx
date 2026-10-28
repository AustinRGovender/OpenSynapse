import type { ReactNode } from 'react'
import logoUrl from '../../assets/logo.png'
import { useAuthStore } from '../../stores/auth-store'
import { useUIStore } from '../../stores/ui-store'

const NAV = [
  { href: '#/plans', label: 'Plans', match: ['/plans', '/'] },
  { href: '#/runs', label: 'Runs', match: ['/runs', '/compare'] },
  { href: '#/environments', label: 'Environments', match: ['/environments'] },
  { href: '#/playground', label: 'Playground', match: ['/playground'] },
  { href: '#/crawler', label: 'Crawler', match: ['/crawler'] },
  { href: '#/settings', label: 'Settings', match: ['/settings'] },
]

function isActive(path: string, prefixes: string[]) {
  return prefixes.some((p) => (p === '/' ? path === '/' : path === p || path.startsWith(p + '/')))
}

/** Top navigation shared by every page. */
export function AppShell({ path, children }: { path: string; children: ReactNode }) {
  const user = useAuthStore((s) => s.user)
  const mode = useAuthStore((s) => s.mode)
  const logout = useAuthStore((s) => s.logout)
  const system = useUIStore((s) => s.system)

  return (
    <div className="flex h-full flex-col bg-slate-950 text-slate-100">
      <header className="flex h-12 shrink-0 items-center gap-4 border-b border-slate-800 px-4">
        <a href="#/plans" className="flex items-center gap-2 text-sm font-semibold tracking-tight text-slate-100">
          <img src={logoUrl} alt="" aria-hidden className="h-6 w-6" />
          OpenSynapse
        </a>
        <nav aria-label="Main" className="flex items-center gap-0.5 overflow-x-auto">
          {NAV.map((n) => {
            const active = isActive(path, n.match)
            return (
              <a
                key={n.href}
                href={n.href}
                aria-current={active ? 'page' : undefined}
                className={`whitespace-nowrap rounded px-2.5 py-1 text-xs font-medium ${active ? 'bg-teal-600/20 text-teal-300' : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'}`}
              >
                {n.label}
              </a>
            )
          })}
        </nav>
        <div className="ml-auto flex items-center gap-3 text-xs">
          {system && !system.k6.available && (
            <a href="#/settings/about" className="rounded bg-amber-500/15 px-2 py-0.5 text-amber-300" title="Runs are disabled until k6 is installed">
              k6 not found
            </a>
          )}
          {system?.k6.available && !system.k6.kv && (
            <span className="rounded bg-slate-800 px-2 py-0.5 text-slate-400" title="This k6 build lacks xk6-kv, so live RPS limits are unavailable. The Docker image bundles it.">
              k6 {system.k6.version} · no RPS control
            </span>
          )}
          {user && (
            <span className="text-slate-400">
              {user.display_name}
              {mode === 'team' && <span className="ml-1 rounded bg-slate-800 px-1.5 py-0.5 text-[10px] uppercase text-slate-400">{user.role}</span>}
            </span>
          )}
          {mode === 'team' && (
            <button type="button" onClick={() => logout()} className="rounded px-2 py-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200">
              Sign out
            </button>
          )}
        </div>
      </header>
      <main className="min-h-0 flex-1 overflow-auto">{children}</main>
    </div>
  )
}
