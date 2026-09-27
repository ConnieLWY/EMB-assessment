import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../lib/api'
import type { Reservation } from '../types/api'
import { ReservationRow } from './ReservationRow'

const scheduled: Reservation = {
  id: '11111111-1111-4111-8111-111111111111', user_id: '22222222-2222-4222-8222-222222222222', charger_id: 'charger-1',
  start_time: '2026-10-15T06:00:00Z', end_time: '2026-10-15T07:00:00Z', status: 'SCHEDULED',
  created_at: '2026-09-27T10:00:00Z', updated_at: '2026-09-27T10:00:00Z',
}

afterEach(() => vi.restoreAllMocks())

describe('ReservationRow', () => {
  it('cancels an unstarted booking and immediately marks it cancelled', async () => {
    vi.spyOn(api, 'cancelReservation').mockResolvedValue({ ...scheduled, status: 'CANCELLED' })
    const onCancelled = vi.fn()
    render(<ReservationRow reservation={scheduled} chargerName="Charger 1" onCancelled={onCancelled} onUnauthorized={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Cancel reservation for Charger 1' }))
    await waitFor(() => expect(screen.getByText('cancelled')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: /Cancel reservation/ })).not.toBeInTheDocument()
    expect(onCancelled).toHaveBeenCalledOnce()
  })

  it('keeps the cancel action and explains a conflict', async () => {
    vi.spyOn(api, 'cancelReservation').mockRejectedValue(new ApiError(409, 'RESERVATION_NOT_CANCELLABLE', 'Only scheduled or waiting reservations can be cancelled.'))
    render(<ReservationRow reservation={scheduled} chargerName="Charger 1" onCancelled={vi.fn()} onUnauthorized={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Cancel reservation for Charger 1' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Only scheduled or waiting reservations can be cancelled.')
    expect(screen.getByRole('button', { name: 'Cancel reservation for Charger 1' })).toBeEnabled()
  })

  it('does not offer cancellation after charging starts', () => {
    render(<ReservationRow reservation={{ ...scheduled, status: 'ACTIVE' }} chargerName="Charger 1" onCancelled={vi.fn()} onUnauthorized={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /Cancel reservation/ })).not.toBeInTheDocument()
  })
})
