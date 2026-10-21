import { createContext, useContext } from 'react'
import type { Catalog, TravelMode } from './api'

export type Route =
  | { name: 'search'; sport?: string }
  | { name: 'quiz' }
  | { name: 'my' }
  | { name: 'partner' }
  | { name: 'venue'; id: string; reschedule?: string }
  | { name: 'ticket'; id: string; fresh?: boolean }

// Origin — откуда пользователь добирается: подставляется в ссылки маршрута,
// чтобы Яндекс Карты не спрашивали точку старта.
export type Origin = { lat: number; lon: number; mode: TravelMode }

export type Nav = {
  catalog: Catalog | null
  origin: Origin | null
  setOrigin: (o: Origin | null) => void
  push: (r: Route) => void
  tab: (r: Route) => void
  back: () => void
  toast: (text: string) => void
}

export const NavContext = createContext<Nav | null>(null)

export function useNav(): Nav {
  const nav = useContext(NavContext)
  if (!nav) throw new Error('NavContext is missing')
  return nav
}

export function routeFromStart(param: string): Route {
  const [kind, ...rest] = param.split('_')
  const value = rest.join('_')
  switch (kind) {
    case 'sport':
      return { name: 'search', sport: value }
    case 'venue':
      return value ? { name: 'venue', id: value } : { name: 'search' }
    case 'ticket':
      return value ? { name: 'ticket', id: value } : { name: 'my' }
    case 'quiz':
      return { name: 'quiz' }
    case 'my':
      return { name: 'my' }
    case 'partner':
      return { name: 'partner' }
  }
  return { name: 'search' }
}
