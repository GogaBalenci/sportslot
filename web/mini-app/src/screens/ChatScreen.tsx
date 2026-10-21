import { useEffect, useState } from 'react'
import { api, type Message } from '../api'
import { ChatView } from '../ChatView'
import { ErrorBlock, Loader } from '../components'
import { useNav } from '../nav'

// Переписка клиента со студией. Ответы студии приходят и сюда, и в чат с ботом.
export function ChatScreen({ venueId, venueName }: { venueId: string; venueName: string }) {
  const nav = useNav()
  const [messages, setMessages] = useState<Message[] | null>(null)
  const [error, setError] = useState('')

  const load = () =>
    api
      .clientMessages(venueId)
      .then((r) => setMessages(r.messages))
      .catch((e) => setError(e.message))

  useEffect(() => {
    void load()
    const t = window.setInterval(load, 15000)
    return () => window.clearInterval(t)
  }, [venueId])

  if (error) return <div className="screen"><ErrorBlock text={error} /></div>
  if (!messages) return <div className="screen"><Loader /></div>
  return (
    <div className="screen">
      <header className="hero">
        <h1 className="h1">{venueName}</h1>
        <p className="muted small">Студия получит сообщение и ответит сюда и в чат с ботом.</p>
      </header>
      <ChatView
        messages={messages}
        mine="client"
        placeholder="Вопрос студии"
        empty="Спросите про занятие: что взять, есть ли душ, можно ли прийти с другом."
        onSend={async (text) => {
          try {
            const m = await api.sendClientMessage(venueId, text)
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
