import { useEffect, useState } from 'react'
import QRCode from 'qrcode'
import { CalendarClock, CheckCircle2, Clock, MapPin, Navigation } from 'lucide-react'
import { api, type Booking } from '../api'
import { Btn, ErrorBlock, Loader, SportBadge } from '../components'
import { code, dayTitle, routeUrl, time, until } from '../format'
import { brightenScreen, haptic, openExternal, restoreBrightness } from '../max'
import { useNav } from '../nav'

export function TicketScreen({ id, fresh }: { id: string; fresh?: boolean }) {
  const nav = useNav()
  const [b, setB] = useState<Booking | null>(null)
  const [qr, setQr] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false)

  const load = () => {
    setError('')
    api.booking(id).then(setB).catch((e) => setError(e.message))
  }
  useEffect(load, [id])

  const active = b?.status === 'confirmed'
  useEffect(() => {
    if (!b || !active) return
    QRCode.toDataURL(`SPORTSLOT:${b.checkin_code}`, { margin: 1, width: 480, errorCorrectionLevel: 'M', color: { dark: '#1A237E', light: '#FFFFFF' } })
      .then(setQr)
      .catch(() => setQr(''))
    brightenScreen()
    return restoreBrightness
  }, [b, active])

  async function cancel() {
    setBusy(true)
    try {
      await api.cancel(id)
      haptic('success')
      nav.toast('Запись отменена — место досталось тому, кто ждал')
      nav.tab({ name: 'my' })
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось отменить')
    } finally {
      setBusy(false)
    }
  }

  async function rate(n: number) {
    try {
      await api.feedback(id, n)
      haptic('success')
      setB((prev) => (prev ? { ...prev, rating: n } : prev))
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось сохранить оценку')
    }
  }

  if (error) return <div className="screen"><ErrorBlock text={error} onRetry={load} /></div>
  if (!b) return <div className="screen"><Loader /></div>

  return (
    <div className="screen">
      {fresh && (
        <div className="success">
          <CheckCircle2 size={22} />
          <div>
            <b>Слот за тобой! Ждём на тренировке</b>
            <div>Напомним в чате за сутки и за 2 часа.</div>
          </div>
        </div>
      )}
      <section className="ticket">
        <SportBadge sport={b.slot.sport_type} />
        <h2 className="h2">{b.slot.title}</h2>
        <div className="ticket__meta">
          <span><CalendarClock size={15} /> {dayTitle(b.slot.start_at)}</span>
          <span><Clock size={15} /> {time(b.slot.start_at)}–{time(b.slot.end_at)}</span>
        </div>
        <div className="meta meta--center"><MapPin size={14} /><span>{b.venue.name}, {b.venue.address}</span></div>
        {active ? (
          <>
            <div className="ticket__qr">{qr && <img src={qr} alt="QR-код для входа" />}</div>
            <div className="ticket__code">{code(b.checkin_code)}</div>
            <div className="muted small">Покажи QR администратору или назови код · {until(b.slot.start_at)}</div>
          </>
        ) : (
          <div className={`ticket__status status--${b.status}`}>
            {b.status === 'attended' ? 'Посещение отмечено' : b.status === 'cancelled' ? 'Запись отменена' : 'Занятие пропущено'}
          </div>
        )}
      </section>

      {b.status === 'attended' && (
        <section className="rating">
          <h2 className="h2">{b.rating ? 'Спасибо за оценку!' : 'Как прошло занятие?'}</h2>
          <div className="rating__row">
            {[1, 2, 3, 4, 5].map((n) => (
              <button key={n} className={b.rating && n <= b.rating ? 'on' : ''} onClick={() => rate(n)}>{n}</button>
            ))}
          </div>
          <Btn block onClick={() => nav.push({ name: 'venue', id: b.venue.id })}>Записаться ещё</Btn>
        </section>
      )}

      {active && (
        <div className="stack">
          {b.venue.what_to_bring && <div className="info-card"><p className="muted">Что взять: {b.venue.what_to_bring}</p></div>}
          <Btn size="lg" block icon={<Navigation size={18} />} onClick={() => openExternal(routeUrl(b.venue.lat, b.venue.lon))}>Как добраться</Btn>
          <Btn size="lg" variant="secondary" block onClick={() => nav.push({ name: 'venue', id: b.venue.id, reschedule: b.id })}>Перенести</Btn>
          {confirmCancel ? (
            <div className="confirm">
              <div>Отменить запись? Место уйдёт тому, кто ждёт в очереди.</div>
              <div className="row">
                <Btn variant="danger" loading={busy} onClick={cancel}>Отменить</Btn>
                <Btn variant="secondary" onClick={() => setConfirmCancel(false)}>Оставить</Btn>
              </div>
            </div>
          ) : (
            <Btn size="lg" variant="ghost" block onClick={() => setConfirmCancel(true)}>Отменить запись</Btn>
          )}
        </div>
      )}
    </div>
  )
}
