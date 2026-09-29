import { useState } from 'react'
import { Button, Typography } from '@maxhub/max-ui'
import { ApiError, api, type Booking, type SlotInfo, type Venue, type VenueCard } from './api'
import { dayTitle, distance, routeUrl, seats, shortDay, time, trial } from './format'
import { haptic, openExternal } from './max'
import { useNav } from './nav'

export function Chips<T extends string>(props: {
  items: { id: T; title: string }[]
  value: T
  onChange: (id: T) => void
}) {
  return (
    <div className="chips" role="tablist">
      {props.items.map((item) => (
        <button
          key={item.id}
          className={`chip${item.id === props.value ? ' chip--active' : ''}`}
          onClick={() => {
            haptic('tap')
            props.onChange(item.id)
          }}
        >
          {item.title}
        </button>
      ))}
    </div>
  )
}

export function SourceBadge({ venue }: { venue: Venue }) {
  return venue.source === 'demo_partner' ? (
    <span className="badge badge--partner">Онлайн-запись · демо-партнёр</span>
  ) : (
    <span className="badge">OpenStreetMap</span>
  )
}

export function districtTitle(id: string, districts: { id: string; title: string }[] = []): string {
  const d = districts.find((x) => x.id === id)
  return d ? `${d.title} р-н` : ''
}

export function VenueCardView({ card, onSlot }: { card: VenueCard; onSlot: (venue: Venue, slot: SlotInfo) => void }) {
  const nav = useNav()
  const v = card.venue
  const place = [districtTitle(v.district, nav.catalog?.districts), distance(card.distance_km)].filter(Boolean).join(' · ')
  return (
    <article className="card" onClick={() => nav.push({ name: 'venue', id: v.id })}>
      <div className="card__head">
        <Typography.Title variant="small-strong">{v.name}</Typography.Title>
        <SourceBadge venue={v} />
      </div>
      <div className="muted">{place}</div>
      {v.address && <div className="muted">{v.address}</div>}
      {v.booking_mode === 'instant' ? (
        <>
          {trial(v.trial_price) && <div className="accent-text">{trial(v.trial_price)}</div>}
          <div className="slots" onClick={(e) => e.stopPropagation()}>
            {card.slots.slice(0, 4).map((s) => (
              <button
                key={s.slot_id}
                className={`slot-pill${s.quota_available === 0 ? ' slot-pill--full' : ''}`}
                onClick={() => onSlot(v, s)}
              >
                <b>{shortDay(s.start_at)} · {time(s.start_at)}</b>
                <span>{s.quota_available === 0 ? 'в очередь' : seats(s.quota_available)}</span>
              </button>
            ))}
          </div>
        </>
      ) : (
        <div className="muted small">Запись у самого зала — контакты и маршрут внутри</div>
      )}
    </article>
  )
}

export function Sheet({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  return (
    <div className="sheet-backdrop" onClick={onClose}>
      <div className="sheet" onClick={(e) => e.stopPropagation()} role="dialog">
        <div className="sheet__grip" />
        {children}
      </div>
    </div>
  )
}

// BookingSheet — подтверждение записи на занятие (или переноса существующей записи).
export function BookingSheet(props: {
  venue: Venue
  slot: SlotInfo
  reschedule?: string
  onClose: () => void
}) {
  const nav = useNav()
  const { venue, slot } = props
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [full, setFull] = useState(slot.quota_available === 0)

  async function confirm() {
    setBusy(true)
    setError('')
    try {
      const booking: Booking = props.reschedule
        ? await api.reschedule(props.reschedule, slot.slot_id)
        : await api.book(slot.slot_id)
      haptic('success')
      props.onClose()
      nav.push({ name: 'ticket', id: booking.id, fresh: !props.reschedule })
      if (props.reschedule) nav.toast('Запись перенесена, код входа прежний')
    } catch (e) {
      haptic('error')
      if (e instanceof ApiError && e.code === 'quota_exceeded') setFull(true)
      setError(e instanceof Error ? e.message : 'Не получилось записаться')
    } finally {
      setBusy(false)
    }
  }

  async function joinWaitlist() {
    setBusy(true)
    try {
      const { position } = await api.waitlist(slot.slot_id)
      haptic('success')
      props.onClose()
      nav.toast(`Вы ${position}-й в листе ожидания. Освободится место — запишем и пришлём сообщение в чат`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не получилось встать в очередь')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Sheet onClose={props.onClose}>
      <Typography.Title variant="medium-strong">{slot.title}</Typography.Title>
      <div className="sheet__row">
        <b>{dayTitle(slot.start_at)}, {time(slot.start_at)}–{time(slot.end_at)}</b>
      </div>
      <div className="sheet__row">{venue.name}</div>
      <div className="muted">{venue.address}</div>
      {trial(venue.trial_price) && <div className="sheet__row accent-text">{trial(venue.trial_price)}</div>}
      {venue.what_to_bring && <div className="sheet__row muted">Что взять: {venue.what_to_bring}</div>}
      {!full && <div className="sheet__row muted small">{seats(slot.quota_available)} из {slot.quota_total}. Напомним в чате за сутки и за два часа.</div>}
      {error && <div className="error">{error}</div>}
      <div className="sheet__actions">
        {full && !props.reschedule ? (
          <Button size="large" stretched loading={busy} onClick={joinWaitlist}>
            Встать в лист ожидания
          </Button>
        ) : (
          <Button size="large" stretched loading={busy} disabled={full} onClick={confirm}>
            {props.reschedule ? 'Перенести сюда' : 'Записаться'}
          </Button>
        )}
        <Button size="large" variant="secondary" stretched onClick={() => openExternal(routeUrl(venue.lat, venue.lon))}>
          Как добраться
        </Button>
      </div>
    </Sheet>
  )
}

export function Loader() {
  return <div className="center muted">Загружаем…</div>
}

export function ErrorBlock({ text, onRetry }: { text: string; onRetry?: () => void }) {
  return (
    <div className="empty">
      <div>{text}</div>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          Повторить
        </Button>
      )}
    </div>
  )
}
