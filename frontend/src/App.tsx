import { useState } from 'react'
import { ReservationDialog } from './components/ReservationDialog'
import { ReservationRow } from './components/ReservationRow'
import { useAuth } from './hooks/useAuth'
import { useChargers } from './hooks/useChargers'
import { useReservations } from './hooks/useReservations'
import type { Charger, ChargerStatus } from './types/api'

const statusCopy: Record<ChargerStatus, string> = { AVAILABLE: 'Available', CHARGING: 'Charging', MAINTENANCE: 'Maintenance' }

function LoginForm({ onLogin, busy, error }: { onLogin: (username: string, password: string) => Promise<void>, busy: boolean, error: string | null }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  return <form className="login-form" onSubmit={async (event) => { event.preventDefault(); try { await onLogin(username, password) } catch { /* The hook exposes the API error. */ } }}>
    <div className="panel-heading"><span className="eyebrow">YOUR SPACE</span><h2>Sign in to reserve</h2><p>See your bookings and choose your next charging window.</p></div>
    <label htmlFor="login-username">Username</label><input id="login-username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required />
    <label htmlFor="login-password">Password</label><input id="login-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
    {error && <p className="form-error" role="alert">{error}</p>}
    <button type="submit" className="button-primary full-width" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
    <p className="login-hint">Demo account: <strong>demo</strong> / <strong>DemoPass123!</strong></p>
  </form>
}

function ChargerRow({ charger, onReserve }: { charger: Charger, onReserve: () => void }) {
  return <article className="charger-row">
    <div className="charger-symbol" aria-hidden="true"><span>↗</span></div>
    <div className="charger-main"><div className="charger-name-line"><h3>{charger.name}</h3><span className={`status-pill status-${charger.status.toLowerCase()}`}><i aria-hidden="true" />{statusCopy[charger.status]}</span></div><p>{charger.location}</p></div>
    <div className="charger-action"><span className="charger-id">{charger.id}</span><button className="button-outline" type="button" onClick={onReserve}>Reserve <span aria-hidden="true">↗</span></button></div>
  </article>
}

export default function App() {
  const auth = useAuth()
  const live = useChargers()
  const bookings = useReservations(auth.user?.id ?? null, auth.onUnauthorized, live.connectionEpoch)
  const [selected, setSelected] = useState<Charger | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [loginPrompt, setLoginPrompt] = useState(false)
  const available = live.chargers.filter((charger) => charger.status === 'AVAILABLE').length
  const charging = live.chargers.filter((charger) => charger.status === 'CHARGING').length

  const reserve = (charger: Charger) => {
    setNotice(null)
    if (!auth.user) { setLoginPrompt(true); document.getElementById('account-panel')?.scrollIntoView({ behavior: 'smooth' }); return }
    setSelected(charger)
  }

  return <div className="app-shell">
    <header className="site-header"><div className="site-header-inner"><a className="brand" href="#top" aria-label="Chargeyard home"><span className="brand-mark" aria-hidden="true">↗</span><span>chargeyard<span className="brand-dot">.</span></span></a><div className="header-right"><span className={`connection-label ${live.connected ? 'is-live' : ''}`}><i aria-hidden="true" />{live.connected ? 'Live feed connected' : 'Reconnecting live feed'}</span>{auth.user && <span className="header-user">{auth.user.username}</span>}</div></div></header>
    <main id="top">
      <section className="hero"><div className="hero-copy"><span className="eyebrow"><span className="eyebrow-line" /> LIVE CHARGER NETWORK</span><h1>Your next charge<br /><em>starts here.</em></h1><p>Check current charger activity, choose a time that works, and keep track of every reservation in one place.</p><a className="hero-link" href="#chargers">Explore chargers <span aria-hidden="true">↓</span></a></div><div className="hero-graphic" aria-hidden="true"><div className="orb orb-one" /><div className="orb orb-two" /><div className="hero-grid"><span /><span /><span /><span /></div><div className="hero-graphic-label">POWER IN MOTION / 01</div></div></section>
      <div className="content-wrap"><section className="dashboard" id="chargers"><div className="section-heading"><div><span className="eyebrow">THE NETWORK</span><h2>Charger availability</h2><p>Status changes appear automatically as chargers report in.</p></div><div className="metric-strip"><div><strong>{available.toString().padStart(2, '0')}</strong><span>Available</span></div><div><strong>{charging.toString().padStart(2, '0')}</strong><span>Charging</span></div><div><strong>{live.chargers.length.toString().padStart(2, '0')}</strong><span>Total</span></div></div></div>
        <div className="charger-list-header"><span>CHARGER / LOCATION</span><span>RESERVATION</span></div>
        {live.loading && live.chargers.length === 0 ? <p className="list-message">Loading chargers…</p> : live.chargers.length === 0 ? <p className="list-message">No chargers are available yet.</p> : <div className="charger-list">{live.chargers.map((charger) => <ChargerRow key={charger.id} charger={charger} onReserve={() => reserve(charger)} />)}</div>}
        {live.error && <p className="inline-error" role="status">{live.error}</p>}
        <p className="status-note">Current status is a live device reading. Future reservations are checked for conflicts when you submit.</p>
      </section>
      <aside className="account-panel" id="account-panel">{auth.user ? <><div className="panel-heading"><span className="eyebrow">YOUR SPACE</span><div className="panel-title-row"><h2>Your reservations</h2><button type="button" className="text-button" onClick={() => { setSelected(null); void auth.logout() }}>Sign out</button></div><p>Signed in as <strong>{auth.user.username}</strong></p></div>{notice && <p className="success-notice" role="status">{notice}</p>}{bookings.error && <p className="form-error" role="alert">{bookings.error}</p>}{bookings.loading && bookings.reservations.length === 0 ? <p className="booking-empty">Loading your reservations…</p> : bookings.reservations.length === 0 ? <div className="booking-empty"><span aria-hidden="true">↗</span><strong>No reservations yet</strong><p>Pick a charger to plan your next session.</p></div> : <ul className="reservation-list">{bookings.reservations.map((reservation) => <ReservationRow key={reservation.id} reservation={reservation} chargerName={live.chargers.find((charger) => charger.id === reservation.charger_id)?.name ?? reservation.charger_id} onUnauthorized={auth.onUnauthorized} onCancelled={() => { setNotice('Reservation cancelled. This time slot is available again.'); bookings.refresh() }} />)}</ul>}</> : <><LoginForm onLogin={auth.login} busy={auth.loading} error={auth.error} />{loginPrompt && <p className="login-prompt" role="status">Sign in first to reserve a charger.</p>}</>}</aside></div>
    </main><footer><span>CHARGEYARD / LIVE NETWORK</span><span>All times shown in your local time zone.</span></footer>
    {selected && auth.user && <ReservationDialog charger={selected} user={auth.user} onClose={() => setSelected(null)} onUnauthorized={auth.onUnauthorized} onSuccess={() => { setSelected(null); setNotice('Reservation confirmed. Your booking appears below.'); bookings.refresh() }} />}
  </div>
}
