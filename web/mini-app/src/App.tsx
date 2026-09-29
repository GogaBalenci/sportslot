import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, type Catalog } from './api'
import { setBackButton, startParam } from './max'
import { NavContext, routeFromStart, type Nav, type Route } from './nav'
import { MyScreen } from './screens/MyScreen'
import { PartnerScreen } from './screens/PartnerScreen'
import { QuizScreen } from './screens/QuizScreen'
import { SearchScreen } from './screens/SearchScreen'
import { TicketScreen } from './screens/TicketScreen'
import { VenueScreen } from './screens/VenueScreen'

const tabs: { route: Route; title: string }[] = [
  { route: { name: 'search' }, title: 'Поиск' },
  { route: { name: 'quiz' }, title: 'Подбор' },
  { route: { name: 'my' }, title: 'Мои занятия' },
]

function initialStack(): Route[] {
  const first = routeFromStart(startParam())
  if (first.name === 'venue') return [{ name: 'search' }, first]
  if (first.name === 'ticket') return [{ name: 'my' }, first]
  return [first]
}

export default function App() {
  const [stack, setStack] = useState<Route[]>(initialStack)
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [toast, setToast] = useState('')
  const toastTimer = useRef<number>()

  useEffect(() => {
    api.catalog().then(setCatalog).catch(() => setToast('Сервис временно недоступен'))
  }, [])

  const back = useCallback(() => setStack((s) => (s.length > 1 ? s.slice(0, -1) : s)), [])
  const nav: Nav = useMemo(
    () => ({
      catalog,
      push: (r) => setStack((s) => [...s, r]),
      tab: (r) => setStack([r]),
      back,
      toast: (text) => {
        setToast(text)
        window.clearTimeout(toastTimer.current)
        toastTimer.current = window.setTimeout(() => setToast(''), 3500)
      },
    }),
    [catalog, back],
  )

  useEffect(() => {
    setBackButton(stack.length > 1 ? back : null)
    window.scrollTo(0, 0)
  }, [stack, back])

  const route = stack[stack.length - 1]
  const root = stack[0].name

  let screen
  switch (route.name) {
    case 'search':
      screen = <SearchScreen key={route.sport ?? 'all'} initialSport={route.sport} />
      break
    case 'quiz':
      screen = <QuizScreen />
      break
    case 'my':
      screen = <MyScreen />
      break
    case 'partner':
      screen = <PartnerScreen />
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
        {stack.length > 1 && (
          <button className="back" onClick={back}>
            ← Назад
          </button>
        )}
        {screen}
      </main>
      {toast && <div className="toast">{toast}</div>}
      {route.name !== 'partner' && (
        <nav className="tabbar">
          {tabs.map((t) => (
            <button key={t.title} className={root === t.route.name ? 'active' : ''} onClick={() => nav.tab(t.route)}>
              {t.title}
            </button>
          ))}
        </nav>
      )}
    </NavContext.Provider>
  )
}
