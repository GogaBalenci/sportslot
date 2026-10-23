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
  source: 'osm' | 'demo_partner' | 'partner'
  source_url?: string
  booking_mode: 'instant' | 'external'
  phone?: string
  website?: string
  opening_hours?: string
  description?: string
  what_to_bring?: string
  trial_price?: number
  verified_at?: string
  logo_url?: string
}

export type SlotInfo = {
  slot_id: string
  title: string
  sport_type: string
  start_at: string
  end_at: string
  quota_total: number
  quota_available: number
  repeats?: boolean
}

export type TravelMode = 'walking' | 'transit' | 'driving'

export type Travel = { mode: TravelMode; minutes: number; estimated: boolean }

export type Message = { id: number; venue_id: string; user_id: string; author: 'client' | 'studio'; text: string; created_at: string }

export type Template = {
  id: string
  venue_id: string
  weekday: number
  start_minute: number
  duration_min: number
  title: string
  sport_type: string
  level: string
  quota: number
}

export type StudioSlot = {
  id: string
  venue_id: string
  title: string
  sport_type: string
  start_at: string
  end_at: string
  quota_total: number
  quota_booked: number
  template_id?: string
  closed?: boolean
  cancelled?: boolean
  booked: number
}

export type StudioSummary = { venue: Venue; today_bookings: number; upcoming_bookings: number; unread_messages: number }

export type ClientCard = {
  booking: Booking
  max_user_id: string
  history: Booking[]
  visits: number
  no_shows: number
  messages: Message[]
}

export type Thread = { user_id: string; user_name: string; last_text: string; last_at: string; last_by: string; unread: number }

export type StudioStats = {
  response: { answered: number; median_minutes: number; within_hour: number; waiting: number }
  paid: { count: number; amount: number }
  since: string
  bookings: number
  attended: number
  no_show: number
  cancelled_by_client: number
  upcoming: number
  clients: number
  returning_clients: number
  rating_avg: number
  ratings: number
}

export type VenueProfile = { phone: string; website: string; description: string; what_to_bring: string; trial_price: number | null }

export type PlatformStudio = {
  venue: Venue
  admins: { max_user_id: string; name: string; created_at: string }[]
  invites: { token: string; expires_at: string; used_by?: string }[]
}

export type LastLocation = { lat: number; lon: number; address?: string; at: string }

export type VenueCard = {
  venue: Venue
  distance_km: number
  travel?: Travel
  beyond_limit?: boolean
  slots: SlotInfo[]
  available_slots: number
  rating?: ReviewSummary
}

export type Review = { rating: number; text?: string; author: string; date: string }
export type ReviewSummary = { average: number; count: number; items?: Review[] }

export type Pass = { id: string; venue_id: string; title: string; visits: number; valid_days: number; price: number; active: boolean }

export type UserPass = {
  id: string
  pass_id?: string
  venue_id: string
  title: string
  visits_total: number
  visits_left: number
  valid_days: number
  price: number
  status: 'requested' | 'active' | 'used' | 'expired' | 'cancelled'
  paid_by?: 'online' | 'studio'
  valid_until?: string
  created_at: string
  venue_name?: string
  user_name?: string
}

export type Payment = {
  id: string
  venue_id: string
  kind: 'trial' | 'pass'
  booking_id?: string
  user_pass_id?: string
  amount: number
  description: string
  status: 'pending' | 'succeeded' | 'canceled'
  confirmation_url?: string
  created_at: string
  paid_at?: string
  user_name?: string
}

export type FriendPlace = { booking: Booking; link?: string; claimed: boolean; friend_name?: string }

export type SeriesResult = { bookings: Booking[]; skipped: string[] }

export type BookResult = Booking & { series?: SeriesResult; friend?: FriendPlace; friend_error?: string }

export type StudioSettings = {
  venue_id: string
  show_reviews: boolean
  daily_digest: boolean
  payment_provider: '' | 'yookassa' | 'tbank'
  payment_account: string
  payment_secret_hint: string
  payments_enabled: boolean
  trial_online: boolean
  secret_saved: boolean
  encryption_ready: boolean
  yookassa_webhook: string
  tbank_webhook: string
}

export type SettingsInput = Partial<{
  show_reviews: boolean
  daily_digest: boolean
  payment_provider: string
  payment_account: string
  payment_secret: string
  payments_enabled: boolean
  trial_online: boolean
}>

export type StudioCommerce = { passes: Pass[]; user_passes: UserPass[]; payments: Payment[]; month: { count: number; amount: number } }

export type VenueDetails = Venue & {
  venue_id: string
  all_slots: SlotInfo[]
  reviews?: ReviewSummary
  passes?: Pass[]
  online_payment?: { trial: boolean; passes: boolean }
}

