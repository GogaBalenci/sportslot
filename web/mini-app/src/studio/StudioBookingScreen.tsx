import { useEffect, useState } from 'react'
import { Bell, MessageCircle } from 'lucide-react'
import { api, type ClientCard } from '../api'
import { Btn, ErrorBlock, Loader } from '../components'
import { code, dayTitle, time } from '../format'
import { useNav } from '../nav'

const statusText = { confirmed: 'Записан', attended: 'Пришёл', no_show: 'Не пришёл', cancelled: 'Отменено' } as const

// Карточка клиента для администратора студии.
export function StudioBookingScreen({ id }: { id: string }) {
  const nav = useNav()
  const [card, setCard] = useState<ClientCard | null>(null)
  const [error, setError] = useState('')
  const load = () => {
    setError('')
    api.studioBooking(id).then(setCard).catch((e) => setError(e.message))
  }
  useEffect(load, [id])

  async function remind() {
    try {
      await api.studioRemind(id)
      nav.toast('Напоминание отправлено клиенту в чат')
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось отправить')
    }
  }

  if (error) return <div className="screen"><ErrorBlock text={error} onRetry={load} /></div>
  if (!card) return <div className="screen"><Loader /></div>
  const b = card.booking
  const active = b.status === 'confirmed'

  return (
    <div className="screen">
      <header className="hero">
        <span className={`status status--${b.status}`}>{statusText[b.status]}{b.cancel_reason === 'studio' ? ' студией' : ''}</span>
        <h1 className="h1">{b.user_name || 'Клиент'}</h1>
        <div className="muted">{b.venue.name}</div>
      </header>
      <section className="info-card">
        <div className="summary__row"><b>{b.slot.title}</b></div>
        <div>{dayTitle(b.slot.start_at)}, {time(b.slot.start_at)}–{time(b.slot.end_at)}</div>
        {active && <div className="muted">Код входа: <b>{code(b.checkin_code)}</b></div>}
      </section>
      <div className="tiles tiles--3">
        <div className="tile"><div className="tile__value">{card.history.length}</div><div className="tile__label">Записи</div></div>
        <div className="tile"><div className="tile__value">{card.visits}</div><div className="tile__label">Посещения</div></div>
        <div className="tile"><div className="tile__value">{card.no_shows}</div><div className="tile__label">Неявки</div></div>
      </div>
      <div className="stack">
        <Btn size="lg" block icon={<MessageCircle size={18} />} onClick={() => nav.push({ name: 'studioThread', venueId: b.venue.id, userId: b.user_id })}>
          Написать клиенту{card.messages.length ? ` · ${card.messages.length}` : ''}
        </Btn>
        {active && <Btn size="lg" variant="secondary" block icon={<Bell size={18} />} onClick={remind}>Напомнить о занятии</Btn>}
      </div>
      <p className="muted small">Сообщение придёт клиенту в чат с ботом. Профиль клиента в MAX открывается из уведомления о записи в боте — имя там ссылкой.</p>
      {card.history.length > 1 && (
        <>
          <h2 className="h2 section">История в студии</h2>
          <div className="list">
            {card.history.map((h) => (
              <div key={h.id} className="list__row list__row--static">
                <div className="list__main">
                  <div className="list__title">{h.slot.title}</div>
                  <div className="muted small">{dayTitle(h.slot.start_at)}, {time(h.slot.start_at)}</div>
                </div>
                <span className={`status status--${h.status}`}>{statusText[h.status]}{h.rating ? ` · ${h.rating}/5` : ''}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
