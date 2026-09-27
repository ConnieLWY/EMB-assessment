import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../lib/api'
import type { Reservation } from '../types/api'
import { useReservations } from './useReservations'

const reservation: Reservation = {
  id: 'reservation-1', user_id: 'user-1', charger_id: 'charger-1', status: 'SCHEDULED',
  start_time: '2026-10-01T14:00:00Z', end_time: '2026-10-01T15:00:00Z',
  created_at: '2026-09-27T10:00:00Z', updated_at: '2026-09-27T10:00:00Z',
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => { resolve = res })
  return { promise, resolve }
}

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

describe('useReservations', () => {
  it('clears private data on logout and rejects a late response', async () => {
    const pending = deferred<Reservation[]>()
    vi.spyOn(api, 'listReservations').mockReturnValueOnce(Promise.resolve([reservation])).mockReturnValueOnce(pending.promise)
    const onUnauthorized = vi.fn()
    const { result, rerender } = renderHook(({ userId }) => useReservations(userId, onUnauthorized, 0), { initialProps: { userId: 'user-1' as string | null } })
    await waitFor(() => expect(result.current.reservations).toHaveLength(1))
    act(() => result.current.refresh())
    rerender({ userId: null })
    expect(result.current.reservations).toEqual([])
    await act(async () => pending.resolve([reservation]))
    expect(result.current.reservations).toEqual([])
  })

  it('clears a signed-in user after a 401', async () => {
    vi.spyOn(api, 'listReservations').mockRejectedValue(new ApiError(401, 'UNAUTHENTICATED', 'Sign in to continue.'))
    const onUnauthorized = vi.fn()
    const { result } = renderHook(() => useReservations('user-1', onUnauthorized, 0))
    await waitFor(() => expect(onUnauthorized).toHaveBeenCalledOnce())
    expect(result.current.reservations).toEqual([])
  })

  it('refreshes after a live-feed reconnect without duplicating the login fetch', async () => {
    const list = vi.spyOn(api, 'listReservations').mockResolvedValue([])
    const onUnauthorized = vi.fn()
    const { rerender } = renderHook(({ userId, epoch }) => useReservations(userId, onUnauthorized, epoch), {
      initialProps: { userId: null as string | null, epoch: 1 },
    })
    rerender({ userId: 'user-1', epoch: 1 })
    await waitFor(() => expect(list).toHaveBeenCalledTimes(1))
    rerender({ userId: 'user-1', epoch: 2 })
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2))
  })

  it('pauses polling while hidden and refreshes when visible again', async () => {
    vi.useFakeTimers()
    let hidden = false
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => hidden ? 'hidden' : 'visible' })
    const list = vi.spyOn(api, 'listReservations').mockResolvedValue([])
    const onUnauthorized = vi.fn()
    renderHook(() => useReservations('user-1', onUnauthorized, 0))
    await act(async () => Promise.resolve())
    expect(list).toHaveBeenCalledTimes(1)
    hidden = true
    await act(async () => vi.advanceTimersByTimeAsync(10_000))
    expect(list).toHaveBeenCalledTimes(1)
    hidden = false
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    expect(list).toHaveBeenCalledTimes(2)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  })
})
