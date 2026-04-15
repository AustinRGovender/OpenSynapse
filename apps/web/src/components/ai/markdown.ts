/**
 * Simple markdown-to-HTML converter.
 * Handles: headers, bold, italic, inline code, code blocks, lists, paragraphs.
 * No external dependency required.
 */
export function simpleMarkdown(text: string): string {
  if (!text) return ''

  // Escape HTML entities first
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

  // Code blocks (``` ... ```)
  html = html.replace(/```([\s\S]*?)```/g, (_: string, code: string) => {
    return `<pre class="rounded bg-slate-950 px-3 py-2 font-mono text-xs text-slate-300 overflow-x-auto my-2"><code>${code.trim()}</code></pre>`
  })

  // Inline code
  html = html.replace(/`([^`]+)`/g, '<code class="rounded bg-slate-800 px-1 py-0.5 font-mono text-xs text-teal-400">$1</code>')

  // Headers
  html = html.replace(/^### (.+)$/gm, '<h4 class="text-sm font-semibold text-slate-200 mt-3 mb-1">$1</h4>')
  html = html.replace(/^## (.+)$/gm, '<h3 class="text-sm font-bold text-slate-100 mt-4 mb-1">$1</h3>')
  html = html.replace(/^# (.+)$/gm, '<h2 class="text-base font-bold text-slate-100 mt-4 mb-2">$1</h2>')

  // Bold and italic
  html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong class="text-slate-200">$1</strong>')
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')

  // Unordered lists
  html = html.replace(/^[-*] (.+)$/gm, '<li class="ml-4 list-disc text-slate-300">$1</li>')

  // Ordered lists
  html = html.replace(/^\d+\. (.+)$/gm, '<li class="ml-4 list-decimal text-slate-300">$1</li>')

  // Wrap consecutive <li> in <ul>/<ol>
  html = html.replace(/((?:<li class="ml-4 list-disc[^"]*">[^<]*<\/li>\n?)+)/g, '<ul class="my-2 space-y-0.5">$1</ul>')
  html = html.replace(/((?:<li class="ml-4 list-decimal[^"]*">[^<]*<\/li>\n?)+)/g, '<ol class="my-2 space-y-0.5">$1</ol>')

  // Paragraphs: wrap non-tag lines
  html = html.replace(/^(?!<[a-z])((?!^\s*$).+)$/gm, '<p class="my-1">$1</p>')

  // Clean up extra newlines
  html = html.replace(/\n{2,}/g, '\n')

  return html.trim()
}
