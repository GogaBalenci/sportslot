import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CalendarCheck, ChevronLeft, Search, Sparkles } from 'lucide-react'
import { api, type Catalog } from './api'
import { insideMax, setBackButton, startParam } from './max'
import { NavContext, routeFromStart, type Nav, type Origin, type Route } from './nav'
import { MyScreen } from './screens/MyScreen'
import { ChatScreen } from './screens/ChatScreen'
import { FriendScreen } from './screens/FriendScreen'
import { GeoScreen } from './screens/GeoScreen'
import { PaymentScreen } from './screens/PaymentScreen'
import { QuizScreen } from './screens/QuizScreen'
import { SearchScreen } from './screens/SearchScreen'
import { TicketScreen } from './screens/TicketScreen'
import { VenueScreen } from './screens/VenueScreen'
import { InviteScreen } from './studio/InviteScreen'
import { PlatformScreen } from './studio/PlatformScreen'
import { StudioBookingScreen } from './studio/StudioBookingScreen'
import { StudioScreen } from './studio/StudioScreen'
import { StudioMessageScreen, StudioThreadScreen } from './studio/StudioThreadScreen'

const tabs: { route: Route; title: string; icon: typeof Search }[] = [
  { route: { name: 'search' }, title: 'Поиск', icon: Search },
  { route: { name: 'quiz' }, title: 'Подбор', icon: Sparkles },
  { route: { name: 'my' }, title: 'Мои занятия', icon: CalendarCheck },
]

function initialStack(): Route[] {
  const first = routeFromStart(startParam())
  if (first.name === 'venue') return [{ name: 'search' }, first]
  if (first.name === 'ticket') return [{ name: 'my' }, first]
  if (first.name === 'studioBooking') return [{ name: 'studio' }, first]
  if (first.name === 'payment' || first.name === 'friend') return [{ name: 'my' }, first]
  return [first]
}

export default function App() {
  const [stack, setStack] = useState<Route[]>(initialStack)
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [toast, setToast] = useState('')
  const toastTimer = useRef<number>()
  const [origin, setOrigin] = useState<Origin | null>(() => {
    try {
      return JSON.parse(sessionStorage.getItem('origin') ?? 'null')
    } catch {
      return null
    }
  })
  const saveOrigin = useCallback((o: Origin | null) => {
    setOrigin(o)
    try {
      sessionStorage.setItem('origin', JSON.stringify(o))
    } catch {
      /* хранилище недоступно — точка живёт до закрытия экрана */
    }
  }, [])

  useEffect(() => {
    api.catalog().then(setCatalog).catch(() => setToast('Сервис временно недоступен'))
  }, [])

  const back = useCallback(() => setStack((s) => (s.length > 1 ? s.slice(0, -1) : s)), [])
  const nav: Nav = useMemo(
    () => ({
      catalog,
      origin,
      setOrigin: saveOrigin,
      push: (r) => setStack((s) => [...s, r]),
      tab: (r) => setStack([r]),
      back,
      toast: (text) => {
        setToast(text)
        window.clearTimeout(toastTimer.current)
        toastTimer.current = window.setTimeout(() => setToast(''), 3500)
      },
    }),
    [catalog, back, origin, saveOrigin],
  )

  useEffect(() => {
    setBackButton(stack.length > 1 ? back : null)
    window.scrollTo(0, 0)
  }, [stack, back])

  const route = stack[stack.length - 1]
  const root = stack[0].name
  // Кабинет студии и переписка живут без нижней панели клиента.
  const studioArea = ['studio', 'studioBooking', 'studioThread', 'studioMessage', 'platform', 'invite'].includes(root)
  const hideTabbar = studioArea || route.name === 'chat' || route.name === 'studioThread'

  let screen
  switch (route.name) {
    case 'search':
      screen = (
        <SearchScreen
          key={`${route.sport ?? 'all'}|${route.point ? `${route.point.lat},${route.point.lon}` : ''}`}
          initialSport={route.sport}
          initialWhen={route.when}
          initialPoint={route.point}
        />
      )
      break
    case 'quiz':
      screen = <QuizScreen />
      break
    case 'my':
      screen = <MyScreen />
      break
    case 'studio':
      screen = <StudioScreen key={route.venueId ?? ''} venueId={route.venueId} tab={route.tab} />
      break
    case 'studioBooking':
      screen = <StudioBookingScreen key={route.id} id={route.id} />
      break
    case 'studioThread':
      screen = <StudioThreadScreen key={route.venueId + route.userId} venueId={route.venueId} userId={route.userId} />
      break
    case 'studioMessage':
      screen = <StudioMessageScreen id={route.id} />
      break
    case 'platform':
      screen = <PlatformScreen />
      break
    case 'invite':
      screen = <InviteScreen token={route.token} />
      break
    case 'geo':
      screen = <GeoScreen />
      break
    case 'friend':
      screen = <FriendScreen token={route.token} />
      break
    case 'payment':
      screen = <PaymentScreen key={route.id} id={route.id} />
      break
    case 'chat':
      screen = <ChatScreen key={route.venueId} venueId={route.venueId} venueName={route.venueName} />
      break
    case 'venue':
      screen = <VenueScreen key={route.id + (route.reschedule ?? '')} id={route.id} reschedule={route.reschedule} />
      break
    case 'ticket':
      screen = <TicketScreen key={route.id} id={route.id} fresh={route.fresh} />
      break
  }

  return (
    <NavContext.Provider value={nav}>
      <main className="app">
        {stack.length > 1 && !insideMax() && (
          <button className="back" onClick={back}>
            <ChevronLeft size={20} /> Назад
          </button>
        )}
        {screen}
      </main>
      {toast && <div className="toast">{toast}</div>}
      {!hideTabbar && (
        <nav className="tabbar">
          {tabs.map((t) => (
            <button key={t.title} className={root === t.route.name ? 'active' : ''} onClick={() => nav.tab(t.route)}>
              <t.icon size={22} strokeWidth={root === t.route.name ? 2.4 : 2} />
              <span>{t.title}</span>
            </button>
          ))}
        </nav>
      )}
    </NavContext.Provider>
  )
}
