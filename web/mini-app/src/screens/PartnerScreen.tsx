import { useEffect, useState } from 'react'
import { Button, Input, Typography } from '@maxhub/max-ui'
import { ApiError, api, hasPartnerToken, setPartnerToken, type Booking } from '../api'
import { ErrorBlock, Loader } from '../components'
import { dayKey, dayTitle, time } from '../format'
import { canScanQR, haptic, scanQR } from '../max'
import { useNav } from '../nav'

const status: Record<Booking['status'], string> = {
  confirmed: 'ждём',
  attended: 'пришёл',
  no_show: 'не пришёл',
  cancelled: 'отменил',
}

function Login({ onDone }: { onDone: () => void }) {
  const [codeValue, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function submit() {
    setBusy(true)
    setError('')
    try {
      const { token } = await api.partnerLogin(codeValue.trim())
      setPartnerToken(token)
      onDone()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не получилось войти')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="screen">
      <header className="screen__header">
        <Typography.Headline variant="medium">Кабинет студии</Typography.Headline>
        <div className="muted">Отмечайте посещения по QR и смотрите, кто записан. В пилоте кабинет открыт для демо-партнёров.</div>
      </header>
      <Input placeholder="Код доступа" value={codeValue} onChange={(e) => setCode(e.target.value)} />
      {error && <div className="error">{error}</div>}
      <Button size="large" stretched loading={busy} disabled={!codeValue.trim()} onClick={submit}>Войти</Button>
    </div>
  )
}

export function PartnerScreen() {
  const nav = useNav()
  const [logged, setLogged] = useState(hasPartnerToken())
  const [items, setItems] = useState<Booking[] | null>(null)
  const [error, setError] = useState('')
  const [manual, setManual] = useState('')
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const load = () => {
    setError('')
    api
      .partnerBookings()
      .then((r) => setItems(r.bookings))
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) {
          setPartnerToken('')
          setLogged(false)
        } else setError(e.message)
      })
  }
  useEffect(() => {
    if (logged) load()
  }, [logged])

  if (!logged) return <Login onDone={() => setLogged(true)} />

  async function checkIn(raw: string) {
    if (!raw.trim()) return
    setBusy(true)
    setResult(null)
    try {
      const b = await api.partnerCheckIn(raw)
      haptic('success')
      setResult({ ok: true, text: `Отмечено: ${b.user_name || 'гость'} — ${b.slot.title}, ${dayTitle(b.slot.start_at).toLowerCase()} в ${time(b.slot.start_at)}. Клиенту ушёл вопрос «Как прошло?».` })
      setManual('')
      load()
    } catch (e) {
      haptic('error')
      setResult({ ok: false, text: e instanceof Error ? e.message : 'Не получилось отметить' })
    } finally {
      setBusy(false)
    }
  }

  async function scan() {
    try {
      const value = await scanQR()
      if (value) await checkIn(value)
    } catch {
      nav.toast('Сканер закрыт. Можно ввести код вручную')
    }
  }

  async function remind(id: string) {
    try {
      await api.partnerRemind(id)
      nav.toast('Напоминание отправлено клиенту в чат')
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось отправить')
    }
  }

  const groups: { key: string; title: string; items: Booking[] }[] = []
  items?.forEach((b) => {
    const key = dayKey(b.slot.start_at)
    let g = groups.find((x) => x.key === key)
    if (!g) groups.push((g = { key, title: dayTitle(b.slot.start_at), items: [] }))
    g.items.push(b)
  })

  return (
    <div className="screen">
      <header className="screen__header">
        <Typography.Headline variant="medium">Кабинет студии</Typography.Headline>
        <div className="muted">Демо-партнёры СпортСлота</div>
      </header>

      <div className="stack">
        {canScanQR() && <Button size="large" stretched loading={busy} onClick={scan}>Сканировать QR-пропуск</Button>}
        <div className="filters">
          <Input placeholder="Код из 6 цифр" inputMode="numeric" value={manual} onChange={(e) => setManual(e.target.value)} />
          <Button variant="secondary" loading={busy} disabled={manual.replace(/\D/g, '').length !== 6} onClick={() => checkIn(manual)}>
            Отметить
          </Button>
        </div>
        {result && <div className={result.ok ? 'notice notice--success' : 'error'}>{result.text}</div>}
      </div>

      {error && <ErrorBlock text={error} onRetry={load} />}
      {!error && !items && <Loader />}
      {items && items.length === 0 && <div className="empty">На ближайшую неделю записей нет.</div>}
      {groups.map((g) => (
        <section key={g.key}>
          <h3 className="section">{g.title}</h3>
          <div className="list">
            {g.items.map((b) => (
              <div key={b.id} className="partner-row">
                <b>{time(b.slot.start_at)}</b>
                <div className="partner-row__main">
                  <div>{b.user_name || 'Гость'} · {b.slot.title}</div>
                  <div className="muted small">{b.venue.name}</div>
                </div>
                {b.status === 'confirmed' ? (
                  <button className="text-button small" onClick={() => remind(b.id)}>Напомнить</button>
                ) : (
                  <span className={`status status--${b.status}`}>{status[b.status]}</span>
                )}
              </div>
            ))}
          </div>
        </section>
      ))}
      <button className="text-button" onClick={() => { setPartnerToken(''); setLogged(false) }}>Выйти</button>
    </div>
  )
}
