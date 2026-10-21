import { useEffect, useState } from 'react'
import { api, type StudioStats } from '../api'
import { ErrorBlock, Loader } from '../components'

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

export function StatsTab({ venueId }: { venueId: string }) {
  const [s, setS] = useState<StudioStats | null>(null)
  const [error, setError] = useState('')
  const load = () => {
    setError('')
    api.studioStats(venueId).then(setS).catch((e) => setError(e.message))
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
    </>
  )
}
