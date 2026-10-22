import { useEffect, useState } from 'react'
import { Copy, ShieldCheck } from 'lucide-react'
import { api, type Payment, type StudioSettings } from '../api'
import { Btn, Chips, ErrorBlock, Loader, Toggle } from '../components'
import { dayTitle, time } from '../format'
import { copyText } from '../max'
import { useNav } from '../nav'

const rub = (kop: number) => `${(kop / 100).toLocaleString('ru-RU')} ₽`

const guide = {
  yookassa: {
    account: 'shopId',
    secret: 'Секретный ключ',
    steps: [
      'Личный кабинет ЮKassa → «Интеграция» → «Ключи API»: скопируйте shopId и выпустите секретный ключ.',
      'Там же «HTTP-уведомления»: вставьте адрес ниже и отметьте события payment.succeeded и payment.canceled.',
      'Для проверки без денег создайте тестовый магазин в ЮKassa и введите его shopId и тестовый ключ.',
    ],
  },
  tbank: {
    account: 'TerminalKey',
    secret: 'Пароль терминала',
    steps: [
      'Т-Бизнес → «Интернет-эквайринг» → «Магазины» → «Терминалы»: скопируйте TerminalKey и пароль.',
      'Уведомления настраивать не нужно: адрес передаётся в каждом платеже. Если в терминале задан свой адрес, укажите адрес ниже.',
      'Для проверки используйте тестовый терминал (TerminalKey с окончанием DEMO).',
    ],
  },
}

// Онлайн-оплата: магазин студии в ЮKassa или Т-Кассе. Деньги идут напрямую студии.
export function PaymentsTab({ venueId, readOnly }: { venueId: string; readOnly: boolean }) {
  const nav = useNav()
  const [st, setSt] = useState<StudioSettings | null>(null)
  const [paymentsList, setPayments] = useState<{ list: Payment[]; month: { count: number; amount: number } } | null>(null)
  const [error, setError] = useState('')
  const [provider, setProvider] = useState<'yookassa' | 'tbank'>('yookassa')
  const [account, setAccount] = useState('')
  const [secret, setSecret] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [trial, setTrial] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = () => {
    setError('')
    api
      .studioSettings(venueId)
      .then((s) => {
        setSt(s)
        setProvider(s.payment_provider || 'yookassa')
        setAccount(s.payment_account)
        setEnabled(s.payments_enabled)
        setTrial(s.trial_online)
        setSecret('')
      })
      .catch((e) => setError(e.message))
    api.studioCommerce(venueId).then((c) => setPayments({ list: c.payments, month: c.month })).catch(() => undefined)
  }
  useEffect(load, [venueId])

  async function save() {
    setBusy(true)
    try {
      const saved = await api.saveStudioSettings(venueId, {
        payment_provider: provider, payment_account: account, payment_secret: secret || undefined,
        payments_enabled: enabled, trial_online: trial,
      })
      setSt(saved)
      setSecret('')
      nav.toast(saved.payments_enabled ? 'Оплата онлайн включена' : 'Настройки сохранены')
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось сохранить')
    } finally {
      setBusy(false)
    }
  }

  async function check() {
    setBusy(true)
    try {
      await api.checkPayments(venueId)
      nav.toast('ЮKassa приняла ключи — всё подключено')
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Проверка не прошла')
    } finally {
      setBusy(false)
    }
  }

  if (error) return <ErrorBlock text={error} onRetry={load} />
  if (!st) return <Loader />
  const g = guide[provider]
  const webhook = provider === 'yookassa' ? st.yookassa_webhook : st.tbank_webhook
  const switched = provider !== (st.payment_provider || 'yookassa')

  return (
    <>
      <section className="info-card">
        <h2 className="h2">Онлайн-оплата</h2>
        <p className="muted small">Клиенты оплачивают пробное занятие и абонементы картой или через СБП. Деньги приходят напрямую на счёт студии, СпортСлот их не касается.</p>
        {!st.encryption_ready && <div className="note note--warning">Сервер пока не настроен для хранения ключей (PAYMENT_SECRET_KEY). Сохранить ключ не получится.</div>}
        {readOnly && <div className="note">В демо-доступе настройки только просматриваются.</div>}
        <Chips items={[{ id: 'yookassa', title: 'ЮKassa' }, { id: 'tbank', title: 'Т-Касса' }]} value={provider} onChange={setProvider} tone="navy" />
        <ol className="steps">{g.steps.map((s) => <li key={s}>{s}</li>)}</ol>
        <label className="field"><span>{g.account}</span>
          <input className="input" disabled={readOnly} value={account} onChange={(e) => setAccount(e.target.value)} autoComplete="off" /></label>
        <label className="field"><span>{g.secret}</span>
          <input className="input" disabled={readOnly} type="password" value={secret} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password"
            placeholder={st.secret_saved && !switched ? `сохранён ${st.payment_secret_hint}` : ''} /></label>
        <div className="field"><span>Адрес для уведомлений</span>
          <div className="invite-link">{webhook}</div>
          <Btn variant="secondary" icon={<Copy size={16} />} onClick={async () => nav.toast((await copyText(webhook)) ? 'Скопировано' : 'Выделите адрес вручную')}>Скопировать адрес</Btn>
        </div>
        <Toggle checked={enabled} disabled={readOnly} onChange={setEnabled} label="Принимать оплату онлайн" hint="Абонементы в карточке студии можно будет купить сразу" />
        <Toggle checked={trial} disabled={readOnly} onChange={setTrial} label="Пробное занятие — тоже онлайн" hint="Кнопка «Оплатить» в подтверждении записи; оплатить на месте по-прежнему можно" />
        {!readOnly && (
          <div className="stack">
            <Btn size="lg" block loading={busy} onClick={save}>Сохранить</Btn>
            {provider === 'yookassa' && st.secret_saved && !switched && (
              <Btn variant="secondary" block icon={<ShieldCheck size={18} />} loading={busy} onClick={check}>Проверить ключи</Btn>
            )}
          </div>
        )}
        <p className="muted small">Ключ хранится на сервере в зашифрованном виде и больше нигде не показывается. Если в магазине включены чеки ЮKassa или онлайн-касса Т-Бизнеса, проверьте настройки чеков в личном кабинете.</p>
      </section>

      {paymentsList && (
        <>
          <h2 className="h2 section">Оплаты за 30 дней: {paymentsList.month.count} на {paymentsList.month.amount.toLocaleString('ru-RU')} ₽</h2>
          {paymentsList.list.length > 0 && (
            <div className="list">
              {paymentsList.list.map((p) => (
                <div key={p.id} className="list__row list__row--static">
                  <div className="list__main">
                    <div className="list__title">{p.user_name || 'Клиент'} · {rub(p.amount)}</div>
                    <div className="muted small">{p.description}</div>
                  </div>
                  <span className="muted small">{p.paid_at ? `${dayTitle(p.paid_at).split(',')[0]} ${time(p.paid_at)}` : ''}</span>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </>
  )
}
