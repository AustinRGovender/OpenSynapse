/**
 * Plan schema types and validation.
 * Node type JSON schemas will live in schemas/nodes/.
 * This package will export TypeScript types and a validation function.
 */

export interface Node {
  id: string
  type: string
  name: string
  enabled: boolean
  properties: Record<string, unknown>
  children: Node[]
}

export interface Plan {
  id: string
  name: string
  description: string
  tags: string[]
  created_at: string
  updated_at: string
  version: number
  default_environment_id?: string
  root: Node
}
