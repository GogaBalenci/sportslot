import { useEffect, useState } from 'react'
import { CheckCircle2 } from 'lucide-react'
import { api, type Venue } from '../api'
import { Btn, ErrorBlock, Loader } from '../components'
import { insideMax } from '../max'
import { useNav } from '../nav'

// Принятие ссылки-приглашения: пользователь MAX становится администратором студии.
export function InviteScreen({ token }: { token: string }) {
  const nav = useNav()
  const [venue, setVenue] = useState<Venue | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api.acceptInvite(token).then(setVenue).catch((e) => setError(e.message))
  }, [token])

  if (error) {
    return (
      <div className="screen">
        <ErrorBlock text={insideMax() ? error : 'Откройте ссылку-приглашение в приложении MAX.'} />
        <p className="muted small">Попросите команду СпортСлот прислать новую ссылку: она одноразовая и действует 7 дней.</p>
      </div>
    )
  }
  if (!venue) return <div className="screen"><Loader /></div>
  return (
    <div className="screen">
      <section className="success success--hero">
        <CheckCircle2 size={48} />
        <h1 className="h1">Вы администратор студии</h1>
        <p className="muted">«{venue.name}» подключена. Уведомления о новых записях и сообщениях клиентов будут приходить в чат с ботом СпортСлот.</p>
      </section>
      <div className="stack">
        <Btn size="lg" block onClick={() => nav.tab({ name: 'studio', venueId: venue.id, tab: 'schedule' })}>Настроить расписание</Btn>
        <Btn size="lg" variant="secondary" block onClick={() => nav.tab({ name: 'studio', venueId: venue.id, tab: 'help' })}>Как работать с кабинетом</Btn>
      </div>
    </div>
  )
}
