import { useEffect, useState } from 'react'
import { api, type ReviewSummary, type StudioStats } from '../api'
import { ErrorBlock, Loader, Stars } from '../components'
import { shortDay } from '../format'

function Tile({ value, label, hint }: { value: string | number; label: string; hint?: string }) {
  return (
    <div className="tile">
      <div className="tile__value">{value}</div>
      <div className="tile__label">{label}</div>
      {hint && <div className="tile__hint">{hint}</div>}
    </div>
  )
}

const percent = (part: number, total: number) => (total > 0 ? `${Math.round((part / total) * 100)}%` : '—')
const minutes = (m: number) => (m < 60 ? `${Math.max(1, Math.round(m))} мин` : `${Math.floor(m / 60)} ч ${Math.round(m % 60)} мин`)

export function StatsTab({ venueId }: { venueId: string }) {
  const [s, setS] = useState<StudioStats | null>(null)
  const [reviews, setReviews] = useState<ReviewSummary | null>(null)
  const [error, setError] = useState('')
  const load = () => {
    setError('')
    api.studioStats(venueId).then(setS).catch((e) => setError(e.message))
    api.studioReviews(venueId).then(setReviews).catch(() => setReviews(null))
  }
  useEffect(load, [venueId])

  if (error) return <ErrorBlock text={error} onRetry={load} />
  if (!s) return <Loader />
  const finished = s.attended + s.no_show
  return (
    <>
      <p className="muted small">За последние 30 дней — только клиенты, пришедшие через СпортСлот.</p>
      <div className="tiles">
        <Tile value={s.bookings} label="Записи" />
        <Tile value={s.attended} label="Пришли" hint={finished ? `${percent(s.attended, finished)} от прошедших` : undefined} />
        <Tile value={s.no_show} label="Неявки" hint={finished ? percent(s.no_show, finished) : undefined} />
        <Tile value={s.cancelled_by_client} label="Отменили заранее" hint="место ушло другим" />
        <Tile value={s.clients} label="Новые клиенты" />
        <Tile value={s.returning_clients} label="Записались повторно" />
        <Tile value={s.ratings ? s.rating_avg.toFixed(1) : '—'} label="Средняя оценка" hint={s.ratings ? `${s.ratings} оценок` : undefined} />
        <Tile value={s.upcoming} label="Предстоящие записи" />
      </div>

      <h2 className="h2 section">Ответы клиентам</h2>
      <div className="tiles">
        <Tile value={s.response.answered ? minutes(s.response.median_minutes) : '—'} label="Обычно отвечаете за" hint="медиана первого ответа" />
        <Tile value={percent(s.response.within_hour, s.response.answered)} label="Ответов в течение часа" hint={s.response.answered ? `${s.response.within_hour} из ${s.response.answered}` : undefined} />
        <Tile value={s.response.waiting} label="Ждут ответа сейчас" />
        <Tile value={s.paid.count ? `${s.paid.amount.toLocaleString('ru-RU')} ₽` : '—'} label="Оплачено онлайн" hint={s.paid.count ? `${s.paid.count} оплат` : undefined} />
      </div>
      <p className="muted small">Если клиент ждёт ответа больше часа, бот напомнит администраторам; через три часа клиенту придёт телефон студии. Быстрый ответ — главный способ довести клиента до занятия.</p>

      {reviews && reviews.count > 0 && (
        <>
          <h2 className="h2 section">Отзывы <Stars average={reviews.average} count={reviews.count} /></h2>
          <div className="list">
            {reviews.items?.map((r, i) => (
              <div key={i} className="list__row list__row--static review">
                <div className="list__main">
                  <div className="list__title">{'★'.repeat(r.rating)}<span className="muted">{'★'.repeat(5 - r.rating)}</span> · {r.author}</div>
                  {r.text && <div className="small">{r.text}</div>}
                </div>
                <span className="muted small">{shortDay(r.date)}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </>
  )
}
