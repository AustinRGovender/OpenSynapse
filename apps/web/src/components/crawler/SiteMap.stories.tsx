import type { Meta, StoryObj } from '@storybook/react-vite'
import { useEffect, useState } from 'react'
import type { Crawl } from '@opensynapse/api-client'
import { shopCrawl } from '../../test/fixtures'
import { SiteMap } from './SiteMap'

/** The shop crawl as the poller sees it after `count` pages. */
function partial(count: number): Crawl {
  const nodes = shopCrawl.graph.nodes.slice(0, count)
  const urls = new Set(nodes.map((n) => n.url))
  return {
    ...shopCrawl,
    status: count < shopCrawl.graph.nodes.length ? 'crawling' : 'completed',
    progress: { pages_discovered: nodes.length, requests_captured: nodes.length },
    graph: { nodes, edges: shopCrawl.graph.edges.filter((e) => urls.has(e.source)) },
  }
}

/** A catalogue: categories, paged listings and products. */
function catalogue(): Crawl {
  const base = 'http://shop.example.test'
  const nodes = [{ url: `${base}/`, title: 'Home' }]
  const edges: Crawl['graph']['edges'] = []
  const link = (from: string, to: string, title: string) => {
    nodes.push({ url: base + to, title })
    edges.push({ source: base + from, target: base + to })
  }
  for (const cat of ['laptops', 'phones', 'audio', 'cameras', 'gaming', 'home']) {
    link('/', `/c/${cat}`, cat)
    for (let p = 1; p <= 3; p++) {
      link(`/c/${cat}`, `/c/${cat}?page=${p}`, `${cat}, page ${p}`)
      for (let i = 1; i <= 6; i++) link(`/c/${cat}?page=${p}`, `/p/${cat}-${p}${i}`, `${cat} item ${p}${i}`)
    }
  }
  return { ...shopCrawl, entry_url: `${base}/`, engine: 'colly', graph: { nodes, edges }, requests: [], progress: { pages_discovered: nodes.length, requests_captured: nodes.length } }
}

function Frame({ crawl }: { crawl: Crawl }) {
  return (
    <div className="h-[560px] rounded-lg border border-slate-800">
      <SiteMap crawl={crawl} />
    </div>
  )
}

function Growing() {
  const [count, setCount] = useState(1)
  useEffect(() => {
    if (count >= shopCrawl.graph.nodes.length) return
    const t = setTimeout(() => setCount((c) => c + 2), 1500)
    return () => clearTimeout(t)
  }, [count])
  return <Frame crawl={partial(count)} />
}

const meta = {
  title: 'Crawler/SiteMap',
  component: SiteMap,
  args: { crawl: shopCrawl },
  render: (args) => <Frame crawl={args.crawl} />,
} satisfies Meta<typeof SiteMap>

export default meta
type Story = StoryObj<typeof meta>

/** The test shop after a Rod crawl. The dashed page was reached by a form's redirect; the red one refused the crawler. */
export const Completed: Story = {}

/** Pages arrive as the poller fetches them, and the map refits to keep them in view. */
export const Crawling: Story = { render: () => <Growing /> }

export const Catalogue: Story = { args: { crawl: catalogue() } }

export const NothingYet: Story = { args: { crawl: partial(0) } }
