import { useEffect, useState } from 'react'
import QRCode from 'qrcode'
import { Button, Typography } from '@maxhub/max-ui'
import { api, type Booking } from '../api'
import { ErrorBlock, Loader } from '../components'
import { code, dayTitle, routeUrl, time } from '../format'
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
    QRCode.toDataURL(`SPORTSLOT:${b.checkin_code}`, { margin: 1, width: 480, errorCorrectionLevel: 'M' })
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
      nav.toast('Запись отменена')
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
      {fresh && <div className="notice notice--success">Вы записаны! Напомним в чате за сутки и за два часа.</div>}
      <div className="ticket">
        <Typography.Title variant="medium-strong">{b.slot.title}</Typography.Title>
        <div><b>{dayTitle(b.slot.start_at)}, {time(b.slot.start_at)}–{time(b.slot.end_at)}</b></div>
        <div className="muted">{b.venue.name}, {b.venue.address}</div>
        {active ? (
          <>
            {qr ? <img className="ticket__qr" src={qr} alt="QR-код для входа" /> : <div className="ticket__qr" />}
            <div className="ticket__code">{code(b.checkin_code)}</div>
            <div className="muted small">Покажите QR администратору или назовите код</div>
          </>
        ) : (
          <div className="ticket__status">
            {b.status === 'attended' ? 'Посещение отмечено' : b.status === 'cancelled' ? 'Запись отменена' : 'Занятие пропущено'}
          </div>
        )}
      </div>

      {b.status === 'attended' && (
        <div className="rating">
          <div>{b.rating ? 'Спасибо за оценку!' : 'Как прошло занятие?'}</div>
          <div className="rating__stars">
            {[1, 2, 3, 4, 5].map((n) => (
              <button key={n} className={b.rating && n <= b.rating ? 'on' : ''} onClick={() => rate(n)}>{n}</button>
            ))}
          </div>
          <Button variant="secondary" stretched onClick={() => nav.push({ name: 'venue', id: b.venue.id })}>
            Записаться ещё
          </Button>
        </div>
      )}

      {active && (
        <div className="stack">
          {b.venue.what_to_bring && <p className="muted">Что взять: {b.venue.what_to_bring}</p>}
          <Button size="large" stretched onClick={() => openExternal(routeUrl(b.venue.lat, b.venue.lon))}>Как добраться</Button>
          <Button size="large" variant="secondary" stretched onClick={() => nav.push({ name: 'venue', id: b.venue.id, reschedule: b.id })}>
            Перенести
          </Button>
          {confirmCancel ? (
            <div className="confirm">
              <div>Отменить запись? Место уйдёт тому, кто ждёт в очереди.</div>
              <div className="actions-row">
                <Button variant="destructive" loading={busy} onClick={cancel}>Отменить</Button>
                <Button variant="secondary" onClick={() => setConfirmCancel(false)}>Оставить</Button>
              </div>
            </div>
          ) : (
            <Button size="large" variant="ghost" stretched onClick={() => setConfirmCancel(true)}>Отменить запись</Button>
          )}
        </div>
      )}
    </div>
  )
}
