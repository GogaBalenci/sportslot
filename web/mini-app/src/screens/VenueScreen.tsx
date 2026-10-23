import { useEffect, useMemo, useState } from 'react'
import { Globe, MapPin, MessageCircle, Navigation, Phone } from 'lucide-react'
import { api, type Booking, type Pass, type ReviewSummary, type SlotInfo, type UserPass, type VenueDetails } from '../api'
import { AccessBadge, BookingSheet, Btn, Empty, ErrorBlock, Loader, Sheet, SlotPill, SportBadge, Stars, VenueLogo, districtTitle } from '../components'
import { catalogDate, dayKey, dayNumber, dayTitle, hours, nextDays, routeUrl, trial } from '../format'
import { haptic, openExternal } from '../max'
import { useNav } from '../nav'

export function VenueScreen({ id, reschedule }: { id: string; reschedule?: string }) {
  const nav = useNav()
  const [venue, setVenue] = useState<VenueDetails | null>(null)
  const [mine, setMine] = useState<Booking[]>([])
  const [error, setError] = useState('')
  const [picked, setPicked] = useState<SlotInfo | null>(null)
  const [day, setDay] = useState('')

  const load = () => {
    setError('')
    api.venue(id).then(setVenue).catch((e) => setError(e.message))
    api.me().then((r) => setMine(r.bookings.filter((b) => b.status === 'confirmed'))).catch(() => setMine([]))
  }
  useEffect(load, [id])

  const byDay = useMemo(() => {
    const map = new Map<string, SlotInfo[]>()
    venue?.all_slots.forEach((s) => {
      const key = dayKey(s.start_at)
      map.set(key, [...(map.get(key) ?? []), s])
    })
    return map
  }, [venue])

  const days = useMemo(() => nextDays(14), [])
  const bookedSlots = new Set(mine.map((b) => b.slot_id))
  const bookedDays = new Set(mine.filter((b) => b.venue.id === id).map((b) => dayKey(b.slot.start_at)))
  const activeDay = day || days.find((d) => byDay.has(d.key))?.key || ''

  if (error) return <div className="screen"><ErrorBlock text={error} onRetry={load} /></div>
  if (!venue) return <div className="screen"><Loader /></div>

  const website = venue.website?.startsWith('http') ? venue.website : ''
  const place = [districtTitle(venue.district, nav.catalog?.districts), venue.address].filter(Boolean).join(' · ')
  const slots = byDay.get(activeDay) ?? []

  function pick(s: SlotInfo) {
    const booking = mine.find((b) => b.slot_id === s.slot_id)
    if (booking && !reschedule) {
      nav.push({ name: 'ticket', id: booking.id })
      return
    }
    setPicked(s)
  }

  return (
    <div className="screen">
      <header className="venue-hero">
        <div className="card__badges">
          {venue.sports.filter((s) => s !== 'multi').map((s) => <SportBadge key={s} sport={s} />)}
          <AccessBadge venue={venue} />
        </div>
        <div className="venue-title">
          {venue.logo_url && <VenueLogo venue={venue} size={56} />}
          <div>
            <h1 className="h1">{venue.name}</h1>
            {venue.reviews && venue.reviews.count > 0 && <Stars average={venue.reviews.average} count={venue.reviews.count} />}
          </div>
        </div>
        {venue.description && <p className="venue-about">{venue.description}</p>}
        {place && <div className="meta"><MapPin size={14} /><span>{place}</span></div>}
        <div className="row row--wrap">
          <Btn variant="secondary" icon={<Navigation size={18} />} onClick={() => openExternal(routeUrl(venue.lat, venue.lon, nav.origin))}>Маршрут</Btn>
          {website && <Btn variant="secondary" icon={<Globe size={18} />} onClick={() => openExternal(website)}>Сайт</Btn>}
          {venue.phone && (
            <a className="btn btn--secondary btn--md" href={`tel:${venue.phone.replace(/[^+\d]/g, '')}`}>
              <Phone size={18} /><span>Позвонить</span>
            </a>
          )}
          {venue.booking_mode === 'instant' && (
            <Btn variant="secondary" icon={<MessageCircle size={18} />} onClick={() => nav.push({ name: 'chat', venueId: venue.id, venueName: venue.name })}>
              Написать
            </Btn>
          )}
        </div>
      </header>

      {reschedule && <div className="note">Выбери новое время — код входа останется прежним.</div>}

      {venue.booking_mode === 'instant' ? (
        <>
          {(trial(venue.trial_price) || venue.what_to_bring) && (
            <div className="info-card">
              {trial(venue.trial_price) && <div className="trial">{trial(venue.trial_price)}{venue.online_payment?.trial ? ' · можно оплатить онлайн' : ''}</div>}
              {venue.what_to_bring && <p className="muted">Что взять: {venue.what_to_bring}</p>}
            </div>
          )}

          <h2 className="h2 section">Расписание</h2>
          <div className="dates">
            {days.map((d) => {
              const n = dayNumber(d.iso)
              const has = byDay.has(d.key)
              return (
                <button
                  key={d.key}
                  className={`date${d.key === activeDay ? ' date--active' : ''}${has ? '' : ' date--empty'}`}
                  disabled={!has}
                  onClick={() => {
                    haptic('tap')
                    setDay(d.key)
                  }}
                >
                  <span className="date__wd">{n.weekday}</span>
                  <span className="date__day">{n.day}</span>
                  {bookedDays.has(d.key) && <i className="date__dot" />}
                </button>
              )
            })}
          </div>

          {slots.length === 0 ? (
            <Empty title="На ближайшие две недели занятий нет" />
          ) : (
            <>
              <div className="day-title">{dayTitle(slots[0].start_at)}</div>
              <div className="slot-list">
                {slots.map((s) => (
                  <div key={s.slot_id} className="slot-line">
                    <SlotPill slot={s} booked={bookedSlots.has(s.slot_id)} onClick={() => pick(s)} />
                    <button className="slot-line__title" onClick={() => pick(s)}>{s.title}</button>
                  </div>
                ))}
              </div>
            </>
          )}
          {venue.passes && venue.passes.length > 0 && <PassesSection venue={venue} />}
          {venue.reviews && venue.reviews.count > 0 && <ReviewsSection reviews={venue.reviews} />}
          {venue.source === 'demo_partner' && (
            <p className="muted small">Демо-партнёр: студия вымышлена, адрес условный, расписание модельное. На ней можно пройти весь путь — от записи до отметки посещения по QR.</p>
          )}
        </>
      ) : (
        <div className="info-card">
          {venue.opening_hours && <p><b>Часы работы:</b> {hours(venue.opening_hours)}</p>}
          {venue.phone && <p><b>Телефон:</b> {venue.phone}</p>}
          <p>Онлайн-записи здесь пока нет. Расписание и стоимость пробного занятия уточни у администратора зала.</p>
          {venue.verified_at && <p className="muted small">Информация о зале обновлена {catalogDate(venue.verified_at)}</p>}
        </div>
      )}

      {picked && <BookingSheet venue={venue} slot={picked} reschedule={reschedule} onClose={() => setPicked(null)} />}
    </div>
  )
}

