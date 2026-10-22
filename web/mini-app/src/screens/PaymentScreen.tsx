import { useEffect, useState } from 'react'
import { CheckCircle2, Clock3, XCircle } from 'lucide-react'
import { api, type Payment } from '../api'
import { Btn, ErrorBlock, Loader } from '../components'
import { openExternal } from '../max'
import { useNav } from '../nav'

// Статус оплаты: сюда возвращает платёжная страница, а пока она открыта — опрашиваем сервер.
export function PaymentScreen({ id }: { id: string }) {
  const nav = useNav()
  const [p, setP] = useState<Payment | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let stopped = false
    const started = Date.now()
    const poll = () =>
      api
        .payment(id)
        .then((pay) => {
          if (stopped) return
          setP(pay)
          if (pay.status === 'pending' && Date.now() - started < 15 * 60 * 1000) window.setTimeout(poll, 3000)
        })
        .catch((e) => !stopped && setError(e.message))
    void poll()
    return () => {
      stopped = true
    }
  }, [id])

  if (error) return <div className="screen"><ErrorBlock text={error} /></div>
  if (!p) return <div className="screen"><Loader /></div>
  const amount = `${(p.amount / 100).toLocaleString('ru-RU')} ₽`

  if (p.status === 'succeeded') {
    return (
      <div className="screen">
        <section className="success success--hero">
          <CheckCircle2 size={48} />
          <h1 className="h1">Оплачено {amount}</h1>
          <p className="muted">{p.description}. Подтверждение пришло в чат с ботом.</p>
        </section>
        {p.booking_id ? (
          <Btn size="lg" block onClick={() => nav.tab({ name: 'ticket', id: p.booking_id! })}>QR-пропуск</Btn>
        ) : (
          <Btn size="lg" block onClick={() => nav.tab({ name: 'venue', id: p.venue_id })}>Записаться на занятие</Btn>
        )}
      </div>
    )
  }
  if (p.status === 'canceled') {
    return (
      <div className="screen">
        <section className="info-card payment-state">
          <XCircle size={40} />
          <h1 className="h2">Оплата не прошла</h1>
          <p className="muted">Деньги не списаны. Попробуйте ещё раз или оплатите в студии.</p>
        </section>
        <Btn size="lg" block onClick={() => nav.tab({ name: 'my' })}>Мои занятия</Btn>
      </div>
    )
  }
  return (
    <div className="screen">
      <section className="info-card payment-state">
        <Clock3 size={40} />
        <h1 className="h2">Ждём оплату {amount}</h1>
        <p className="muted">{p.description}. Страница оплаты открыта в браузере: карта или СБП. Как только платёж пройдёт, здесь появится подтверждение.</p>
        <Loader />
      </section>
      {p.confirmation_url && (
        <Btn size="lg" variant="secondary" block onClick={() => openExternal(p.confirmation_url!)}>Открыть страницу оплаты</Btn>
      )}
    </div>
  )
}