export type Booking = {
  id: string
  user_id: string
  slot_id: string
  cancel_reason?: 'user' | 'studio'
  status: 'confirmed' | 'cancelled' | 'attended' | 'no_show'
  checkin_code: string
  rating?: number
  review?: string
  is_guest?: boolean
  host_booking_id?: string
  user_pass_id?: string
  paid_at?: string
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

// assetUrl — адрес файла с сервера API (логотипы), с учётом VITE_API_BASE_URL.
export const assetUrl = (path?: string) => (path ? `${BASE}${path}` : '')

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
    throw new ApiError(res.status, data.code ?? 'error', data.error ?? 'Что-то пошло не так, попробуй ещё раз')
  }
  return data as T
}

const user = <T>(method: string, path: string, body?: unknown) => request<T>(method, path, body, userHeaders())

// Кабинет студии: администратор входит по подписи MAX, проверяющие — по демо-коду.
const studioHeaders = (): Record<string, string> => (partnerToken ? { Authorization: `Partner ${partnerToken}` } : userHeaders())
const studio = <T>(method: string, path: string, body?: unknown) => request<T>(method, path, body, studioHeaders())

export type VenueQuery = {
  sport?: string
  when?: string
  district?: string
  lat?: number
  lon?: number
  mode?: TravelMode
  max_minutes?: number
}

