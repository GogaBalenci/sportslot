import { useEffect, useState } from 'react'
import { Button, Panel } from '@maxhub/max-ui'
import { createBooking, findSlots, type VenueSlot } from './api'
import { maxUserID, maxUserName } from './max'

const sports = [
  { value: 'boxing', label: 'Бокс' },
  { value: 'yoga', label: 'Йога' },
  { value: 'football', label: 'Футбол' },
]

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

export default function App() {
  const searchParams = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : null
  const initialSport = searchParams?.get('sport') || 'boxing'
  const [sport, setSport] = useState(initialSport)
  const [slots, setSlots] = useState<VenueSlot[]>([])
  const [loading, setLoading] = useState(true)
  const [bookingSlotID, setBookingSlotID] = useState<string | null>(null)
  const [message, setMessage] = useState('')
  const [statusType, setStatusType] = useState<'success' | 'error' | ''>('')
  const userID = maxUserID()
  const userName = maxUserName()

  useEffect(() => {
    let active = true
    setLoading(true)
    setMessage('')
    setStatusType('')
    findSlots(userID, sport)
      .then(({ results }) => active && setSlots(results))
      .catch((error: Error) => {
        if (active) {
          setMessage(error.message)
          setStatusType('error')
        }
      })
      .finally(() => active && setLoading(false))
    return () => { active = false }
  }, [sport, userID])

  async function book(slotID: string) {
    setBookingSlotID(slotID)
    setMessage('')
    setStatusType('')
    try {
      await createBooking(userID, slotID)
      setMessage('Место забронировано. Подтверждение придёт в чат MAX.')
      setStatusType('success')
      setSlots((current) => current.map((item) => item.slot.slot_id === slotID
        ? { ...item, slot: { ...item.slot, quota_available: Math.max(0, item.slot.quota_available - 1) } }
        : item))
    } catch (error) {
      const errMsg = error instanceof Error ? error.message : 'Не удалось создать бронь'
      setMessage(errMsg)
      setStatusType('error')
      if (errMsg.toLowerCase().includes('квота') || errMsg.toLowerCase().includes('исчерпана')) {
        setSlots((current) => current.map((item) => item.slot.slot_id === slotID
          ? { ...item, slot: { ...item.slot, quota_available: 0 } }
          : item))
      }
    } finally {
      setBookingSlotID(null)
    }
  }

  return (
    <main className="page">
      <header className="hero">
        <p className="eyebrow">СпортСлот в MAX</p>
        <h1>Тренировка рядом — в подходящее время</h1>
        <p>{userName ? `Подбираем варианты для ${userName}.` : 'Выберите спорт и забронируйте свободное место.'}</p>
      </header>

      <section className="filters" aria-label="Выбор вида спорта">
        {sports.map((item) => (
          <button key={item.value} className={sport === item.value ? 'filter active' : 'filter'} onClick={() => setSport(item.value)}>
            {item.label}
          </button>
        ))}
      </section>

      {message && <div className={statusType === 'error' ? 'notice error' : 'notice'} role="status">{message}</div>}
      {loading && <p className="status">Ищем свободные тренировки…</p>}
      {!loading && !message && slots.length === 0 && <p className="status">Подходящих свободных слотов пока нет.</p>}

      <section className="cards" aria-label="Найденные площадки">
        {slots.map((item) => (
          <Panel key={item.slot.slot_id} className="card">
            <div className="card-content">
              <div>
                <p className="card-meta">{item.level === 'beginner' ? 'Для начинающих' : item.level}</p>
                <h2>{item.name}</h2>
                <p>{item.address}</p>
              </div>
              <div className="slot-info">
                <strong>{formatDate(item.slot.start_at)}</strong>
                <span className={item.slot.quota_available ? 'places' : 'places sold-out'}>
                  {item.slot.quota_available ? `Свободно мест: ${item.slot.quota_available}` : 'Мест нет'}
                </span>
              </div>
              <Button
                disabled={!item.slot.quota_available || bookingSlotID !== null}
                loading={bookingSlotID === item.slot.slot_id}
                onClick={() => book(item.slot.slot_id)}
              >
                Забронировать
              </Button>
            </div>
          </Panel>
        ))}
      </section>
    </main>
  )
}
