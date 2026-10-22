import { createContext, useContext } from 'react'
import type { Catalog, TravelMode } from './api'

export type Point = { lat: number; lon: number; label: string }

export type Route =
  | { name: 'search'; sport?: string; when?: string; point?: Point }
  | { name: 'quiz' }
  | { name: 'my' }
  | { name: 'studio'; venueId?: string; tab?: string }
  | { name: 'studioBooking'; id: string }
  | { name: 'studioThread'; venueId: string; userId: string }
  | { name: 'studioMessage'; id: string }
  | { name: 'platform' }
  | { name: 'invite'; token: string }
  | { name: 'geo' }
  | { name: 'chat'; venueId: string; venueName: string }
  | { name: 'friend'; token: string }
  | { name: 'payment'; id: string }
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
      return { name: 'studio' }
    case 'studio':
      if (/^[0-9a-f]{32}$/.test(value)) return { name: 'invite', token: value }
      return value ? { name: 'studio', tab: value } : { name: 'studio' }
    case 'friend':
      return /^[0-9a-f]{32}$/.test(value) ? { name: 'friend', token: value } : { name: 'my' }
    case 'pay':
      return value ? { name: 'payment', id: value } : { name: 'my' }
    case 'sb':
      return value ? { name: 'studioBooking', id: value } : { name: 'studio' }
    case 'sm':
      return value ? { name: 'studioMessage', id: value } : { name: 'studio' }
    case 'platform':
      return { name: 'platform' }
    case 'geo':
      return { name: 'geo' }
  }
  return { name: 'search' }
}
