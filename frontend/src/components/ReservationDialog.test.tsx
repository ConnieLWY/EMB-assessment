import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../lib/api'
import type { Charger, Reservation, User } from '../types/api'
import { ReservationDialog } from './ReservationDialog'

const user: User = { id: '11111111-1111-4111-8111-111111111111', username: 'demo' }
const charger: Charger = { id: 'charger-1', name: 'Charger 1', location: 'Level 1, Bay A', status: 'AVAILABLE', updated_at: '2026-09-27T10:00:00Z' }
const reservation: Reservation = { id: 'reservation-1', user_id: user.id, charger_id: charger.id, start_time: '', end_time: '', status: 'SCHEDULED', created_at: '', updated_at: '' }

afterEach(() => vi.restoreAllMocks())

describe('ReservationDialog', () => {
  it('prefills an editable user ID and reports a successful reservation', async () => {
    const reserve = vi.spyOn(api, 'reserve').mockResolvedValue(reservation)
    const onSuccess = vi.fn()
    render(<ReservationDialog charger={charger} user={user} onClose={vi.fn()} onSuccess={onSuccess} onUnauthorized={vi.fn()} />)
    expect(screen.getByLabelText('User ID')).toHaveValue(user.id)
    const tomorrow = new Date(Date.now() + 86_400_000)
    const start = `${tomorrow.getFullYear()}-${String(tomorrow.getMonth() + 1).padStart(2, '0')}-${String(tomorrow.getDate()).padStart(2, '0')}T14:00`
    const end = start.replace('T14:00', 'T15:00')
    await userEvent.clear(screen.getByLabelText('User ID'))
    await userEvent.type(screen.getByLabelText('User ID'), user.id)
    await userEvent.type(screen.getByLabelText('Start time'), start)
    await userEvent.type(screen.getByLabelText('End time'), end)
    await userEvent.click(screen.getByRole('button', { name: 'Confirm reservation' }))
    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith(reservation))
    expect(reserve).toHaveBeenCalledWith(charger.id, { user_id: user.id, start_time: new Date(start).toISOString(), end_time: new Date(end).toISOString() })
  })

  it('shows an explicit conflict and keeps the form open', async () => {
    vi.spyOn(api, 'reserve').mockRejectedValue(new ApiError(409, 'RESERVATION_CONFLICT', 'This charger is already reserved for the selected time slot.'))
    render(<ReservationDialog charger={charger} user={user} onClose={vi.fn()} onSuccess={vi.fn()} onUnauthorized={vi.fn()} />)
    const tomorrow = new Date(Date.now() + 86_400_000)
    const day = `${tomorrow.getFullYear()}-${String(tomorrow.getMonth() + 1).padStart(2, '0')}-${String(tomorrow.getDate()).padStart(2, '0')}`
    await userEvent.type(screen.getByLabelText('Start time'), `${day}T14:00`)
    await userEvent.type(screen.getByLabelText('End time'), `${day}T15:00`)
    await userEvent.click(screen.getByRole('button', { name: 'Confirm reservation' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Reservation conflict:')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('rejects an end time before the start time without sending a request', async () => {
    const reserve = vi.spyOn(api, 'reserve')
    render(<ReservationDialog charger={charger} user={user} onClose={vi.fn()} onSuccess={vi.fn()} onUnauthorized={vi.fn()} />)
    const tomorrow = new Date(Date.now() + 86_400_000)
    const day = `${tomorrow.getFullYear()}-${String(tomorrow.getMonth() + 1).padStart(2, '0')}-${String(tomorrow.getDate()).padStart(2, '0')}`
    await userEvent.type(screen.getByLabelText('Start time'), `${day}T15:00`)
    await userEvent.type(screen.getByLabelText('End time'), `${day}T14:00`)
    await userEvent.click(screen.getByRole('button', { name: 'Confirm reservation' }))
    expect(screen.getByRole('alert')).toHaveTextContent('End time must be after start time.')
    expect(reserve).not.toHaveBeenCalled()
  })
})
