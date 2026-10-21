import { useEffect, useState } from 'react'
import { Building2, LogOut } from 'lucide-react'
import { ApiError, api, hasPartnerToken, setPartnerToken, type StudioSummary } from '../api'
import { Btn, Chips, ErrorBlock, Loader } from '../components'
import { useNav } from '../nav'
import { BookingsTab } from './BookingsTab'
import { HelpTab } from './HelpTab'
import { MessagesTab } from './MessagesTab'
import { ProfileTab } from './ProfileTab'
import { ScheduleTab } from './ScheduleTab'
import { StatsTab } from './StatsTab'

type Tab = 'bookings' | 'schedule' | 'messages' | 'stats' | 'profile' | 'help'

function DemoLogin({ onDone }: { onDone: () => void }) {
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function submit() {
    setBusy(true)
    setError('')
    try {
      const { token } = await api.partnerLogin(code.trim())
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
        <p className="muted">
          Администратор студии входит по ссылке-приглашению от СпортСлота — паролей нет, кабинет привязывается к аккаунту MAX.
        </p>
      </header>
      <section className="info-card">
        <b>Демо-доступ</b>
        <p className="muted">Для проверки работы кабинета на демо-студиях введите код доступа.</p>
        <input className="input" placeholder="Код доступа" value={code} onChange={(e) => setCode(e.target.value)} />
        {error && <div className="error">{error}</div>}
        <Btn size="lg" block loading={busy} disabled={!code.trim()} onClick={submit}>Войти</Btn>
      </section>
    </div>
  )
}

export function StudioScreen({ venueId, tab }: { venueId?: string; tab?: string }) {
  const nav = useNav()
  const [data, setData] = useState<{ studios: StudioSummary[]; platform: boolean; demo: boolean } | null>(null)
  const [error, setError] = useState('')
  const [current, setCurrent] = useState(venueId ?? sessionStorage.getItem('studio_venue') ?? '')
  const [active, setActive] = useState<Tab>((tab as Tab) ?? 'bookings')

  const load = () => {
    setError('')
    api
      .studioMe()
      .then(setData)
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401 && hasPartnerToken()) {
          setPartnerToken('')
          load()
          return
        }
        if (e instanceof ApiError && e.status === 401) setData({ studios: [], platform: false, demo: false })
        else setError(e.message)
      })
  }
  useEffect(load, [])

  useEffect(() => {
    if (!data || data.studios.length === 0) return
    if (!data.studios.some((s) => s.venue.id === current)) setCurrent(data.studios[0].venue.id)
  }, [data, current])

  useEffect(() => {
    if (current) sessionStorage.setItem('studio_venue', current)
  }, [current])

  if (error) return <div className="screen"><ErrorBlock text={error} onRetry={load} /></div>
  if (!data) return <div className="screen"><Loader /></div>
  if (data.studios.length === 0 && !data.platform) return <DemoLogin onDone={load} />

  const summary = data.studios.find((s) => s.venue.id === current)
  const unread = data.studios.reduce((n, s) => n + (s.venue.id === current ? s.unread_messages : 0), 0)
  const tabs: { id: Tab; title: string }[] = [
    { id: 'bookings', title: 'Записи' },
    { id: 'schedule', title: 'Расписание' },
    { id: 'messages', title: unread ? `Сообщения · ${unread}` : 'Сообщения' },
    { id: 'stats', title: 'Статистика' },
    { id: 'profile', title: 'Профиль' },
    { id: 'help', title: 'Как работать' },
  ]

  return (
    <div className="screen">
      <header className="hero">
        <div className="studio-head">
          <h1 className="h1">Кабинет студии</h1>
          {data.platform && (
            <Btn variant="secondary" icon={<Building2 size={16} />} onClick={() => nav.push({ name: 'platform' })}>Платформа</Btn>
          )}
        </div>
        {data.demo && <span className="badge badge--orange">Демо-доступ · только демо-студии</span>}
        {data.studios.length > 1 ? (
          <select className="select" value={current} onChange={(e) => setCurrent(e.target.value)}>
            {data.studios.map((s) => (
              <option key={s.venue.id} value={s.venue.id}>
                {s.venue.name}{s.unread_messages ? ` · ${s.unread_messages} нов.` : ''}
              </option>
            ))}
          </select>
        ) : (
          summary && <div className="studio-name">{summary.venue.name}</div>
        )}
        {summary && (
          <div className="muted small">
            Сегодня записей: {summary.today_bookings} · на неделю: {summary.upcoming_bookings}
          </div>
        )}
      </header>

      {data.studios.length === 0 ? (
        <div className="info-card">
          <p>Студий пока нет. Подключите первую в разделе «Платформа».</p>
        </div>
      ) : (
        <>
          <Chips items={tabs} value={active} onChange={setActive} tone="navy" />
          {current && active === 'bookings' && <BookingsTab venueId={current} />}
          {current && active === 'schedule' && <ScheduleTab venueId={current} />}
          {current && active === 'messages' && <MessagesTab venueId={current} onRead={load} />}
          {current && active === 'stats' && <StatsTab venueId={current} />}
          {current && active === 'profile' && summary && <ProfileTab venue={summary.venue} readOnly={data.demo} onSaved={load} />}
          {active === 'help' && <HelpTab />}
        </>
      )}

      {data.demo && (
        <Btn variant="ghost" block icon={<LogOut size={18} />} onClick={() => { setPartnerToken(''); setData(null); load() }}>
          Выйти из демо-доступа
        </Btn>
      )}
    </div>
  )
}
