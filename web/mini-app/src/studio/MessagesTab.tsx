import { useEffect, useState } from 'react'
import { api, type Thread } from '../api'
import { Empty, ErrorBlock, Loader } from '../components'
import { dayTitle, time } from '../format'
import { useNav } from '../nav'

export function MessagesTab({ venueId, onRead }: { venueId: string; onRead: () => void }) {
  const nav = useNav()
  const [items, setItems] = useState<Thread[] | null>(null)
  const [error, setError] = useState('')
  const load = () => {
    setError('')
    api.threads(venueId).then((r) => setItems(r.threads)).catch((e) => setError(e.message))
  }
  useEffect(load, [venueId])

  if (error) return <ErrorBlock text={error} onRetry={load} />
  if (!items) return <Loader />
  if (items.length === 0) {
    return <Empty title="Сообщений пока нет" text="Клиенты пишут студии из бота или из мини-приложения. О новом сообщении придёт уведомление в бот." />
  }
  return (
    <div className="list">
      {items.map((t) => (
        <button
          key={t.user_id}
          className="list__row"
          onClick={() => {
            onRead()
            nav.push({ name: 'studioThread', venueId, userId: t.user_id })
          }}
        >
          <div className="list__main">
            <div className="list__title">{t.user_name || 'Клиент'}</div>
            <div className="muted small thread-preview">{t.last_by === 'studio' ? 'Вы: ' : ''}{t.last_text}</div>
          </div>
          <div className="thread-meta">
            <span className="muted small">{dayTitle(t.last_at).split(',')[0]} {time(t.last_at)}</span>
            {t.unread > 0 && <span className="counter">{t.unread}</span>}
          </div>
        </button>
      ))}
    </div>
  )
}
