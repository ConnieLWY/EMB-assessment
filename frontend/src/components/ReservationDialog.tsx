import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { localToUtc } from '../lib/time'
import type { Charger, Reservation, User } from '../types/api'

interface Props {
  charger: Charger
  user: User
  onClose: () => void
  onSuccess: (reservation: Reservation) => void
  onUnauthorized: () => void
}

export function ReservationDialog({ charger, user, onClose, onSuccess, onUnauthorized }: Props) {
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const dialog = useRef<HTMLDivElement>(null)
  const firstField = useRef<HTMLInputElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    firstField.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); onCloseRef.current() }
      if (event.key !== 'Tab' || !dialog.current) return
      const focusable = [...dialog.current.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled])')]
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => { document.removeEventListener('keydown', onKeyDown); previous?.focus() }
  }, [])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setError(null)
    try {
      const startTime = localToUtc(start)
      const endTime = localToUtc(end)
      if (new Date(startTime).getTime() < Date.now()) throw new Error('Start time must be in the future.')
      if (endTime <= startTime) throw new Error('End time must be after start time.')
      setSubmitting(true)
      const reservation = await api.reserve(charger.id, { user_id: user.id, start_time: startTime, end_time: endTime })
      onSuccess(reservation)
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) { onUnauthorized(); onClose(); return }
      if (cause instanceof ApiError && cause.status === 409) setError(`Reservation conflict: ${cause.message}`)
      else setError(cause instanceof Error ? cause.message : 'Could not create the reservation.')
    } finally { setSubmitting(false) }
  }

  return <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <div className="reservation-dialog" ref={dialog} role="dialog" aria-modal="true" aria-labelledby="reservation-title">
      <div className="dialog-topline"><span className="eyebrow">New reservation</span><button className="icon-button" type="button" onClick={onClose} aria-label="Close reservation form">×</button></div>
      <h2 id="reservation-title">Reserve {charger.name}</h2>
      <p className="dialog-subtitle">{charger.location}. Times are shown in your local time zone.</p>
      <form onSubmit={submit}>
        <label htmlFor="reservation-user">User ID</label>
        <input id="reservation-user" ref={firstField} value={user.id} readOnly />
        <div className="field-pair">
          <div><label htmlFor="reservation-start">Start time</label><input id="reservation-start" type="datetime-local" value={start} onChange={(event) => setStart(event.target.value)} required /></div>
          <div><label htmlFor="reservation-end">End time</label><input id="reservation-end" type="datetime-local" value={end} onChange={(event) => setEnd(event.target.value)} required /></div>
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
        <div className="dialog-actions"><button type="button" className="button-quiet" onClick={onClose}>Cancel</button><button type="submit" className="button-primary" disabled={submitting}>{submitting ? 'Reserving…' : 'Confirm reservation'}</button></div>
      </form>
    </div>
  </div>
}
