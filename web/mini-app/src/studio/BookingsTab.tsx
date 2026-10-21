import { useEffect, useState } from 'react'
import { Bell, ChevronRight, ScanLine } from 'lucide-react'
import { api, type Booking } from '../api'
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

// CheckInBox — отметка посещения: сканер QR в мобильном MAX или ввод кода.
export function CheckInBox({ onDone }: { onDone?: () => void }) {
  const nav = useNav()
  const [manual, setManual] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null)

  async function checkIn(raw: string) {
    if (!raw.trim()) return
    setBusy(true)
    setResult(null)
    try {
      const b = await api.studioCheckIn(raw)
      haptic('success')
      if (b.training) {
        setResult({ ok: true, text: b.message ?? 'Учебный пропуск отсканирован' })
      } else {
        setResult({ ok: true, text: `${b.user_name || 'Гость'} отмечен: ${b.slot.title}, ${dayTitle(b.slot.start_at).toLowerCase()} в ${time(b.slot.start_at)}. Клиенту ушёл вопрос «Как прошло?».` })
        onDone?.()
      }
      setManual('')
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

  return (
    <section className="checkin">
      {canScanQR() && <Btn size="lg" block loading={busy} icon={<ScanLine size={20} />} onClick={scan}>Сканировать QR-пропуск</Btn>}
      <div className="row">
        <input className="input" placeholder="Код из 6 цифр" inputMode="numeric" value={manual} onChange={(e) => setManual(e.target.value)} />
        <Btn variant="secondary" loading={busy} disabled={manual.replace(/\D/g, '').length !== 6} onClick={() => checkIn(manual)}>Отметить</Btn>
      </div>
      {result && <div className={result.ok ? 'success success--compact' : 'error'}>{result.text}</div>}
    </section>
  )
}

export function BookingsTab({ venueId }: { venueId: string }) {
  const nav = useNav()
  const [items, setItems] = useState<Booking[] | null>(null)
  const [error, setError] = useState('')

  const load = () => {
    setError('')
    api.studioBookings(venueId).then((r) => setItems(r.bookings)).catch((e) => setError(e.message))
  }
  useEffect(load, [venueId])

  async function remind(id: string) {
    try {
      await api.studioRemind(id)
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
    <>
      <CheckInBox onDone={load} />
      {error && <ErrorBlock text={error} onRetry={load} />}
      {!error && !items && <Loader />}
      {items && items.length === 0 && <Empty title="На неделю записей пока нет" text="Как только клиент запишется, придёт уведомление в бот." />}
      {groups.map((g) => (
        <section key={g.key}>
          <h2 className="h2 section">{g.title}</h2>
          <div className="list">
            {g.items.map((b) => (
              <div key={b.id} className="list__row">
                <b className="list__time">{time(b.slot.start_at)}</b>
                <button className="list__main list__link" onClick={() => nav.push({ name: 'studioBooking', id: b.id })}>
                  <div className="list__title">{b.user_name || 'Гость'}</div>
                  <div className="muted small">{b.slot.title}</div>
                </button>
                {b.status === 'confirmed' ? (
                  <button className="icon-round" aria-label="Напомнить" onClick={() => remind(b.id)}><Bell size={16} /></button>
                ) : (
                  <span className={`status status--${b.status}`}>{status[b.status]}</span>
                )}
                <ChevronRight className="muted" size={18} />
              </div>
            ))}
          </div>
        </section>
      ))}
    </>
  )
}
