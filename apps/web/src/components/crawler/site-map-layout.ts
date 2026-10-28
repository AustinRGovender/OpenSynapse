import type { Crawl } from '@opensynapse/api-client'

export const NODE_W = 220
export const NODE_H = 50
const COL_GAP = 64
const ROW_GAP = 12
const TREE_GAP = 48

export interface SitePage {
  url: string
  title: string
  depth: number
  /** Status of the page's GET, when the crawler captured it. */
  status?: number
  /** Not linked from the start page: reached by a redirect or a form. */
  orphan: boolean
  children: SitePage[]
  /** Pages below this one in the tree. */
  descendants: number
}

export interface PlacedPage {
  page: SitePage
  x: number
  y: number
  /** Pages folded away below this one; 0 when its children are shown. */
  hidden: number
}

export interface PlacedLink {
  source: string
  target: string
  depth: number
}

export function pathOf(url: string) {
  try {
    const u = new URL(url)
    return u.pathname + u.search
  } catch {
    return url
  }
}

/**
 * Pages as a forest, breadth-first from the entry page along discovered links,
 * so each page hangs off the first page that linked to it and sits at its
 * shortest link distance. Back-links and repeat links are dropped; pages no
 * link reached become roots of their own.
 */
export function siteForest(crawl: Crawl): SitePage[] {
  const nodes = crawl.graph.nodes
  if (!nodes.length) return []
  const titles = new Map(nodes.map((n) => [n.url, n.title]))
  const links = new Map<string, string[]>()
  for (const e of crawl.graph.edges) {
    const out = links.get(e.source)
    if (out) out.push(e.target)
    else links.set(e.source, [e.target])
  }
  const status = new Map<string, number>()
  for (const r of crawl.requests) if (r.method === 'GET' && !status.has(r.url)) status.set(r.url, r.status_code)

  const seen = new Set<string>()
  const page = (url: string, depth: number, orphan: boolean): SitePage => {
    seen.add(url)
    // Colly has no page titles and records the URL instead.
    const title = titles.get(url) ?? ''
    return { url, title: title === url ? '' : title, depth, status: status.get(url), orphan, children: [], descendants: 0 }
  }
  const grow = (url: string, orphan: boolean) => {
    const root = page(url, 0, orphan)
    const queue = [root]
    for (let i = 0; i < queue.length; i++) {
      const from = queue[i]
      for (const to of links.get(from.url) ?? []) {
        if (seen.has(to) || !titles.has(to)) continue
        const child = page(to, from.depth + 1, false)
        from.children.push(child)
        queue.push(child)
      }
    }
    // Deepest first, so every page's children are counted before it.
    for (let i = queue.length - 1; i >= 0; i--) queue[i].descendants = queue[i].children.reduce((s, c) => s + 1 + c.descendants, 0)
    return root
  }

  const entry = titles.has(crawl.entry_url) ? crawl.entry_url : nodes[0].url
  const forest = [grow(entry, false)]
  for (const n of nodes) if (!seen.has(n.url)) forest.push(grow(n.url, true))
  return forest
}

/**
 * The deepest level to show on first sight: whole link levels are added while
 * the map stays within `budget` pages, so a large crawl opens readable.
 * Pages at this depth or deeper start folded.
 */
export function foldDepth(forest: SitePage[], budget: number): number {
  const perDepth: number[] = []
  const count = (p: SitePage) => {
    perDepth[p.depth] = (perDepth[p.depth] ?? 0) + 1
    p.children.forEach(count)
  }
  forest.forEach(count)
  let total = 0
  let fold = 1
  for (let d = 0; d < perDepth.length; d++) {
    total += perDepth[d]
    if (total > budget) break
    fold = Math.max(1, d)
  }
  return fold
}

/**
 * Left-to-right tidy tree: one column per link depth, every subtree reserves
 * the height of its shown children, and each page sits centred beside them.
 * Separate trees stack below the entry page's.
 */
export function layoutSiteMap(forest: SitePage[], open: (p: SitePage) => boolean = () => true): { pages: PlacedPage[]; links: PlacedLink[]; depth: number } {
  const pages: PlacedPage[] = []
  const links: PlacedLink[] = []
  const height = new Map<SitePage, number>()
  let depth = 0

  const shown = (p: SitePage) => (open(p) ? p.children : [])
  const span = (kids: SitePage[]) => kids.reduce((s, c) => s + height.get(c)!, 0) + ROW_GAP * Math.max(0, kids.length - 1)
  function measure(p: SitePage): number {
    const kids = shown(p)
    for (const c of kids) measure(c)
    const h = Math.max(NODE_H, span(kids))
    height.set(p, h)
    return h
  }

  function place(p: SitePage, top: number, parent?: SitePage) {
    const kids = shown(p)
    const h = height.get(p)!
    depth = Math.max(depth, p.depth)
    pages.push({ page: p, x: p.depth * (NODE_W + COL_GAP), y: top + h / 2 - NODE_H / 2, hidden: kids.length ? 0 : p.descendants })
    if (parent) links.push({ source: parent.url, target: p.url, depth: p.depth })
    let y = top + (h - span(kids)) / 2
    for (const c of kids) {
      place(c, y, p)
      y += height.get(c)! + ROW_GAP
    }
  }

  let top = 0
  for (const root of forest) {
    const h = measure(root)
    place(root, top)
    top += h + TREE_GAP
  }
  return { pages, links, depth }
}
