import { useEffect, useState } from 'react'
import { Bell, LogOut, ScanLine } from 'lucide-react'
import { ApiError, api, hasPartnerToken, setPartnerToken, type Booking } from '../api'
import { Btn, Empty, ErrorBlock, Loader } from '../components'
import { dayKey, dayTitle, time } from '../format'
import { canScanQR, haptic, scanQR } from '../max'
import { useNav } from '../nav'

const status: Record<Booking['status'], string> = {
  confirmed: 'Ждём',
  attended: 'Пришёл',
  no_show: 'Не пришёл',
  cancelled: 'Отменил',
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
      <header className="hero">
        <h1 className="h1">Кабинет студии</h1>
        <p className="muted">Отмечайте посещения по QR и смотрите, кто записан на неделю.</p>
      </header>
      <input className="input" placeholder="Код доступа" value={codeValue} onChange={(e) => setCode(e.target.value)} />
      {error && <div className="error">{error}</div>}
      <Btn size="lg" block loading={busy} disabled={!codeValue.trim()} onClick={submit}>Войти</Btn>
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
      setResult({ ok: true, text: `${b.user_name || 'Гость'} отмечен: ${b.slot.title}, ${dayTitle(b.slot.start_at).toLowerCase()} в ${time(b.slot.start_at)}. Клиенту ушёл вопрос «Как прошло?».` })
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
      <header className="hero">
        <h1 className="h1">Кабинет студии</h1>
        <p className="muted">Отметка посещений и записи на неделю</p>
      </header>

      <section className="checkin">
        {canScanQR() && <Btn size="lg" block loading={busy} icon={<ScanLine size={20} />} onClick={scan}>Сканировать QR-пропуск</Btn>}
        <div className="row">
          <input className="input" placeholder="Код из 6 цифр" inputMode="numeric" value={manual} onChange={(e) => setManual(e.target.value)} />
          <Btn variant="secondary" loading={busy} disabled={manual.replace(/\D/g, '').length !== 6} onClick={() => checkIn(manual)}>Отметить</Btn>
        </div>
        {result && <div className={result.ok ? 'success success--compact' : 'error'}>{result.text}</div>}
      </section>

      {error && <ErrorBlock text={error} onRetry={load} />}
      {!error && !items && <Loader />}
      {items && items.length === 0 && <Empty title="На ближайшую неделю записей нет" />}
      {groups.map((g) => (
        <section key={g.key}>
          <h2 className="h2 section">{g.title}</h2>
          <div className="list">
            {g.items.map((b) => (
              <div key={b.id} className="list__row list__row--static">
                <b className="list__time">{time(b.slot.start_at)}</b>
                <div className="list__main">
                  <div className="list__title">{b.user_name || 'Гость'}</div>
                  <div className="muted small">{b.slot.title} · {b.venue.name}</div>
                </div>
                {b.status === 'confirmed' ? (
                  <Btn variant="secondary" icon={<Bell size={16} />} onClick={() => remind(b.id)}>Напомнить</Btn>
                ) : (
                  <span className={`status status--${b.status}`}>{status[b.status]}</span>
                )}
              </div>
            ))}
          </div>
        </section>
      ))}
      <Btn variant="ghost" block icon={<LogOut size={18} />} onClick={() => { setPartnerToken(''); setLogged(false) }}>Выйти</Btn>
    </div>
  )
}
