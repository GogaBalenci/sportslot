import { useEffect, useState } from 'react'
import { api, type Message } from '../api'
import { ChatView } from '../ChatView'
import { ErrorBlock, Loader } from '../components'
import { useNav } from '../nav'

export function StudioThreadScreen({ venueId, userId }: { venueId: string; userId: string }) {
  const nav = useNav()
  const [name, setName] = useState('')
  const [messages, setMessages] = useState<Message[] | null>(null)
  const [error, setError] = useState('')

  const load = () =>
    api
      .thread(venueId, userId)
      .then((r) => {
        setName(r.user.name)
        setMessages(r.messages)
      })
      .catch((e) => setError(e.message))

  useEffect(() => {
    void load()
    const t = window.setInterval(load, 10000)
    return () => window.clearInterval(t)
  }, [venueId, userId])

  if (error) return <div className="screen"><ErrorBlock text={error} /></div>
  if (!messages) return <div className="screen"><Loader /></div>
  return (
    <div className="screen">
      <header className="hero">
        <h1 className="h1">{name || 'Клиент'}</h1>
        <p className="muted small">Ответ придёт клиенту в чат с ботом СпортСлот.</p>
      </header>
      <ChatView
        messages={messages}
        mine="studio"
        placeholder="Ответ клиенту"
        empty="Сообщений пока нет. Напишите клиенту первым — например, уточните детали занятия."
        onSend={async (text) => {
          try {
            const m = await api.reply(venueId, userId, text)
            setMessages((prev) => [...(prev ?? []), m])
          } catch (e) {
            nav.toast(e instanceof Error ? e.message : 'Не получилось отправить')
            throw e
          }
        }}
      />
    </div>
  )
}

// StudioMessageScreen открывается из уведомления о новом сообщении и ведёт в переписку.
export function StudioMessageScreen({ id }: { id: string }) {
  const nav = useNav()
  const [error, setError] = useState('')
  useEffect(() => {
    api
      .messageRef(id)
      .then((r) => nav.tab({ name: 'studioThread', venueId: r.venue_id, userId: r.user_id }))
      .catch((e) => setError(e.message))
  }, [id])
  return <div className="screen">{error ? <ErrorBlock text={error} /> : <Loader />}</div>
}
