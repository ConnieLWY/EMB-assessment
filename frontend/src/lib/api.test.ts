import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('API client', () => {
  it('sends login and reservations with browser credentials', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ user: { id: 'user-id', username: 'demo' } }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ reservation: { id: 'reservation-id' } }), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)

    await api.login('demo', 'DemoPass123!')
    await api.reserve('charger-1', { user_id: 'user-id', start_time: '2026-10-01T14:00:00Z', end_time: '2026-10-01T15:00:00Z' })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/auth/login', expect.objectContaining({ credentials: 'include', method: 'POST' }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/chargers/charger-1/reserve', expect.objectContaining({ credentials: 'include', method: 'POST' }))
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ user_id: 'user-id', start_time: '2026-10-01T14:00:00Z', end_time: '2026-10-01T15:00:00Z' })
  })

  it('preserves the server conflict code and message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'RESERVATION_CONFLICT', message: 'This charger is already reserved.' } }), { status: 409 })))
    await expect(api.reserve('charger-1', { user_id: 'user-id', start_time: '', end_time: '' })).rejects.toMatchObject({
      status: 409, code: 'RESERVATION_CONFLICT', message: 'This charger is already reserved.',
    } satisfies Partial<ApiError>)
  })

  it('sends cancellation with the current browser session', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ reservation: { id: 'reservation-id', status: 'CANCELLED' } }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const result = await api.cancelReservation('reservation-id')
    expect(result.status).toBe('CANCELLED')
    expect(fetchMock).toHaveBeenCalledWith('/api/reservations/reservation-id/cancel', expect.objectContaining({ credentials: 'include', method: 'POST' }))
  })

  it('requests a reservation page and keeps pagination metadata', async () => {
    const payload = { reservations: [], page: 2, limit: 5, total: 14 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    expect(await api.listReservations('history', 2)).toEqual(payload)
    expect(fetchMock).toHaveBeenCalledWith('/api/reservations?group=history&page=2&limit=5', expect.objectContaining({ credentials: 'include' }))
  })
})
