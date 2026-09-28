import { useState } from 'react'
import { ReservationDialog } from './components/ReservationDialog'
import { ReservationRow } from './components/ReservationRow'
import { useAuth } from './hooks/useAuth'
import { useChargers } from './hooks/useChargers'
import { useReservations } from './hooks/useReservations'
import type { Charger, ChargerStatus } from './types/api'

const statusCopy: Record<ChargerStatus, string> = { AVAILABLE: 'Available', CHARGING: 'Charging', MAINTENANCE: 'Maintenance' }

function ChargerIcon({ small = false }: { small?: boolean }) {
  return <svg className={small ? 'charger-icon charger-icon-small' : 'charger-icon'} viewBox="0 0 64 76" fill="none" aria-hidden="true">
    <rect x="10" y="4" width="39" height="67" rx="8" stroke="currentColor" strokeWidth="3" />
    <rect x="16" y="11" width="27" height="25" rx="3" stroke="currentColor" strokeWidth="2.5" />
    <path d="M32 15l-8 11h7l-3 7 10-12h-8l2-6Z" fill="currentColor" />
    <path d="M27 54h6m16-36h4c3 0 5 2 5 5v23c0 4-2 7-6 7h-3" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    <path d="M54 13v8m4-8v8" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
  </svg>
}

function LoginForm({ onLogin, busy, error }: { onLogin: (username: string, password: string) => Promise<void>, busy: boolean, error: string | null }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  return <form className="login-form" onSubmit={async (event) => { event.preventDefault(); try { await onLogin(username, password) } catch { /* The hook exposes the API error. */ } }}>
    <div className="panel-heading"><span className="panel-kicker">Account access</span><h2>Sign in to reserve</h2><p>View your bookings and choose a charging window.</p></div>
    <label htmlFor="login-username">Username</label><input id="login-username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required />
    <label htmlFor="login-password">Password</label><input id="login-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
    {error && <p className="form-error" role="alert">{error}</p>}
    <button type="submit" className="button-primary full-width" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
    <p className="login-hint">Demo account <strong>demo</strong> / <strong>DemoPass123!</strong></p>
  </form>
}

function ChargerRow({ charger, onReserve }: { charger: Charger, onReserve: () => void }) {
  return <article className={`charger-row charger-row-${charger.status.toLowerCase()}`}>
    <div className="charger-symbol"><ChargerIcon small /></div>
    <div className="charger-main"><span className="charger-id">{charger.id}</span><h3>{charger.name}</h3><p>{charger.location}</p></div>
    <div className="charger-action"><span className={`status-pill status-${charger.status.toLowerCase()}`}><i aria-hidden="true" />{statusCopy[charger.status]}</span><button className="button-outline" type="button" onClick={onReserve}>Reserve a time <span aria-hidden="true">↗</span></button></div>
  </article>
}

function ReservationTable({ id, title, empty, bookings, chargers, onCancelled, onUnauthorized }: {
  id: string
  title: string
  empty: string
  bookings: ReturnType<typeof useReservations>
  chargers: Charger[]
  onCancelled: () => void
  onUnauthorized: () => void
}) {
  return <section className="reservation-group" aria-labelledby={id}>
    <div className="reservation-group-heading"><h3 id={id}>{title}</h3><span>{bookings.total}</span></div>
    {bookings.error && <p className="form-error" role="alert">{bookings.error}</p>}
    <table className="reservation-table" aria-labelledby={id}><thead><tr><th scope="col">Reservation</th><th scope="col">Status</th></tr></thead><tbody>{bookings.reservations.length === 0 ? <tr><td className="booking-empty" colSpan={2}>{bookings.loading ? 'Loading reservations…' : bookings.total ? 'No reservations on this page.' : empty}</td></tr> : bookings.reservations.map((reservation) => <ReservationRow key={reservation.id} reservation={reservation} chargerName={chargers.find((charger) => charger.id === reservation.charger_id)?.name ?? reservation.charger_id} onUnauthorized={onUnauthorized} onCancelled={onCancelled} />)}</tbody></table>
    {bookings.pageCount > 1 && <nav className="reservation-pagination" aria-label={`${title} pages`}><button type="button" disabled={bookings.page === 1} onClick={() => bookings.goToPage(bookings.page - 1)}>Previous</button><span>Page {bookings.page} of {bookings.pageCount}</span><button type="button" disabled={bookings.page >= bookings.pageCount} onClick={() => bookings.goToPage(bookings.page + 1)}>Next</button></nav>}
  </section>
}

