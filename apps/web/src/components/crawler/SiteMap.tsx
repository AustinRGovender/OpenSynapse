import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import {
  Background,
  BackgroundVariant,
  BaseEdge,
  Controls,
  type Edge,
  type EdgeProps,
  getSmoothStepPath,
  Handle,
  MarkerType,
  type Node,
  type NodeProps,
  Panel,
  Position,
  ReactFlow,
  useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { Crawl } from '@opensynapse/api-client'
import { isActive } from '../../stores/crawler-store'
import { foldDepth, layoutSiteMap, NODE_H, NODE_W, pathOf, type SitePage, siteForest } from './site-map-layout'

// Entrance: each link depth arrives one step after the one before it; every
// single transition stays within the 400ms motion budget.
const STEP_MS = 140
const MAX_STEPS = 8
const steps = (depth: number, from: number) => Math.min(Math.max(depth - from, 0), MAX_STEPS)
// The pulse leaves the start page, crosses one level of links per beat, then
// rests: the crawler's breadth-first walk, replayed.
const BEAT_MS = 1100
const REST_MS = 900
const MAX_BEATS = 6
/** Pulse length as a fraction of its link. */
const PULSE = 0.16
/** Pages shown before deeper levels start folded. */
const BUDGET = 40
// Large maps open at a readable zoom around the start page rather than shrunk to fit.
const FIT = { padding: 0.15, minZoom: 0.5, maxZoom: 1.1 }

type PageData = { page: SitePage; hidden: number; trail: boolean; delay: number }
type PageNode = Node<PageData, 'page'>
type LinkData = { depth: number; beats: number; trail: boolean; delay: number; motion: boolean }
type LinkEdge = Edge<LinkData, 'link'>

const ToggleFold = createContext<(url: string) => void>(() => {})

const hiddenHandle = { opacity: 0, border: 0, width: 1, height: 1, minWidth: 0, minHeight: 0, pointerEvents: 'none' } as const

function PageCard({ data: { page, hidden, trail, delay } }: NodeProps<PageNode>) {
  const toggle = useContext(ToggleFold)
  const path = pathOf(page.url)
  const entry = page.depth === 0 && !page.orphan
  const failed = (page.status ?? 0) >= 400
  const border = trail || entry ? 'border-teal-500' : failed ? 'border-red-500/60' : 'border-slate-700'
  return (
    <div
      className={`sitemap-node group relative flex h-full w-full flex-col justify-center rounded-md border bg-slate-900 px-3 text-xs ${border} ${page.orphan ? 'border-dashed' : ''} ${trail ? 'ring-2 ring-teal-500/30' : ''}`}
      style={{ animationDelay: `${delay}ms` }}
      title={page.orphan ? `${page.url}\nNo link led here: the crawler arrived through a redirect or a form.` : page.url}
    >
      <Handle type="target" position={Position.Left} isConnectable={false} style={hiddenHandle} />
      <div className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-mono text-slate-100">{path}</span>
        {entry && <span className="text-[10px] font-medium uppercase tracking-wide text-teal-400">Start</span>}
        {page.status ? <span className={`font-mono text-[10px] ${failed ? 'text-red-400' : 'text-slate-500'}`}>{page.status}</span> : null}
      </div>
      {page.title && <div className="truncate text-[11px] text-slate-500">{page.title}</div>}
      <Handle type="source" position={Position.Right} isConnectable={false} style={hiddenHandle} />
      {page.children.length > 0 && (
        <button
          type="button"
          onClick={() => toggle(page.url)}
          aria-expanded={!hidden}
          aria-label={hidden ? `Show ${hidden} more pages under ${path}` : `Hide the pages under ${path}`}
          className={`nodrag nopan absolute right-0 top-1/2 flex h-5 min-w-5 -translate-y-1/2 translate-x-1/2 items-center justify-center rounded-full border border-slate-600 bg-slate-800 px-1 font-mono text-[10px] text-slate-300 hover:border-teal-500 hover:text-teal-300 ${hidden ? '' : 'opacity-0 focus-visible:opacity-100 group-hover:opacity-100'}`}
        >
          {hidden ? `+${hidden}` : '−'}
        </button>
      )}
    </div>
  )
}

function LinkPath({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, markerEnd, data }: EdgeProps<LinkEdge>) {
  const [path] = getSmoothStepPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition, borderRadius: 10 })
  const pulse = useRef<SVGPathElement>(null)
  const { depth = 1, beats = 1, trail = false, delay = 0, motion = false } = data ?? {}

  useEffect(() => {
    const el = pulse.current
    if (!motion || !el || typeof el.animate !== 'function') return
    const period = beats * BEAT_MS + REST_MS
    const beat = (depth - 1) % beats
    const anim = el.animate(
      [
        { strokeDashoffset: PULSE, offset: 0 },
        { strokeDashoffset: PULSE, offset: (beat * BEAT_MS) / period },
        { strokeDashoffset: -1, offset: ((beat + 1) * BEAT_MS) / period },
        { strokeDashoffset: -1, offset: 1 },
      ],
      { duration: period, iterations: Infinity },
    )
    // Each link is its own <svg>. Pinning every pulse to the page's clock keeps
    // links drawn later (as the crawl grows, or on scrolling into view) in step.
    anim.startTime = 0
    return () => anim.cancel()
  }, [motion, depth, beats])

  return (
    <g className="sitemap-link" style={{ animationDelay: `${delay}ms` }}>
      <BaseEdge id={id} path={path} markerEnd={markerEnd} pathLength={1} style={{ stroke: trail ? 'var(--color-teal-400)' : 'var(--color-slate-600)', strokeWidth: trail ? 2 : 1.25 }} />
      <path ref={pulse} d={path} pathLength={1} className="sitemap-pulse" strokeDasharray={`${PULSE} 2`} strokeDashoffset={PULSE} />
    </g>
  )
}

