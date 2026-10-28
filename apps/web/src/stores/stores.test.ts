import { beforeEach, describe, expect, it } from 'vitest'
import { plan } from '../test/fixtures'
import { findNode, usePlanStore } from './plan-store'
import { emptyAuth, usePlaygroundStore } from './playground-store'

describe('playground request building', () => {
  beforeEach(() => {
    usePlaygroundStore.setState({ method: 'GET', url: '', params: [{ key: '', value: '', enabled: true }], headers: [{ key: '', value: '', enabled: true }], body: '', bodyType: 'none', authType: 'none', auth: { ...emptyAuth }, environmentId: '' })
  })

  it('appends enabled query parameters and skips disabled ones', () => {
    usePlaygroundStore.setState({
      url: '${ECOMMERCE_API}/products?sort=price',
      params: [
        { key: 'page', value: '2', enabled: true },
        { key: 'debug', value: '1', enabled: false },
        { key: 'q', value: 'a b', enabled: true },
      ],
    })
    expect(usePlaygroundStore.getState().buildRequest().url).toBe('${ECOMMERCE_API}/products?sort=price&page=2&q=a%20b')
  })

  it('drops the body for GET and keeps it for POST', () => {
    usePlaygroundStore.setState({ body: '{"a":1}', bodyType: 'json' })
    expect(usePlaygroundStore.getState().buildRequest()).toMatchObject({ body: '', body_type: 'none' })
    usePlaygroundStore.setState({ method: 'POST' })
    expect(usePlaygroundStore.getState().buildRequest()).toMatchObject({ body: '{"a":1}', body_type: 'json' })
  })

  it('sends only the fields of the selected auth method', () => {
    usePlaygroundStore.setState({ authType: 'api_key', auth: { ...emptyAuth, username: 'leak', key: 'X-Key', value: 'k', add_to: 'query' } })
    expect(usePlaygroundStore.getState().buildRequest().auth_config).toEqual({ key: 'X-Key', value: 'k', add_to: 'query' })
    usePlaygroundStore.setState({ authType: 'digest', auth: { ...emptyAuth, username: 'demo', password: 'demo-pass', token: 'leak' } })
    expect(usePlaygroundStore.getState().buildRequest().auth_config).toEqual({ username: 'demo', password: 'demo-pass' })
  })

  it('selects an environment only when one is chosen', () => {
    expect(usePlaygroundStore.getState().buildRequest().environment_id).toBeUndefined()
    usePlaygroundStore.setState({ environmentId: 'env-docker' })
    expect(usePlaygroundStore.getState().buildRequest().environment_id).toBe('env-docker')
  })
})

describe('plan editing', () => {
  beforeEach(() => {
    usePlanStore.setState({ plan: structuredClone(plan), selectedNodeId: null, undoStack: [], redoStack: [], dirty: false })
  })

  it('moves a node, then undoes and redoes the move', () => {
    const { moveNode } = usePlanStore.getState()
    moveNode('think', 'tx', 0)
    expect(findNode(usePlanStore.getState().plan!.root, 'tx')!.children[0].id).toBe('think')
    expect(usePlanStore.getState().dirty).toBe(true)

    usePlanStore.getState().undo()
    expect(findNode(usePlanStore.getState().plan!.root, 'tx')!.children.map((c) => c.id)).toEqual(['list', 'order', 'think'])

    usePlanStore.getState().redo()
    expect(findNode(usePlanStore.getState().plan!.root, 'tx')!.children[0].id).toBe('think')
  })

  it('will not move a node inside its own subtree', () => {
    usePlanStore.getState().moveNode('tx', 'list', 0)
    expect(findNode(usePlanStore.getState().plan!.root, 'scenario')!.children[0].id).toBe('tx')
  })

  it('duplicates a subtree with fresh ids', () => {
    usePlanStore.getState().duplicateNode('list')
    const children = findNode(usePlanStore.getState().plan!.root, 'tx')!.children
    expect(children).toHaveLength(4)
    expect(children[1].name).toMatch(/GET \/products/)
    expect(children[1].id).not.toBe('list')
    expect(children[1].children[0].id).not.toBe('a1')
  })
})