export default function App() {
  const auth = useAuth()
  const live = useChargers()
  const upcoming = useReservations(auth.user?.id ?? null, auth.onUnauthorized, live.connectionEpoch, 'upcoming')
  const history = useReservations(auth.user?.id ?? null, auth.onUnauthorized, live.connectionEpoch, 'history')
  const [selected, setSelected] = useState<Charger | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [loginPrompt, setLoginPrompt] = useState(false)
  const available = live.chargers.filter((charger) => charger.status === 'AVAILABLE').length
  const charging = live.chargers.filter((charger) => charger.status === 'CHARGING').length
  const maintenance = live.chargers.filter((charger) => charger.status === 'MAINTENANCE').length

  const reserve = (charger: Charger) => {
    setNotice(null)
    if (!auth.user) { setLoginPrompt(true); document.getElementById('account-panel')?.scrollIntoView({ behavior: 'smooth' }); return }
    setSelected(charger)
  }

  return <div className="app-shell">
    <header className="site-header"><div className="site-header-inner"><a className="brand" href="#top" aria-label="ChargeHere home"><span className="brand-mark"><ChargerIcon small /></span><span>chargehere<span className="brand-dot">.</span></span></a><div className="header-right"><span className={`connection-label ${live.connected ? 'is-live' : ''}`}><i aria-hidden="true" />{live.connected ? 'Live connection' : 'Reconnecting'}</span>{auth.user && <span className="header-user">{auth.user.username}</span>}</div></div></header>
    <main id="top">
      <section className="hero" aria-labelledby="hero-title"><div className="hero-inner"><div className="hero-copy"><span className="hero-label"><span className="signal-dot" /> Live charging network</span><h1 id="hero-title">Find your next<br />charging window.</h1><p>See what each charger is doing right now, then reserve a time that works for you.</p><a className="hero-link" href="#chargers">View chargers <span aria-hidden="true">↓</span></a></div><div className="network-panel" aria-label="Current charger network summary"><div className="network-panel-top"><span>Network overview</span><span>{live.chargers.length.toString().padStart(2, '0')} chargers</span></div><div className="network-orbit" aria-hidden="true"><div className="orbit-ring orbit-ring-outer" /><div className="orbit-ring orbit-ring-inner" /><div className="orbit-core"><ChargerIcon /></div><span className="orbit-node orbit-node-one" /><span className="orbit-node orbit-node-two" /><span className="orbit-node orbit-node-three" /></div><div className="network-panel-bottom"><span className="network-live-dot" />{live.connected ? 'Updates streaming live' : 'Waiting for live updates'}</div></div></div></section>
      <div className="content-wrap"><section className="dashboard" id="chargers" aria-labelledby="chargers-title"><div className="section-heading"><div><span className="section-kicker">Charger status</span><h2 id="chargers-title">Choose a charger</h2><p>Status updates appear here without refreshing the page.</p></div></div>
        <div className="metric-strip" aria-label="Charger status counts"><div className="metric-available"><strong>{available.toString().padStart(2, '0')}</strong><span>Available</span></div><div className="metric-charging"><strong>{charging.toString().padStart(2, '0')}</strong><span>Charging</span></div><div className="metric-maintenance"><strong>{maintenance.toString().padStart(2, '0')}</strong><span>Maintenance</span></div></div>
        {live.loading && live.chargers.length === 0 ? <p className="list-message">Loading chargers…</p> : live.chargers.length === 0 ? <p className="list-message">No chargers are available yet.</p> : <div className="charger-list">{live.chargers.map((charger) => <ChargerRow key={charger.id} charger={charger} onReserve={() => reserve(charger)} />)}</div>}
        {live.error && <p className="inline-error" role="status">{live.error}</p>}
        <p className="status-note">Live status shows current activity. Future time slots are checked when you book.</p>
      </section>
      <aside className="account-panel" id="account-panel">{auth.user ? <>
        <div className="panel-heading"><span className="panel-kicker">Your account</span><div className="panel-title-row"><h2>Your reservations</h2><button type="button" className="text-button" onClick={() => { setSelected(null); void auth.logout() }}>Sign out</button></div><p>Signed in as <strong>{auth.user.username}</strong></p></div>
        {notice && <p className="success-notice" role="status">{notice}</p>}
        <ReservationTable id="upcoming-reservations" title="Active & upcoming" empty="No active or upcoming reservations." bookings={upcoming} chargers={live.chargers} onUnauthorized={auth.onUnauthorized} onCancelled={() => { setNotice('Reservation cancelled. This time slot is available again.'); upcoming.refresh(); history.refresh() }} />
        <ReservationTable id="reservation-history" title="History" empty="No completed or closed reservations." bookings={history} chargers={live.chargers} onUnauthorized={auth.onUnauthorized} onCancelled={() => { upcoming.refresh(); history.refresh() }} />
      </> : <><LoginForm onLogin={auth.login} busy={auth.loading} error={auth.error} />{loginPrompt && <p className="login-prompt" role="status">Sign in first to reserve a charger.</p>}</>}</aside></div>
    </main><footer><span>ChargeHere</span><span>All times shown in your local time zone.</span></footer>
    {selected && auth.user && <ReservationDialog charger={selected} user={auth.user} onClose={() => setSelected(null)} onUnauthorized={auth.onUnauthorized} onSuccess={() => { setSelected(null); setNotice('Reservation confirmed.'); upcoming.showFirstPage() }} />}
  </div>
}
