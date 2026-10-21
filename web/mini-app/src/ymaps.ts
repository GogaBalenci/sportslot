// Загрузка JavaScript API Яндекс Карт: одна копия скрипта на всё приложение.
// Ключ карты нужен для карты и геокодера, ключ Геосаджеста — для подсказок адреса.

const MAP_KEY = import.meta.env.VITE_YANDEX_MAPS_API_KEY ?? ''
const SUGGEST_KEY = import.meta.env.VITE_YANDEX_SUGGEST_API_KEY ?? ''

// Ростов-на-Дону: подсказки и поиск адреса ограничены городом.
export const ROSTOV_BOUNDS: [[number, number], [number, number]] = [
  [47.15, 39.45],
  [47.37, 39.87],
]

export const hasMapKey = () => MAP_KEY !== ''
export const hasSuggestKey = () => MAP_KEY !== '' && SUGGEST_KEY !== ''

let loader: Promise<any> | null = null

export function loadYmaps(): Promise<any> {
  if (!MAP_KEY) return Promise.reject(new Error('yandex maps key is not set'))
  if (window.ymaps) return new Promise((resolve) => window.ymaps.ready(() => resolve(window.ymaps)))
  loader ??= new Promise((resolve, reject) => {
    const params = new URLSearchParams({ apikey: MAP_KEY, lang: 'ru_RU' })
    if (SUGGEST_KEY) params.set('suggest_apikey', SUGGEST_KEY)
    const script = document.createElement('script')
    script.src = `https://api-maps.yandex.ru/2.1/?${params}`
    script.onload = () => window.ymaps.ready(() => resolve(window.ymaps))
    script.onerror = () => {
      loader = null
      reject(new Error('yandex maps are unavailable'))
    }
    document.head.appendChild(script)
  })
  return loader
}

export type AddressSuggestion = { title: string; value: string }

export async function suggestAddress(text: string): Promise<AddressSuggestion[]> {
  const ymaps = await loadYmaps()
  const items: { displayName: string; value: string }[] = await ymaps.suggest(`Ростов-на-Дону, ${text}`, {
    boundedBy: ROSTOV_BOUNDS,
    results: 5,
  })
  return items.map((i) => ({ title: i.displayName.replace(/,?\s*Ростов-на-Дону,?\s*Россия$/, ''), value: i.value }))
}

// geocodeAddress возвращает координаты адреса в Ростове или null.
export async function geocodeAddress(text: string): Promise<{ lat: number; lon: number } | null> {
  const ymaps = await loadYmaps()
  const query = /ростов/i.test(text) ? text : `Ростов-на-Дону, ${text}`
  const res = await ymaps.geocode(query, { results: 1, boundedBy: ROSTOV_BOUNDS, strictBounds: true })
  const first = res.geoObjects.get(0)
  if (!first) return null
  const [lat, lon] = first.geometry.getCoordinates()
  return { lat, lon }
}
