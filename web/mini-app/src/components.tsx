import { useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { Calendar, Check, Clock, MapPin, Navigation, QrCode, Zap } from 'lucide-react'
import { ApiError, api, type Booking, type SlotInfo, type Venue, type VenueCard } from './api'
import { dayTitle, distance, routeUrl, seats, shortDay, time, trial, until } from './format'
import { haptic, openExternal } from './max'
import { useNav } from './nav'

export function Logo() {
  return (
    <div className="logo">
      <span className="logo__mark">
        <Zap size={20} strokeWidth={2.5} />
      </span>
      <span className="logo__text">
        Спорт<span>Слот</span>
      </span>
    </div>
  )
}

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'light'
  size?: 'md' | 'lg'
  block?: boolean
  loading?: boolean
  icon?: ReactNode
}

export function Btn({ variant = 'primary', size = 'md', block, loading, icon, children, className, disabled, ...rest }: BtnProps) {
  const cls = ['btn', `btn--${variant}`, `btn--${size}`, block ? 'btn--block' : '', className ?? ''].join(' ')
  return (
    <button className={cls} disabled={disabled || loading} {...rest}>
      {loading ? <span className="spinner" /> : icon}
      {children && <span>{children}</span>}
    </button>
  )
}

export function Chips<T extends string>(props: {
  items: { id: T; title: string }[]
  value: T
  onChange: (id: T) => void
  tone?: 'orange' | 'navy'
}) {
  return (
    <div className="chips" role="tablist">
      {props.items.map((item) => (
        <button
          key={item.id}
          className={`chip chip--${props.tone ?? 'orange'}${item.id === props.value ? ' chip--active' : ''}`}
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

export function AccessBadge({ venue }: { venue: Venue }) {
  return venue.booking_mode === 'instant' ? (
    <span className="badge badge--orange">
      <Zap size={12} strokeWidth={2.5} /> Онлайн-запись
    </span>
  ) : (
    <span className="badge badge--muted">Запись в зале</span>
  )
}

export function SportBadge({ sport }: { sport: string }) {
  const nav = useNav()
  const title = nav.catalog?.sports.find((s) => s.id === sport)?.title
  return title ? <span className="badge badge--navy">{title}</span> : null
}

export function districtTitle(id: string, districts: { id: string; title: string }[] = []): string {
  const d = districts.find((x) => x.id === id)
  return d ? `${d.title} р-н` : ''
}

function seatsTone(n: number): string {
  if (n <= 0) return 'full'
  return n <= 2 ? 'few' : 'ok'
}

// SlotPill — время занятия с одним из состояний брендбука:
// свободно, мало мест, мест нет (можно в очередь), уже записан.
export function SlotPill({ slot, booked, showDay, onClick }: { slot: SlotInfo; booked?: boolean; showDay?: boolean; onClick: () => void }) {
  const tone = booked ? 'booked' : seatsTone(slot.quota_available)
  return (
    <button className={`slot slot--${tone}`} onClick={onClick}>
      {showDay && <span className="slot__day">{shortDay(slot.start_at)}</span>}
      <span className="slot__time">
        {booked && <Check size={13} strokeWidth={3} />}
        {time(slot.start_at)}
      </span>
      <span className="slot__meta">{booked ? 'Твой слот' : slot.quota_available === 0 ? 'В очередь' : seats(slot.quota_available)}</span>
    </button>
  )
}

export function VenueCardView({ card, sport, onSlot }: { card: VenueCard; sport?: string; onSlot: (venue: Venue, slot: SlotInfo) => void }) {
  const nav = useNav()
  const v = card.venue
  const place = [districtTitle(v.district, nav.catalog?.districts), distance(card.distance_km)].filter(Boolean).join(' · ')
  const instant = v.booking_mode === 'instant'
  return (
    <article className={`card${instant ? ' card--instant' : ''}`} onClick={() => nav.push({ name: 'venue', id: v.id })}>
      <div className="card__badges">
        <SportBadge sport={sport && v.sports.includes(sport) ? sport : v.sport_type} />
        <AccessBadge venue={v} />
      </div>
      <h3 className="card__title">{v.name}</h3>
      <div className="meta">
        <MapPin size={14} />
        <span>{[place, v.address].filter(Boolean).join(' · ')}</span>
      </div>
      {instant ? (
        <>
          {trial(v.trial_price) && <div className="trial">{trial(v.trial_price)}</div>}
          <div className="card__label">Ближайшие занятия</div>
          <div className="slots" onClick={(e) => e.stopPropagation()}>
            {card.slots.slice(0, 4).map((s) => (
              <SlotPill key={s.slot_id} slot={s} showDay onClick={() => onSlot(v, s)} />
            ))}
          </div>
        </>
      ) : (
        <div className="card__hint">Расписание и пробное занятие — у администратора зала</div>
      )}
    </article>
  )
}

export function Sheet({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  return (
    <div className="sheet-backdrop" onClick={onClose}>
      <div className="sheet" onClick={(e) => e.stopPropagation()} role="dialog">
        <div className="sheet__grip" />
        {children}
      </div>
    </div>
  )
}

// BookingSheet — подтверждение записи (или переноса существующей записи).
export function BookingSheet(props: { venue: Venue; slot: SlotInfo; reschedule?: string; onClose: () => void }) {
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
      nav.toast(`Твоё место в очереди: ${position}. Освободится слот — запишем и напишем в чат`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не получилось встать в очередь')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Sheet onClose={props.onClose}>
      <SportBadge sport={slot.sport_type} />
      <h2 className="h2">{slot.title}</h2>
      <div className="summary">
        <div className="summary__row"><Calendar size={18} /><span>{dayTitle(slot.start_at)}</span></div>
        <div className="summary__row"><Clock size={18} /><span>{time(slot.start_at)}–{time(slot.end_at)}</span></div>
        <div className="summary__row"><MapPin size={18} /><span>{venue.name}<small>{venue.address}</small></span></div>
      </div>
      {trial(venue.trial_price) && <div className="trial">{trial(venue.trial_price)}</div>}
      {venue.what_to_bring && <p className="muted">Что взять: {venue.what_to_bring}</p>}
      {full ? (
        <div className="note note--warning">Мест нет. Встань в очередь — если кто-то отменит запись, место достанется тебе автоматически.</div>
      ) : (
        <div className="note">{seats(slot.quota_available)} из {slot.quota_total}. Напомним в чате за сутки и за 2 часа.</div>
      )}
      {error && <div className="error">{error}</div>}
      <div className="stack">
        {full && !props.reschedule ? (
          <Btn size="lg" block loading={busy} onClick={joinWaitlist}>Встать в очередь</Btn>
        ) : (
          <Btn size="lg" block loading={busy} disabled={full} onClick={confirm}>
            {props.reschedule ? 'Перенести сюда' : 'Записаться'}
          </Btn>
        )}
        <Btn size="lg" variant="secondary" block icon={<Navigation size={18} />} onClick={() => openExternal(routeUrl(venue.lat, venue.lon))}>
          Как добраться
        </Btn>
      </div>
    </Sheet>
  )
}

// NextWorkout — виджет ближайшей тренировки на главном экране.
export function NextWorkout({ booking }: { booking: Booking }) {
  const nav = useNav()
  return (
    <section className="next">
      <div className="next__label">Ближайшая тренировка · {until(booking.slot.start_at)}</div>
      <div className="next__title">{booking.slot.title}</div>
      <div className="next__meta">
        <MapPin size={14} /> {booking.venue.name}, {booking.venue.address}
      </div>
      <div className="next__actions">
        <Btn variant="primary" icon={<QrCode size={18} />} onClick={() => nav.push({ name: 'ticket', id: booking.id })}>QR-пропуск</Btn>
        <Btn variant="light" icon={<Navigation size={18} />} onClick={() => openExternal(routeUrl(booking.venue.lat, booking.venue.lon))}>Маршрут</Btn>
      </div>
    </section>
  )
}

export function Loader() {
  return (
    <div className="loader">
      <span className="spinner spinner--dark" /> Загружаем…
    </div>
  )
}

export function ErrorBlock({ text, onRetry }: { text: string; onRetry?: () => void }) {
  return (
    <div className="empty">
      <div>{text}</div>
      {onRetry && <Btn variant="secondary" onClick={onRetry}>Повторить</Btn>}
    </div>
  )
}

export function Empty({ title, text, action }: { title: string; text?: string; action?: ReactNode }) {
  return (
    <div className="empty">
      <div className="empty__title">{title}</div>
      {text && <div>{text}</div>}
      {action}
    </div>
  )
}
