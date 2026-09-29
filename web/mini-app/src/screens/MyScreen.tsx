import { useEffect, useState } from 'react'
import { ChevronRight, MapPin, Store } from 'lucide-react'
import { ApiError, api, type Booking } from '../api'
import { Btn, Empty, ErrorBlock, Loader } from '../components'
import { code, dayNumber, dayTitle, time, until } from '../format'

// when — «07:30 · через 6 ч» или просто «завтра, 1 октября в 20:30».
function when(iso: string): string {
  const rel = until(iso)
  return rel.startsWith('через') ? `${time(iso)} · ${rel}` : rel
}
import { useNav } from '../nav'

const statusText: Record<Booking['status'], string> = {
  confirmed: 'Активна',
  attended: 'Посещено',
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
      <header className="hero">
        <h1 className="h1">Мои занятия</h1>
      </header>
      {error && (error.auth ? <Empty title="Открой приложение в MAX" text="Записи привязаны к твоему аккаунту MAX." /> : <ErrorBlock text={error.text} onRetry={load} />)}
      {!error && !items && <Loader />}
      {items && upcoming.length === 0 && (
        <Empty
          title="В расписании пока пусто"
          text="Самое время размяться — пробное занятие ждёт."
          action={<Btn onClick={() => nav.tab({ name: 'search' })}>Найти тренировку</Btn>}
        />
      )}
      {upcoming.map((b) => {
        const d = dayNumber(b.slot.start_at)
        return (
          <article key={b.id} className="booking" onClick={() => nav.push({ name: 'ticket', id: b.id })}>
            <div className="booking__date">
              <span className="booking__day">{d.day}</span>
              <span>{d.month}</span>
              <span>{d.weekday}</span>
            </div>
            <div className="booking__body">
              <div className="booking__when">{when(b.slot.start_at)}</div>
              <div className="booking__title">{b.slot.title}</div>
              <div className="meta"><MapPin size={13} /><span>{b.venue.name}</span></div>
              <span className="badge badge--mint">Код {code(b.checkin_code)}</span>
            </div>
            <ChevronRight className="booking__chevron" size={20} />
          </article>
        )
      })}
      {past.length > 0 && <h2 className="h2 section">История</h2>}
      {past.length > 0 && (
        <div className="list">
          {past.map((b) => (
            <button key={b.id} className="list__row" onClick={() => nav.push({ name: 'ticket', id: b.id })}>
              <div>
                <div className="list__title">{b.slot.title}</div>
                <div className="muted small">{dayTitle(b.slot.start_at)} · {b.venue.name}</div>
              </div>
              <span className={`status status--${b.status}`}>{statusText[b.status]}{b.rating ? ` · ${b.rating}/5` : ''}</span>
            </button>
          ))}
        </div>
      )}
      <button className="link-row" onClick={() => nav.push({ name: 'partner' })}>
        <Store size={18} /> Я администратор студии
      </button>
    </div>
  )
}
