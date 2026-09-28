import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError, RESERVATION_PAGE_SIZE } from '../lib/api'
import type { ReservationGroup } from '../lib/api'
import type { Reservation } from '../types/api'

export function useReservations(userId: string | null, onUnauthorized: () => void, connectionEpoch: number, group: ReservationGroup = 'upcoming') {
  const [reservations, setReservations] = useState<Reservation[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const refreshRef = useRef<() => void>(() => {})
  const previousEpoch = useRef(connectionEpoch)
  const onUnauthorizedRef = useRef(onUnauthorized)
  onUnauthorizedRef.current = onUnauthorized

  useEffect(() => setPage(1), [userId])

  useEffect(() => {
    let active = true
    let sequence = 0
    let controller: AbortController | null = null
    setReservations([])
    setTotal(0)
    setError(null)
    if (!userId) {
      setLoading(false)
      refreshRef.current = () => {}
      return () => { active = false }
    }
    const refresh = () => {
      controller?.abort()
      controller = new AbortController()
      const currentController = controller
      const currentSequence = ++sequence
      setLoading(true)
      api.listReservations(group, page, currentController.signal).then((result) => {
        if (active && sequence === currentSequence && !currentController.signal.aborted) {
          const lastPage = Math.max(1, Math.ceil(result.total / result.limit))
          if (page > lastPage) { setPage(lastPage); return }
          setReservations(result.reservations)
          setTotal(result.total)
          setError(null)
        }
      }).catch((cause: unknown) => {
        if (!active || sequence !== currentSequence || currentController.signal.aborted) return
        if (cause instanceof ApiError && cause.status === 401) {
          setReservations([])
          onUnauthorizedRef.current()
        } else if (!(cause instanceof DOMException && cause.name === 'AbortError')) {
          setError('Could not load your reservations.')
        }
      }).finally(() => {
        if (active && sequence === currentSequence) setLoading(false)
      })
    }
    refreshRef.current = refresh
    refresh()
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') refresh()
    }, 5000)
    const visible = () => {
      if (document.visibilityState === 'visible') refresh()
    }
    document.addEventListener('visibilitychange', visible)
    return () => {
      active = false
      sequence++
      controller?.abort()
      clearInterval(timer)
      document.removeEventListener('visibilitychange', visible)
      refreshRef.current = () => {}
    }
  }, [userId, page, group])

  useEffect(() => {
    if (connectionEpoch !== previousEpoch.current && userId) refreshRef.current()
    previousEpoch.current = connectionEpoch
  }, [connectionEpoch, userId])

  const refresh = useCallback(() => refreshRef.current(), [])
  const showFirstPage = useCallback(() => {
    if (page === 1) refreshRef.current()
    else setPage(1)
  }, [page])
  return { reservations, loading, error, refresh, page, total, pageCount: Math.max(1, Math.ceil(total / RESERVATION_PAGE_SIZE)), goToPage: setPage, showFirstPage }
}