const nodeTypes = { page: PageCard }
const edgeTypes = { link: LinkPath }

const REDUCED_MOTION = '(prefers-reduced-motion: reduce)'
function subscribeMotion(onChange: () => void) {
  const query = window.matchMedia(REDUCED_MOTION)
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}
const prefersMotion = () => !window.matchMedia(REDUCED_MOTION).matches

/** Keeps the map in view as a running crawl adds pages. */
function FitOnGrow({ count, motion }: { count: number; motion: boolean }) {
  const { fitView } = useReactFlow()
  useEffect(() => {
    void fitView({ ...FIT, duration: motion ? 250 : 0 })
  }, [count, motion, fitView])
  return null
}

/**
 * The crawled site as a flow chart: one column per link depth from the start
 * page, each page linked from the page that first led to it. Pages cascade in
 * by depth, and a pulse retraces the crawl along the links. Large maps open
 * folded to the levels that fit; each page's button shows or hides its branch.
 */
export function SiteMap({ crawl }: { crawl: Crawl }) {
  const motion = useSyncExternalStore(subscribeMotion, prefersMotion, () => false)
  const [focus, setFocus] = useState<string | null>(null)
  // Pages whose fold the user flipped, and the page to hold still on screen
  // (with the depth its branch grows from) while the map reflows around it.
  const [flipped, setFlipped] = useState<ReadonlySet<string>>(() => new Set())
  const [anchor, setAnchor] = useState<{ url: string; y: number; depth: number } | null>(null)

  const forest = useMemo(() => siteForest(crawl), [crawl])
  const fold = useMemo(() => foldDepth(forest, BUDGET), [forest])
  const { pages, links, depth } = useMemo(() => layoutSiteMap(forest, (p) => (p.depth < fold) !== flipped.has(p.url)), [forest, fold, flipped])
  const shift = useMemo(() => {
    const held = anchor && pages.find((p) => p.page.url === anchor.url)
    return held ? anchor.y - held.y : 0
  }, [anchor, pages])

  const toggle = useCallback(
    (url: string) => {
      const at = pages.find((p) => p.page.url === url)
      if (at) setAnchor({ url, y: at.y + shift, depth: at.page.depth })
      setFlipped((f) => {
        const next = new Set(f)
        if (!next.delete(url)) next.add(url)
        return next
      })
    },
    [pages, shift],
  )

  // The route from the start page to the page under the pointer.
  const trail = useMemo(() => {
    const parent = new Map(links.map((l) => [l.target, l.source]))
    const on = new Set<string>()
    for (let url = focus; url; url = parent.get(url) ?? null) on.add(url)
    return on
  }, [links, focus])

  // Entrances cascade from the start page, or from the page just unfolded.
  const base = anchor?.depth ?? 0

  const nodes = useMemo<PageNode[]>(
    () =>
      pages.map(({ page, x, y, hidden }) => ({
        id: page.url,
        type: 'page',
        position: { x, y: y + shift },
        width: NODE_W,
        height: NODE_H,
        draggable: false,
        connectable: false,
        data: { page, hidden, trail: trail.has(page.url), delay: steps(page.depth, base) * STEP_MS },
      })),
    [pages, shift, trail, base],
  )

  const edges = useMemo<LinkEdge[]>(() => {
    const beats = Math.max(1, Math.min(depth, MAX_BEATS))
    return links.map((l) => {
      const on = trail.has(l.target)
      const color = on ? 'var(--color-teal-400)' : 'var(--color-slate-600)'
      return {
        id: `${l.source} -> ${l.target}`,
        source: l.source,
        target: l.target,
        type: 'link',
        zIndex: on ? 1 : 0,
        markerEnd: { type: MarkerType.ArrowClosed, color, width: 22, height: 22, markerUnits: 'userSpaceOnUse' },
        // A link starts drawing half a step before the page it leads to appears.
        data: { depth: l.depth, beats, trail: on, motion, delay: Math.max(steps(l.depth, base) - 0.5, 0) * STEP_MS },
      }
    })
  }, [links, depth, trail, motion, base])

  if (!pages.length) {
    return <p className="p-6 text-center text-xs text-slate-500">{isActive(crawl) ? 'Pages appear here as the crawler finds them.' : 'No pages were found.'}</p>
  }

  return (
    <ToggleFold.Provider value={toggle}>
      <div className="h-full w-full">
        <ReactFlow<PageNode, LinkEdge>
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodeMouseEnter={(_, n) => setFocus(n.id)}
          onNodeMouseLeave={() => setFocus(null)}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
          onlyRenderVisibleElements={pages.length > 200}
          panOnScroll
          fitView
          fitViewOptions={FIT}
          minZoom={0.05}
          proOptions={{ hideAttribution: true }}
        >
          <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="var(--color-slate-800)" />
          <Controls showInteractive={false} />
          <Panel position="top-right" className="rounded bg-slate-950/80 px-2 py-1 text-[11px] text-slate-500">Point at a page to trace how the crawler reached it</Panel>
          <FitOnGrow count={crawl.graph.nodes.length} motion={motion} />
        </ReactFlow>
      </div>
    </ToggleFold.Provider>
  )
}
