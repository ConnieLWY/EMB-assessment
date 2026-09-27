import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../lib/api'
import type { Reservation } from '../types/api'

const dateTime = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

interface Props {
  reservation: Reservation
  chargerName: string
  onCancelled: () => void
  onUnauthorized: () => void
}

export function ReservationRow({ reservation, chargerName, onCancelled, onUnauthorized }: Props) {
  const [cancelled, setCancelled] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const status = cancelled ? 'CANCELLED' : reservation.status
  const canCancel = status === 'SCHEDULED' || status === 'WAITING'

  const cancel = async () => {
    setPending(true)
    setError(null)
    const pendingRequest = new AbortController()
    controller.current = pendingRequest
    try {
      await api.cancelReservation(reservation.id, pendingRequest.signal)
      if (pendingRequest.signal.aborted) return
      setCancelled(true)
      onCancelled()
    } catch (cause) {
      if (pendingRequest.signal.aborted) return
      if (cause instanceof ApiError && cause.status === 401) onUnauthorized()
      else setError(cause instanceof Error ? cause.message : 'Could not cancel this reservation.')
    } finally { if (!pendingRequest.signal.aborted) setPending(false) }
  }

  return <li className="reservation-row"><div className="reservation-details"><strong>{chargerName}</strong><span>{dateTime.format(new Date(reservation.start_time))} – {dateTime.format(new Date(reservation.end_time))}</span>{canCancel && <button className="cancel-button" type="button" onClick={cancel} disabled={pending} aria-label={`Cancel reservation for ${chargerName}`}>{pending ? 'Cancelling…' : 'Cancel reservation'}</button>}{error && <span className="cancel-error" role="alert">{error}</span>}</div><span className={`booking-status booking-${status.toLowerCase()}`}>{status.toLowerCase()}</span></li>
}
