import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../lib/api'
import type { Reservation } from '../types/api'

export function useReservations(userId: string | null, onUnauthorized: () => void, connectionEpoch: number) {
  const [reservations, setReservations] = useState<Reservation[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const refreshRef = useRef<() => void>(() => {})
  const previousEpoch = useRef(connectionEpoch)
  const onUnauthorizedRef = useRef(onUnauthorized)
  onUnauthorizedRef.current = onUnauthorized

  useEffect(() => {
    let active = true
    let sequence = 0
    let controller: AbortController | null = null
    setReservations([])
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
      api.listReservations(currentController.signal).then((items) => {
        if (active && sequence === currentSequence && !currentController.signal.aborted) {
          setReservations(items)
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
  }, [userId])

  useEffect(() => {
    if (connectionEpoch !== previousEpoch.current && userId) refreshRef.current()
    previousEpoch.current = connectionEpoch
  }, [connectionEpoch, userId])

  const refresh = useCallback(() => refreshRef.current(), [])
  return { reservations, loading, error, refresh }
}
