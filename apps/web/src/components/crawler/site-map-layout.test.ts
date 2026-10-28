import { describe, expect, it } from 'vitest'
import { shopCrawl } from '../../test/fixtures'
import { foldDepth, layoutSiteMap, NODE_H, type SitePage, siteForest } from './site-map-layout'

const shop = 'http://mock-ecommerce:9002/shop'
const paths = (pages: SitePage[]) => pages.map((p) => p.url.slice(shop.length))

describe('siteForest', () => {
  const [home, ...orphans] = siteForest(shopCrawl)

  it('hangs each page off the first page that linked to it, at its shortest link distance', () => {
    expect(home.url).toBe(shop)
    expect(paths(home.children)).toEqual(['/products', '/cart', '/app', '/about', '/login', '/staff'])
    const products = home.children[0]
    expect(paths(products.children)).toEqual(['/products/p1', '/products/p2', '/products/p3', '/products/p4', '/products?page=2'])
    expect(paths(products.children[4].children)).toEqual(['/products/p5'])
    expect(products.children[4].children[0].depth).toBe(3)
    // Deals links to a product the product list reached first.
    expect(home.children[2].children).toEqual([])
  })

  it('drops back-links and links to pages the crawler did not visit', () => {
    const all: SitePage[] = []
    const walk = (p: SitePage) => (all.push(p), p.children.forEach(walk))
    walk(home)
    orphans.forEach(walk)
    expect(all).toHaveLength(shopCrawl.graph.nodes.length)
    expect(all.some((p) => p.url.endsWith('/account/delete'))).toBe(false)
  })

  it('makes pages no link reached into roots of their own', () => {
    expect(paths(orphans)).toEqual(['/orders/o-1001'])
    expect(orphans[0].orphan).toBe(true)
    expect(home.orphan).toBe(false)
  })

  it('carries titles and GET status codes', () => {
    expect(home.title).toBe('Home · Synapse Shop')
    expect(home.status).toBe(200)
    expect(home.children[5].status).toBe(401)
  })

  it('hides titles that only repeat the URL', () => {
    const crawl = { ...shopCrawl, graph: { nodes: [{ url: shop, title: shop }], edges: [] }, requests: [] }
    expect(siteForest(crawl)[0]).toMatchObject({ title: '', status: undefined })
  })

  it('starts from the first page when the entry URL redirected elsewhere', () => {
    expect(siteForest({ ...shopCrawl, entry_url: 'http://mock-ecommerce:9002/' })[0].url).toBe(shop)
  })
})

describe('layoutSiteMap', () => {
  const { pages, links, depth } = layoutSiteMap(siteForest(shopCrawl))
  const at = new Map(pages.map((p) => [p.page.url, p]))

  it('places one column per link depth, with one link into every page but the roots', () => {
    expect(depth).toBe(3)
    expect(new Set(pages.map((p) => p.x)).size).toBe(4)
    for (const p of pages) expect(p.x).toBe(at.get(shop)!.x + p.page.depth * (at.get(`${shop}/cart`)!.x - at.get(shop)!.x))
    expect(links).toHaveLength(pages.length - 2)
    expect(links.find((l) => l.target === `${shop}/products/p5`)).toEqual({ source: `${shop}/products?page=2`, target: `${shop}/products/p5`, depth: 3 })
  })

  it('centres each page beside its children', () => {
    const products = at.get(`${shop}/products`)!
    const kids = products.page.children.map((c) => at.get(c.url)!.y)
    expect(products.y).toBeCloseTo((Math.min(...kids) + Math.max(...kids)) / 2)
  })

  it('never overlaps pages in a column, and stacks separate trees below', () => {
    for (const x of new Set(pages.map((p) => p.x))) {
      const ys = pages.filter((p) => p.x === x).map((p) => p.y).sort((a, b) => a - b)
      for (let i = 1; i < ys.length; i++) expect(ys[i] - ys[i - 1]).toBeGreaterThanOrEqual(NODE_H)
    }
    const order = at.get(`${shop}/orders/o-1001`)!
    expect(order.x).toBe(at.get(shop)!.x)
    expect(order.y).toBeGreaterThan(Math.max(...pages.filter((p) => p !== order).map((p) => p.y)))
  })

  it('handles an empty crawl', () => {
    expect(layoutSiteMap([])).toEqual({ pages: [], links: [], depth: 0 })
  })

  it('leaves out folded branches and counts the pages they hold', () => {
    const folded = layoutSiteMap(siteForest(shopCrawl), (p) => p.depth < 1)
    expect(folded.pages).toHaveLength(8)
    expect(folded.links).toHaveLength(6)
    expect(folded.depth).toBe(1)
    const hidden = new Map(folded.pages.map((p) => [p.page.url.slice(shop.length), p.hidden]))
    expect(hidden.get('')).toBe(0)
    expect(hidden.get('/products')).toBe(6)
    expect(hidden.get('/login')).toBe(1)
    expect(hidden.get('/cart')).toBe(0)
  })
})

describe('foldDepth', () => {
  const forest = siteForest(shopCrawl)

  it('counts every page below each page', () => {
    expect(forest[0].descendants).toBe(shopCrawl.graph.nodes.length - 2)
    expect(forest[1].descendants).toBe(0)
  })

  it('shows whole link levels while they fit the budget', () => {
    // Pages per level, counting the orphan as a root: 2, 6, 6, 1.
    expect(foldDepth(forest, 40)).toBe(3)
    expect(foldDepth(forest, 14)).toBe(2)
    expect(foldDepth(forest, 13)).toBe(1)
  })

  it('always shows the start page’s own links', () => {
    expect(foldDepth(forest, 1)).toBe(1)
  })
})
