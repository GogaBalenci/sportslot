export interface VenueSlot {
  venue_id: string
  name: string
  sport_type: string
  level: string
  address: string
  source_ref: string
  slot: {
    slot_id: string
    start_at: string
    end_at: string
    quota_available: number
  }
}

interface ApiErrorBody { error?: string; code?: string }

const apiBaseURL = import.meta.env.VITE_API_BASE_URL || ''

async function request<T>(path: string, maxUserID: string, init: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseURL}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-MAX-User-ID': maxUserID,
      ...init.headers,
    },
  })

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ApiErrorBody
    throw new Error(body.error || `Ошибка API (${response.status})`)
  }
  return response.json() as Promise<T>
}

export function findSlots(maxUserID: string, sportType: string): Promise<{ results: VenueSlot[] }> {
  return request('/api/v1/search', maxUserID, {
    method: 'POST',
    body: JSON.stringify({ sport_type: sportType, lat: 55.751244, lon: 37.618423, radius_km: 15 }),
  })
}

export function createBooking(maxUserID: string, slotID: string): Promise<{ id: string }> {
  return request('/api/v1/bookings', maxUserID, {
    method: 'POST',
    body: JSON.stringify({ slot_id: slotID, source_channel: 'miniapp' }),
  })
}
