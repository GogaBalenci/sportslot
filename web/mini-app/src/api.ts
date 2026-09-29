import { initData } from './max'

const BASE = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export type Venue = {
  id: string
  name: string
  sport_type: string
  sports: string[]
  address: string
  district: string
  lat: number
  lon: number
  source: 'osm' | 'demo_partner'
  source_url?: string
  booking_mode: 'instant' | 'external'
  phone?: string
  website?: string
  opening_hours?: string
  description?: string
  what_to_bring?: string
  trial_price?: number
  verified_at?: string
}

export type SlotInfo = {
  slot_id: string
  title: string
  sport_type: string
  start_at: string
  end_at: string
  quota_total: number
  quota_available: number
}

export type VenueCard = {
  venue: Venue
  distance_km: number
  slots: SlotInfo[]
  available_slots: number
}

export type VenueDetails = Venue & { venue_id: string; all_slots: SlotInfo[] }

export type Booking = {
  id: string
  slot_id: string
  status: 'confirmed' | 'cancelled' | 'attended' | 'no_show'
  checkin_code: string
  rating?: number
  user_name?: string
  slot: { id: string; title: string; sport_type: string; start_at: string; end_at: string }
  venue: Venue
}

export type Option = { id: string; title: string }
export type Question = { id: string; title: string; options: Option[] }

export type Catalog = {
  sports: { id: string; title: string; short: string; bookable: boolean }[]
  districts: { id: string; title: string; lat: number; lon: number }[]
  whens: Option[]
  quiz: Question[]
  data_sources: { catalog_date: string; demo_partners: boolean; partner_enabled: boolean }
}

export type QuizResult = { sport: string; title: string; reason: string }

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
  }
}

let partnerToken = sessionStorage.getItem('partner_token') ?? ''

export function setPartnerToken(token: string): void {
  partnerToken = token
  if (token) sessionStorage.setItem('partner_token', token)
  else sessionStorage.removeItem('partner_token')
}

export function hasPartnerToken(): boolean {
  return partnerToken !== ''
}

function userHeaders(): Record<string, string> {
  const data = initData()
  if (data) return { Authorization: `tma ${data}` }
  const devUser = import.meta.env.VITE_DEV_MAX_USER_ID
  return devUser ? { 'X-MAX-User-ID': devUser } : {}
}

async function request<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  const res = await fetch(`${BASE}/api/v1${path}`, {
    method,
    headers: { 'Content-Type': 'application/json', ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(res.status, data.code ?? 'error', data.error ?? 'Что-то пошло не так, попробуйте ещё раз')
  }
  return data as T
}

const user = <T>(method: string, path: string, body?: unknown) => request<T>(method, path, body, userHeaders())
const partner = <T>(method: string, path: string, body?: unknown) =>
  request<T>(method, path, body, { Authorization: `Partner ${partnerToken}` })

export type VenueQuery = { sport?: string; when?: string; district?: string; lat?: number; lon?: number }

export const api = {
  catalog: () => request<Catalog>('GET', '/catalog'),
  venues: (q: VenueQuery) => {
    const params = new URLSearchParams()
    Object.entries(q).forEach(([k, v]) => v !== undefined && v !== '' && params.set(k, String(v)))
    return request<{ venues: VenueCard[] }>('GET', `/venues?${params}`)
  },
  venue: (id: string) => request<VenueDetails>('GET', `/venues/${id}`),
  recommend: (answers: Record<string, string>) => request<{ results: QuizResult[] }>('POST', '/quiz/recommend', { answers }),

  me: () => user<{ user: { id: string; name: string }; bookings: Booking[] }>('GET', '/me'),
  book: (slotId: string) => user<Booking>('POST', '/bookings', { slot_id: slotId, source_channel: 'miniapp' }),
  booking: (id: string) => user<Booking>('GET', `/bookings/${id}`),
  cancel: (id: string) => user<void>('POST', `/bookings/${id}/cancel`),
  reschedule: (id: string, slotId: string) => user<Booking>('PATCH', `/bookings/${id}/reschedule`, { new_slot_id: slotId }),
  waitlist: (slotId: string) => user<{ position: number }>('POST', '/waitlist', { slot_id: slotId }),
  feedback: (id: string, rating: number) => user<void>('POST', `/bookings/${id}/feedback`, { rating }),

  partnerLogin: (code: string) => request<{ token: string }>('POST', '/partner/login', { code }),
  partnerBookings: () => partner<{ bookings: Booking[] }>('GET', '/partner/bookings'),
  partnerCheckIn: (code: string) => partner<Booking>('POST', '/partner/checkin', { code }),
  partnerRemind: (id: string) => partner<void>('POST', `/partner/bookings/${id}/remind`),
}