export const api = {
  catalog: () => request<Catalog>('GET', '/catalog'),
  venues: (q: VenueQuery) => {
    const params = new URLSearchParams()
    Object.entries(q).forEach(([k, v]) => v !== undefined && v !== '' && params.set(k, String(v)))
    return request<{ venues: VenueCard[] }>('GET', `/venues?${params}`)
  },
  venue: (id: string) => request<VenueDetails>('GET', `/venues/${id}`),
  recommend: (answers: Record<string, string>) => request<{ results: QuizResult[] }>('POST', '/quiz/recommend', { answers }),

  me: () =>
    user<{ user: { id: string; name: string }; bookings: Booking[]; last_location?: LastLocation }>('GET', '/me'),
  geocode: (q: string) => request<{ lat: number; lon: number; address: string }>('GET', `/geocode?q=${encodeURIComponent(q)}`),
  book: (slotId: string, opts: { weeks?: number; withFriend?: boolean } = {}) =>
    user<BookResult>('POST', '/bookings', {
      slot_id: slotId,
      source_channel: 'miniapp',
      weeks: opts.weeks && opts.weeks > 1 ? opts.weeks : undefined,
      with_friend: opts.withFriend || undefined,
    }),
  booking: (id: string) => user<Booking>('GET', `/bookings/${id}`),
  cancel: (id: string) => user<void>('POST', `/bookings/${id}/cancel`),
  reschedule: (id: string, slotId: string) => user<Booking>('PATCH', `/bookings/${id}/reschedule`, { new_slot_id: slotId }),
  waitlist: (slotId: string) => user<{ position: number }>('POST', '/waitlist', { slot_id: slotId }),
  feedback: (id: string, rating: number) => user<void>('POST', `/bookings/${id}/feedback`, { rating }),
  review: (id: string, text: string) => user<void>('POST', `/bookings/${id}/review`, { text }),
  friend: (id: string) => user<{ friend: FriendPlace | null }>('GET', `/bookings/${id}/friend`),
  addFriend: (id: string) => user<FriendPlace>('POST', `/bookings/${id}/friend`),
  claimFriend: (token: string) => user<Booking>('POST', '/friends/claim', { token }),
  payBooking: (id: string) => user<Payment>('POST', `/bookings/${id}/pay`),
  payment: (id: string) => user<Payment>('GET', `/payments/${id}`),
  myPasses: () => user<{ passes: UserPass[] }>('GET', '/me/passes'),
  buyPass: (passId: string) =>
    user<{ user_pass: UserPass; payment?: Payment; payment_error?: boolean; created: boolean }>('POST', `/passes/${passId}/buy`),
  payPass: (id: string) => user<Payment>('POST', `/me/passes/${id}/pay`),
  cancelPassRequest: (id: string) => user<void>('POST', `/me/passes/${id}/cancel`),

  partnerLogin: (code: string) => request<{ token: string }>('POST', '/partner/login', { code }),

  saveLocation: (lat: number, lon: number, address = '') =>
    user<{ sport?: string; when?: string }>('POST', '/me/location', { lat, lon, address }),
  clientMessages: (venueId: string) => user<{ messages: Message[] }>('GET', `/messages?venue_id=${venueId}`),
  sendClientMessage: (venueId: string, text: string) => user<Message>('POST', '/messages', { venue_id: venueId, text }),
  acceptInvite: (token: string) => user<Venue>('POST', '/studio/invites/accept', { token }),

  studioMe: () => studio<{ studios: StudioSummary[]; platform: boolean; demo: boolean }>('GET', '/studio/me'),
  studioBookings: (venueId: string) => studio<{ bookings: Booking[] }>('GET', `/studio/${venueId}/bookings`),
  studioBooking: (id: string) => studio<ClientCard>('GET', `/studio/bookings/${id}`),
  studioRemind: (id: string) => studio<void>('POST', `/studio/bookings/${id}/remind`),
  studioCheckIn: (code: string) => studio<Booking & { training?: boolean; message?: string }>('POST', '/studio/checkin', { code }),
  templates: (venueId: string) => studio<{ templates: Template[] }>('GET', `/studio/${venueId}/templates`),
  saveTemplate: (venueId: string, t: Partial<Template>) =>
    t.id
      ? studio<{ template: Template; kept_with_bookings: number }>('PUT', `/studio/${venueId}/templates/${t.id}`, t)
      : studio<{ template: Template; kept_with_bookings: number }>('POST', `/studio/${venueId}/templates`, t),
  deleteTemplate: (venueId: string, id: string) =>
    studio<{ kept_with_bookings: number }>('DELETE', `/studio/${venueId}/templates/${id}`),
  studioSlots: (venueId: string) => studio<{ slots: StudioSlot[] }>('GET', `/studio/${venueId}/slots`),
  closeSlot: (venueId: string, slotId: string, closed: boolean) =>
    studio<void>('POST', `/studio/${venueId}/slots/${slotId}/close`, { closed }),
  cancelSlot: (venueId: string, slotId: string) =>
    studio<{ notified: number }>('POST', `/studio/${venueId}/slots/${slotId}/cancel`),
  studioStats: (venueId: string) => studio<StudioStats>('GET', `/studio/${venueId}/stats`),
  studioProfile: (venueId: string, p: VenueProfile) => studio<Venue>('PATCH', `/studio/${venueId}/profile`, p),
  threads: (venueId: string) => studio<{ threads: Thread[] }>('GET', `/studio/${venueId}/threads`),
  thread: (venueId: string, userId: string) =>
    studio<{ user: { id: string; name: string }; messages: Message[] }>('GET', `/studio/${venueId}/threads/${userId}`),
  reply: (venueId: string, userId: string, text: string) =>
    studio<Message>('POST', `/studio/${venueId}/threads/${userId}`, { text }),
  messageRef: (id: string) => studio<{ venue_id: string; user_id: string }>('GET', `/studio/messages/${id}`),
  studioSettings: (venueId: string) => studio<StudioSettings>('GET', `/studio/${venueId}/settings`),
  saveStudioSettings: (venueId: string, input: SettingsInput) => studio<StudioSettings>('PATCH', `/studio/${venueId}/settings`, input),
  checkPayments: (venueId: string) => studio<{ ok: boolean }>('POST', `/studio/${venueId}/settings/check`),
  studioCommerce: (venueId: string) => studio<StudioCommerce>('GET', `/studio/${venueId}/commerce`),
  savePass: (venueId: string, p: Partial<Pass>) =>
    p.id ? studio<Pass>('PUT', `/studio/${venueId}/passes/${p.id}`, p) : studio<Pass>('POST', `/studio/${venueId}/passes`, p),
  confirmPass: (venueId: string, id: string) => studio<UserPass>('POST', `/studio/${venueId}/user-passes/${id}/confirm`),
  declinePass: (venueId: string, id: string) => studio<void>('POST', `/studio/${venueId}/user-passes/${id}/decline`),
  studioReviews: (venueId: string) => studio<ReviewSummary>('GET', `/studio/${venueId}/reviews`),
  uploadLogo: async (venueId: string, file: Blob) => {
    const res = await fetch(`${BASE}/api/v1/studio/${venueId}/logo`, {
      method: 'PUT',
      headers: { 'Content-Type': file.type || 'application/octet-stream', ...studioHeaders() },
      body: file,
    })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new ApiError(res.status, data.code ?? 'error', data.error ?? 'Не получилось загрузить логотип')
    return data as Venue
  },
  deleteLogo: (venueId: string) => studio<void>('DELETE', `/studio/${venueId}/logo`),

  platformStudios: () => studio<{ studios: PlatformStudio[] }>('GET', '/platform/studios'),
  platformCatalog: (q: string) => studio<{ venues: Venue[] }>('GET', `/platform/catalog?q=${encodeURIComponent(q)}`),
  platformConnect: (venueId: string) => studio<Venue>('POST', '/platform/studios', { venue_id: venueId }),
  platformCreate: (s: { name: string; address: string; sports: string[]; phone: string }) =>
    studio<Venue>('POST', '/platform/studios', s),
  platformInvite: (venueId: string) =>
    studio<{ token: string; link: string; expires_at: string }>('POST', `/platform/studios/${venueId}/invites`),
  platformRemoveAdmin: (venueId: string, maxUserId: string) =>
    studio<void>('DELETE', `/platform/studios/${venueId}/admins/${maxUserId}`),
}
