import { useEffect, useMemo, useState } from 'react'
import { Ban, Lock, Pencil, Plus, Trash2, Unlock } from 'lucide-react'
import { api, type StudioSlot, type Template } from '../api'
import { Btn, Empty, ErrorBlock, Loader, Sheet } from '../components'
import { dayKey, dayTitle, time } from '../format'
import { useNav } from '../nav'

// Неделя с понедельника; weekday в API — как в Go: 0 — воскресенье.
const WEEK = [1, 2, 3, 4, 5, 6, 0]
const DAY_NAMES = ['Воскресенье', 'Понедельник', 'Вторник', 'Среда', 'Четверг', 'Пятница', 'Суббота']

const hhmm = (minutes: number) => `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`

function TemplateForm(props: { venueId: string; initial?: Template; onClose: () => void; onSaved: (kept: number) => void }) {
  const nav = useNav()
  const t = props.initial
  const sports = nav.catalog?.sports.filter((s) => s.bookable) ?? []
  const [weekday, setWeekday] = useState(t?.weekday ?? 1)
  const [start, setStart] = useState(hhmm(t?.start_minute ?? 19 * 60))
  const [duration, setDuration] = useState(t?.duration_min ?? 60)
  const [title, setTitle] = useState(t?.title ?? '')
  const [sport, setSport] = useState(t?.sport_type ?? sports[0]?.id ?? 'fitness')
  const [quota, setQuota] = useState(t?.quota ?? 8)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function save() {
    const [h, m] = start.split(':').map(Number)
    setBusy(true)
    setError('')
    try {
      const r = await api.saveTemplate(props.venueId, {
        id: t?.id, weekday, start_minute: h * 60 + m, duration_min: duration, title: title.trim(), sport_type: sport, quota,
      })
      props.onSaved(r.kept_with_bookings)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не получилось сохранить')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Sheet onClose={props.onClose}>
      <h2 className="h2">{t ? 'Изменить занятие' : 'Новое занятие'}</h2>
      <label className="field">
        <span>Название</span>
        <input className="input" placeholder="Например, Бокс для новичков" value={title} maxLength={120} onChange={(e) => setTitle(e.target.value)} />
      </label>
      <label className="field">
        <span>Вид спорта</span>
        <select className="select" value={sport} onChange={(e) => setSport(e.target.value)}>
          {sports.map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}
        </select>
      </label>
      <div className="row">
        <label className="field">
          <span>День недели</span>
          <select className="select" value={weekday} onChange={(e) => setWeekday(Number(e.target.value))}>
            {WEEK.map((d) => <option key={d} value={d}>{DAY_NAMES[d]}</option>)}
          </select>
        </label>
        <label className="field field--narrow">
          <span>Начало</span>
          <input className="input" type="time" value={start} onChange={(e) => setStart(e.target.value)} />
        </label>
      </div>
      <div className="row">
        <label className="field">
          <span>Длительность</span>
          <select className="select" value={duration} onChange={(e) => setDuration(Number(e.target.value))}>
            {[30, 45, 50, 60, 75, 90, 120].map((d) => <option key={d} value={d}>{d} мин</option>)}
          </select>
        </label>
        <label className="field field--narrow">
          <span>Мест для новичков</span>
          <input className="input" type="number" min={1} max={100} value={quota} onChange={(e) => setQuota(Number(e.target.value))} />
        </label>
      </div>
      <p className="muted small">Занятие будет появляться каждую неделю — на две недели вперёд. Если на занятие уже записались, его время не изменится: отмените его отдельно, клиенты получат уведомление.</p>
      {error && <div className="error">{error}</div>}
      <Btn size="lg" block loading={busy} disabled={!title.trim() || quota < 1} onClick={save}>Сохранить</Btn>
    </Sheet>
  )
}

export function ScheduleTab({ venueId }: { venueId: string }) {
  const nav = useNav()
  const [templates, setTemplates] = useState<Template[] | null>(null)
  const [slots, setSlots] = useState<StudioSlot[] | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Template | 'new' | null>(null)
  const [cancelling, setCancelling] = useState<StudioSlot | null>(null)

  const load = () => {
    setError('')
    Promise.all([api.templates(venueId), api.studioSlots(venueId)])
      .then(([t, s]) => {
        setTemplates(t.templates)
        setSlots(s.slots)
      })
      .catch((e) => setError(e.message))
  }
  useEffect(load, [venueId])

  const byDay = useMemo(() => {
    const m = new Map<number, Template[]>()
    templates?.forEach((t) => m.set(t.weekday, [...(m.get(t.weekday) ?? []), t]))
    return m
  }, [templates])

  function saved(kept: number) {
    setEditing(null)
    nav.toast(kept > 0 ? `Сохранено. ${kept} занятий с записями остались как были` : 'Расписание обновлено')
    load()
  }

  async function remove(t: Template) {
    try {
      const r = await api.deleteTemplate(venueId, t.id)
      nav.toast(r.kept_with_bookings > 0 ? `Удалено. ${r.kept_with_bookings} занятий с записями остались — отмените их при необходимости` : 'Занятие удалено из расписания')
      load()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось удалить')
    }
  }

  async function toggle(s: StudioSlot) {
    try {
      await api.closeSlot(venueId, s.id, !s.closed)
      nav.toast(s.closed ? 'Запись снова открыта' : 'Запись на занятие приостановлена')
      load()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось')
    }
  }

  async function cancel(s: StudioSlot) {
    try {
      const r = await api.cancelSlot(venueId, s.id)
      setCancelling(null)
      nav.toast(r.notified ? `Занятие отменено, ${r.notified} клиентам ушло уведомление` : 'Занятие отменено')
      load()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось отменить')
    }
  }

  if (error) return <ErrorBlock text={error} onRetry={load} />
  if (!templates || !slots) return <Loader />

  const days: { key: string; title: string; items: StudioSlot[] }[] = []
  slots.forEach((s) => {
    const key = dayKey(s.start_at)
    let g = days.find((x) => x.key === key)
    if (!g) days.push((g = { key, title: dayTitle(s.start_at), items: [] }))
    g.items.push(s)
  })

  return (
    <>
      <section className="info-card">
        <div className="studio-head">
          <h2 className="h2">Шаблон недели</h2>
          <Btn icon={<Plus size={18} />} onClick={() => setEditing('new')}>Занятие</Btn>
        </div>
        {templates.length === 0 && <p className="muted">Добавьте занятия, на которые студия готова брать новичков, и сколько мест отдаёт.</p>}
        {WEEK.filter((d) => byDay.has(d)).map((d) => (
          <div key={d} className="week-day">
            <div className="week-day__name">{DAY_NAMES[d]}</div>
            {byDay.get(d)!.map((t) => (
              <div key={t.id} className="tpl">
                <b>{hhmm(t.start_minute)}</b>
                <div className="tpl__main">
                  <div>{t.title}</div>
                  <div className="muted small">{t.duration_min} мин · {t.quota} мест</div>
                </div>
                <button className="icon-round" aria-label="Изменить" onClick={() => setEditing(t)}><Pencil size={15} /></button>
                <button className="icon-round" aria-label="Удалить" onClick={() => remove(t)}><Trash2 size={15} /></button>
              </div>
            ))}
          </div>
        ))}
      </section>

      <h2 className="h2 section">Ближайшие занятия</h2>
      {days.length === 0 && <Empty title="Занятий пока нет" text="Добавьте занятия в шаблон недели." />}
      {days.map((g) => (
        <section key={g.key}>
          <div className="day-title">{g.title}</div>
          <div className="list">
            {g.items.map((s) => (
              <div key={s.id} className={`list__row${s.cancelled ? ' list__row--muted' : ''}`}>
                <b className="list__time">{time(s.start_at)}</b>
                <div className="list__main">
                  <div className="list__title">{s.title}</div>
                  <div className="muted small">
                    {s.cancelled ? 'Отменено' : `Через СпортСлот: ${s.booked} · занято ${Math.min(s.quota_booked, s.quota_total)} из ${s.quota_total}`}
                    {s.closed && !s.cancelled ? ' · запись закрыта' : ''}
                  </div>
                </div>
                {!s.cancelled && (
                  <>
                    <button className="icon-round" aria-label={s.closed ? 'Открыть запись' : 'Закрыть запись'} onClick={() => toggle(s)}>
                      {s.closed ? <Unlock size={15} /> : <Lock size={15} />}
                    </button>
                    <button className="icon-round icon-round--danger" aria-label="Отменить занятие" onClick={() => setCancelling(s)}><Ban size={15} /></button>
                  </>
                )}
              </div>
            ))}
          </div>
        </section>
      ))}

      {editing && (
        <TemplateForm venueId={venueId} initial={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} onSaved={saved} />
      )}
      {cancelling && (
        <Sheet onClose={() => setCancelling(null)}>
          <h2 className="h2">Отменить занятие?</h2>
          <p>{cancelling.title}, {dayTitle(cancelling.start_at).toLowerCase()} в {time(cancelling.start_at)}</p>
          <div className="note note--warning">
            {cancelling.booked
              ? `Записи ${cancelling.booked} клиентов будут отменены. Каждый получит сообщение в чате: «Занятие отменено студией» с вашими контактами.`
              : 'Записей нет — занятие просто исчезнет из расписания.'}
          </div>
          <Btn size="lg" variant="danger" block onClick={() => cancel(cancelling)}>Отменить занятие</Btn>
          <Btn size="lg" variant="secondary" block onClick={() => setCancelling(null)}>Оставить</Btn>
        </Sheet>
      )}
    </>
  )
}
