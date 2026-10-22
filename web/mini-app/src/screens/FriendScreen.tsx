import { useEffect, useState } from 'react'
import { CheckCircle2 } from 'lucide-react'
import { ApiError, api, type Booking } from '../api'
import { Btn, ErrorBlock, Loader } from '../components'
import { dayTitle, time } from '../format'
import { useNav } from '../nav'

// Друг открыл ссылку-приглашение: место переходит к нему со своим пропуском.
export function FriendScreen({ token }: { token: string }) {
  const nav = useNav()
  const [b, setB] = useState<Booking | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    api
      .claimFriend(token)
      .then(setB)
      .catch((e) => setError(e instanceof ApiError && e.status === 401 ? 'Откройте приглашение в приложении MAX.' : e.message))
  }, [token])

  if (error) {
    return (
      <div className="screen">
        <ErrorBlock text={error} />
        <Btn block onClick={() => nav.tab({ name: 'search' })}>Найти тренировку</Btn>
      </div>
    )
  }
  if (!b) return <div className="screen"><Loader /></div>
  return (
    <div className="screen">
      <section className="success success--hero">
        <CheckCircle2 size={48} />
        <h1 className="h1">Ты записан вместе с другом</h1>
        <p className="muted">{b.slot.title}, {dayTitle(b.slot.start_at)} в {time(b.slot.start_at)}. {b.venue.name}, {b.venue.address}.</p>
      </section>
      <Btn size="lg" block onClick={() => nav.tab({ name: 'ticket', id: b.id, fresh: true })}>Мой QR-пропуск</Btn>
      <p className="muted small">Напомним в чате с ботом СпортСлот за сутки и за 2 часа.</p>
    </div>
  )
}
