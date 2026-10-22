import { useEffect, useState } from 'react'
import QRCode from 'qrcode'
import { CalendarClock, CheckCircle2, Clock, CreditCard, MapPin, MessageCircle, Navigation, Phone, Share2, UserPlus } from 'lucide-react'
import { api, type Booking, type FriendPlace, type VenueDetails } from '../api'
import { Btn, ErrorBlock, Loader, SportBadge } from '../components'
import { code, dayTitle, routeUrl, time, until } from '../format'
import { brightenScreen, copyText, haptic, openExternal, restoreBrightness } from '../max'
import { useNav } from '../nav'

export function TicketScreen({ id, fresh }: { id: string; fresh?: boolean }) {
  const nav = useNav()
  const [b, setB] = useState<Booking | null>(null)
  const [qr, setQr] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false)

  const [venue, setVenue] = useState<VenueDetails | null>(null)
  const [friend, setFriend] = useState<FriendPlace | null>(null)
  const [review, setReview] = useState('')

  const load = () => {
    setError('')
    api
      .booking(id)
      .then((bk) => {
        setB(bk)
        api.venue(bk.venue.id).then(setVenue).catch(() => setVenue(null))
        if (!bk.is_guest) api.friend(bk.id).then((r) => setFriend(r.friend)).catch(() => setFriend(null))
      })
      .catch((e) => setError(e.message))
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

  async function pay() {
    setBusy(true)
    try {
      const p = await api.payBooking(id)
      if (p.confirmation_url) openExternal(p.confirmation_url)
      nav.push({ name: 'payment', id: p.id })
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось начать оплату')
    } finally {
      setBusy(false)
    }
  }

  async function inviteFriend() {
    setBusy(true)
    try {
      const place = await api.addFriend(id)
      setFriend(place)
      haptic('success')
      if (place.link) await shareFriend(place.link)
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось забронировать место')
    } finally {
      setBusy(false)
    }
  }

  async function shareFriend(link: string) {
    const text = `Пойдём вместе на тренировку? ${b?.slot.title}, ${b ? dayTitle(b.slot.start_at) : ''} в ${b ? time(b.slot.start_at) : ''}. Твой пропуск: ${link}`
    if (typeof navigator.share === 'function') {
      try {
        await navigator.share({ text })
        return
      } catch {
        /* пользователь закрыл окно — скопируем */
      }
    }
    nav.toast((await copyText(text)) ? 'Приглашение скопировано — отправь его другу' : 'Скопируй ссылку вручную')
  }

  async function sendReview() {
    try {
      await api.review(id, review)
      setB((prev) => (prev ? { ...prev, review } : prev))
      nav.toast('Спасибо за отзыв!')
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось сохранить отзыв')
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
      {b.is_guest && (
        <section className="info-card friend-card">
          <b>👥 Это место для друга</b>
          <p className="muted small">Отправь другу ссылку: он получит свой пропуск и напоминания. Если не успеет — покажите этот QR на входе.</p>
          <FriendLink bookingId={b.host_booking_id ?? ''} onShare={shareFriend} />
        </section>
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
            {(b.paid_at || b.user_pass_id) && (
              <div className="badge badge--mint">{b.user_pass_id ? '🎟 По абонементу' : '💳 Оплачено онлайн'}</div>
            )}
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
          {b.rating && !b.review && (
            <div className="review-form">
              <textarea className="input textarea" rows={3} maxLength={500} value={review} onChange={(e) => setReview(e.target.value)}
                placeholder="Пара слов о занятии: что понравилось, что нет. Отзыв поможет другим выбрать студию" />
              <Btn variant="secondary" block disabled={!review.trim()} onClick={sendReview}>Отправить отзыв</Btn>
            </div>
          )}
          {b.review && <p className="muted small">Твой отзыв: «{b.review}»</p>}
          <Btn block onClick={() => nav.push({ name: 'venue', id: b.venue.id })}>Записаться ещё</Btn>
        </section>
      )}

      {active && (
        <div className="stack">
          {b.venue.what_to_bring && <div className="info-card"><p className="muted">Что взять: {b.venue.what_to_bring}</p></div>}
          {venue?.online_payment?.trial && !b.paid_at && !b.user_pass_id && !b.is_guest && b.venue.trial_price ? (
            <Btn size="lg" block loading={busy} icon={<CreditCard size={18} />} onClick={pay}>
              Оплатить онлайн · {b.venue.trial_price.toLocaleString('ru-RU')} ₽
            </Btn>
          ) : null}
          {!b.is_guest && (
            friend ? (
              <section className="info-card friend-card">
                {friend.claimed ? (
                  <b>👥 С тобой идёт {friend.friend_name}</b>
                ) : (
                  <>
                    <b>👥 Место для друга забронировано</b>
                    <p className="muted small">Пока друг не открыл ссылку, его пропуск у тебя.</p>
                    <div className="row">
                      <Btn icon={<Share2 size={16} />} onClick={() => friend.link && shareFriend(friend.link)}>Отправить ссылку</Btn>
                      <Btn variant="secondary" onClick={() => nav.push({ name: 'ticket', id: friend.booking.id })}>Пропуск друга</Btn>
                    </div>
                  </>
                )}
              </section>
            ) : (
              <Btn size="lg" variant="secondary" block loading={busy} icon={<UserPlus size={18} />} onClick={inviteFriend}>Позвать друга (+1 место)</Btn>
            )
          )}
          <Btn size="lg" block icon={<Navigation size={18} />} onClick={() => openExternal(routeUrl(b.venue.lat, b.venue.lon, nav.origin))}>Как добраться</Btn>
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

      {b.status === 'cancelled' && b.cancel_reason === 'studio' && (
        <section className="info-card">
          <p>Запись отменена администратором студии. Узнать подробности и подобрать другое время можно у студии.</p>
          {b.venue.phone && (
            <a className="link-row" href={`tel:${b.venue.phone.replace(/[^\d+]/g, '')}`}><Phone size={18} /> {b.venue.phone}</a>
          )}
        </section>
      )}

      {b.venue.booking_mode === 'instant' && (
        <Btn size="lg" variant="ghost" block icon={<MessageCircle size={18} />} onClick={() => nav.push({ name: 'chat', venueId: b.venue.id, venueName: b.venue.name })}>
          Написать студии
        </Btn>
      )}
    </div>
  )
}

// FriendLink — ссылка для друга на экране его пропуска (у пригласившего).
function FriendLink({ bookingId, onShare }: { bookingId: string; onShare: (link: string) => void }) {
  const [link, setLink] = useState('')
  useEffect(() => {
    if (!bookingId) return
    api.friend(bookingId).then((r) => setLink(r.friend?.link ?? '')).catch(() => setLink(''))
  }, [bookingId])
  if (!link) return null
  return <Btn icon={<Share2 size={16} />} onClick={() => onShare(link)}>Отправить ссылку другу</Btn>
}
