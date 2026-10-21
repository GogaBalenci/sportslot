import { useEffect, useRef, useState } from 'react'
import { Send } from 'lucide-react'
import type { Message } from './api'
import { Btn } from './components'
import { dayTitle, time } from './format'

// ChatView — переписка клиента и студии. mine — чьи сообщения справа.
export function ChatView(props: {
  messages: Message[]
  mine: 'client' | 'studio'
  onSend: (text: string) => Promise<void>
  placeholder: string
  empty: string
}) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const bottom = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottom.current?.scrollIntoView({ block: 'end' })
  }, [props.messages.length])

  async function send() {
    const value = text.trim()
    if (!value) return
    setBusy(true)
    try {
      await props.onSend(value)
      setText('')
    } finally {
      setBusy(false)
    }
  }

  let lastDay = ''
  return (
    <div className="chat">
      <div className="chat__list">
        {props.messages.length === 0 && <div className="chat__empty">{props.empty}</div>}
        {props.messages.map((m) => {
          const day = dayTitle(m.created_at)
          const showDay = day !== lastDay
          lastDay = day
          return (
            <div key={m.id}>
              {showDay && <div className="chat__day">{day}</div>}
              <div className={`bubble${m.author === props.mine ? ' bubble--mine' : ''}`}>
                <span>{m.text}</span>
                <small>{time(m.created_at)}</small>
              </div>
            </div>
          )
        })}
        <div ref={bottom} />
      </div>
      <div className="chat__input">
        <textarea
          className="input chat__textarea"
          placeholder={props.placeholder}
          value={text}
          maxLength={2000}
          rows={1}
          onChange={(e) => setText(e.target.value)}
        />
        <Btn loading={busy} disabled={!text.trim()} icon={<Send size={18} />} onClick={send} aria-label="Отправить" />
      </div>
    </div>
  )
}
