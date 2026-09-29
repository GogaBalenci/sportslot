import { useEffect, useState } from 'react'
import { Button, Typography } from '@maxhub/max-ui'
import { ApiError, api, type Booking } from '../api'
import { ErrorBlock, Loader } from '../components'
import { code, dayTitle, time } from '../format'
import { useNav } from '../nav'

const statusText: Record<Booking['status'], string> = {
  confirmed: 'Записаны',
  attended: 'Посещение отмечено',
  no_show: 'Пропущено',
  cancelled: 'Отменено',
}

export function MyScreen() {
  const nav = useNav()
  const [items, setItems] = useState<Booking[] | null>(null)
  const [error, setError] = useState<{ text: string; auth: boolean } | null>(null)

  const load = () => {
    setError(null)
    api
      .me()
      .then((r) => setItems(r.bookings))
      .catch((e) => setError({ text: e.message, auth: e instanceof ApiError && e.status === 401 }))
  }
  useEffect(load, [])

  const now = Date.now()
  const upcoming = items?.filter((b) => b.status === 'confirmed' && new Date(b.slot.end_at).getTime() > now) ?? []
  const past = items?.filter((b) => !upcoming.includes(b)).reverse() ?? []

  return (
    <div className="screen">
      <header className="screen__header">
        <Typography.Headline variant="medium">Мои занятия</Typography.Headline>
      </header>
      {error && (error.auth
        ? <div className="empty">Записи доступны, когда приложение открыто в MAX.</div>
        : <ErrorBlock text={error.text} onRetry={load} />)}
      {!error && !items && <Loader />}
      {items && upcoming.length === 0 && (
        <div className="empty">
          <div>Предстоящих занятий нет.</div>
          <Button onClick={() => nav.tab({ name: 'search' })}>Найти тренировку</Button>
        </div>
      )}
      {upcoming.map((b) => (
        <article key={b.id} className="card" onClick={() => nav.push({ name: 'ticket', id: b.id })}>
          <div className="card__head">
            <Typography.Title variant="small-strong">{b.slot.title}</Typography.Title>
            <span className="badge badge--partner">Код {code(b.checkin_code)}</span>
          </div>
          <div><b>{dayTitle(b.slot.start_at)}, {time(b.slot.start_at)}</b></div>
          <div className="muted">{b.venue.name}, {b.venue.address}</div>
          <div className="link">Открыть QR-пропуск</div>
        </article>
      ))}
      {past.length > 0 && <h3 className="section">История</h3>}
      {past.map((b) => (
        <div key={b.id} className="history-row" onClick={() => nav.push({ name: 'ticket', id: b.id })}>
          <div>
            <div>{b.slot.title}</div>
            <div className="muted small">{dayTitle(b.slot.start_at)} · {b.venue.name}</div>
          </div>
          <span className="muted small">{statusText[b.status]}{b.rating ? ` · ${b.rating}/5` : ''}</span>
        </div>
      ))}
      <button className="text-button" onClick={() => nav.push({ name: 'partner' })}>
        Я администратор студии
      </button>
    </div>
  )
}
