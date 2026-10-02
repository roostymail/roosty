import { describe, expect, it } from 'vitest'
import { currentPath, navigate } from './router'

describe('router', () => {
  it('pushes and replaces history entries', () => {
    const before = history.length
    navigate('/settings')
    expect(currentPath()).toBe('/settings')
    expect(location.pathname).toBe('/settings')
    expect(history.length).toBe(before + 1)
    navigate('/login', true)
    expect(currentPath()).toBe('/login')
    expect(history.length).toBe(before + 1)
  })

  it('follows the back button', () => {
    navigate('/a')
    navigate('/b')
    history.replaceState(null, '', '/a')
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(currentPath()).toBe('/a')
  })
})
