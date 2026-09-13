import { describe, expect, it } from 'vitest'
import { buildApiParams } from '../utils'

describe('buildApiParams time range fallback', () => {
  it('should fall back to today start and end when no time params provided', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams: {},
      isAdmin: true,
    })

    expect(params.start_timestamp).toBeDefined()
    expect(params.end_timestamp).toBeDefined()
    expect(params.start_timestamp).toBeGreaterThan(0)
    expect(params.end_timestamp).toBeGreaterThan(params.start_timestamp!)
  })

  it('should maintain default start_timestamp when only endTime is present (e.g. autoRefresh)', () => {
    const nowMs = Date.now()
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams: { endTime: nowMs + 3600 * 1000 },
      isAdmin: true,
    })

    expect(params.start_timestamp).toBeDefined()
    expect(params.start_timestamp).toBeGreaterThan(0)
    expect(params.end_timestamp).toBe(Math.floor((nowMs + 3600 * 1000) / 1000))
    expect(params.start_timestamp).toBeLessThan(params.end_timestamp!)
  })

  it('should maintain default end_timestamp when only startTime is present', () => {
    const startTimeMs = Date.now() - 3600 * 1000
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams: { startTime: startTimeMs },
      isAdmin: true,
    })

    expect(params.start_timestamp).toBe(Math.floor(startTimeMs / 1000))
    expect(params.end_timestamp).toBeDefined()
    expect(params.end_timestamp).toBeGreaterThan(params.start_timestamp!)
  })
})
