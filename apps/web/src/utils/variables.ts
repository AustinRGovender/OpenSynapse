import type { Node } from '@opensynapse/api-client'

const REF = /\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g
const CODE_SET = /__vars(?:\.([A-Za-z_][A-Za-z0-9_]*)|\[\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]\s*\])\s*=/g

/**
 * The ${NAME} variables an enabled part of the plan uses but does not set
 * itself (code blocks can assign __vars.NAME), so an environment must supply them.
 */
export function requiredVariables(root: Node): string[] {
  const used = new Set<string>()
  const defined = new Set<string>()
  const walk = (n: Node) => {
    if (!n.enabled) return
    const props = JSON.stringify(n.properties ?? {})
    for (const m of props.matchAll(REF)) used.add(m[1])
    if (n.type === 'code-block') {
      const code = String((n.properties as { code?: string })?.code ?? '')
      for (const m of code.matchAll(CODE_SET)) defined.add(m[1] ?? m[2])
    }
    n.children.forEach(walk)
  }
  walk(root)
  return [...used].filter((v) => !defined.has(v)).sort()
}