const visitsLabel = (n: number) =>
  `${n} ${n % 10 === 1 && n % 100 !== 11 ? 'занятие' : [2, 3, 4].includes(n % 10) && ![12, 13, 14].includes(n % 100) ? 'занятия' : 'занятий'}`

// Абонементы студии. Покупка — в два шага: сначала условия, потом подтверждение,
// чтобы случайное нажатие не отправляло заявку студии.
function PassesSection({ venue }: { venue: VenueDetails }) {
  const nav = useNav()
  const [mine, setMine] = useState<UserPass[]>([])
  const [picked, setPicked] = useState<Pass | null>(null)
  const [busy, setBusy] = useState(false)
  const online = !!venue.online_payment?.passes

  const loadMine = () => {
    api
      .myPasses()
      .then((r) => setMine(r.passes.filter((p) => p.venue_id === venue.id)))
      .catch(() => setMine([]))
  }
  useEffect(loadMine, [venue.id])

  const request = mine.find((p) => p.status === 'requested')
  const active = mine.find((p) => p.status === 'active')

  async function confirm(p: Pass) {
    setBusy(true)
    try {
      const r = await api.buyPass(p.id)
      setPicked(null)
      loadMine()
      if (r.payment?.confirmation_url) {
        openExternal(r.payment.confirmation_url)
        nav.push({ name: 'payment', id: r.payment.id })
      } else if (r.payment_error) {
        nav.toast('Платёжная система не ответила. Заявка сохранена — оплатить можно позже в «Моих занятиях»')
      } else {
        haptic('success')
        nav.toast(r.created ? 'Заявка отправлена студии' : 'Заявка уже отправлена — студия свяжется с тобой')
      }
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось оформить абонемент')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <h2 className="h2 section">Абонементы</h2>
      {active && (
        <div className="note note--mint">🎟 У тебя активен «{active.title}»: осталось {active.visits_left} из {active.visits_total}. Занятия спишутся автоматически при записи.</div>
      )}
      {request && !online && (
        <div className="note">Заявка на «{request.title}» отправлена студии. Оплати на месте — абонемент включится после подтверждения. Отменить заявку можно в «Моих занятиях».</div>
      )}
      <div className="stack">
        {venue.passes!.map((p) => {
          const requested = request?.pass_id === p.id
          return (
            <div key={p.id} className="pass-card">
              <div className="pass-card__main">
                <b>{p.title}</b>
                <span className="muted small">{visitsLabel(p.visits)} · действует {p.valid_days} дн.</span>
              </div>
              <Btn variant={requested ? 'secondary' : 'primary'} disabled={!!request && !requested && !online} onClick={() => setPicked(p)}>
                {requested ? (online ? 'Оплатить' : 'Заявка отправлена') : `${p.price.toLocaleString('ru-RU')} ₽`}
              </Btn>
            </div>
          )
        })}
      </div>
      {picked && (
        <Sheet onClose={() => !busy && setPicked(null)}>
          <h2 className="h2">{picked.title}</h2>
          <div className="summary">
            <div className="summary__row"><span><b>{picked.price.toLocaleString('ru-RU')} ₽</b> · {visitsLabel(picked.visits)}</span></div>
            <div className="summary__row"><span>{venue.name}<small>{venue.address}</small></span></div>
          </div>
          <ul className="rules">
            <li>Действует {picked.valid_days} дн. с момента {online ? 'оплаты' : 'подтверждения студией'}.</li>
            <li>Визит списывается при записи на занятие. Отменишь запись — занятие вернётся в абонемент.</li>
            <li>Неявка без отмены сгорает так же, как в студии.</li>
            {online ? (
              <li>Оплата картой или через СБП на странице студии — деньги получает студия. Возврат — по правилам студии.</li>
            ) : (
              <li>Студия получит заявку и свяжется с тобой. Оплата — на месте, после неё абонемент включится. В одной студии — одна заявка за раз.</li>
            )}
          </ul>
          <div className="stack">
            <Btn size="lg" block loading={busy} onClick={() => confirm(picked)}>
              {online ? `Перейти к оплате · ${picked.price.toLocaleString('ru-RU')} ₽` : 'Отправить заявку студии'}
            </Btn>
            <Btn size="lg" variant="ghost" block disabled={busy} onClick={() => setPicked(null)}>Не сейчас</Btn>
          </div>
        </Sheet>
      )}
    </>
  )
}

function ReviewsSection({ reviews }: { reviews: ReviewSummary }) {
  return (
    <>
      <h2 className="h2 section">Отзывы <Stars average={reviews.average} count={reviews.count} /></h2>
      <div className="stack">
        {reviews.items?.filter((r) => r.text).slice(0, 5).map((r, i) => (
          <div key={i} className="review-card">
            <div className="review-card__head"><b>{r.author}</b><span className="review-card__stars">{'★'.repeat(r.rating)}</span></div>
            <p>{r.text}</p>
          </div>
        ))}
      </div>
    </>
  )
}
