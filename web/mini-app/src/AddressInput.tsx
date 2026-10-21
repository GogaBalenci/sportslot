import { useEffect, useRef, useState } from 'react'
import { MapPin, Search } from 'lucide-react'
import { ApiError, api } from './api'
import { Btn } from './components'
import { geocodeAddress, hasMapKey, hasSuggestKey, suggestAddress, type AddressSuggestion } from './ymaps'

type Props = { onPick: (point: { lat: number; lon: number; label: string }) => void; onError: (text: string) => void }

// Ввод адреса для расчёта времени в пути. Подсказки — Геосаджест Яндекса
// (если задан ключ), координаты — геокодер 2ГИС на сервере, Яндекс — запасной.
// Подсказки запрашиваются после паузы в наборе и от трёх символов.
export function AddressInput({ onPick, onError }: Props) {
  const [text, setText] = useState('')
  const [items, setItems] = useState<AddressSuggestion[]>([])
  const [busy, setBusy] = useState(false)
  const timer = useRef<number>()
  const picked = useRef('')

  useEffect(() => {
    window.clearTimeout(timer.current)
    if (!hasSuggestKey() || text.trim().length < 3 || text === picked.current) {
      setItems([])
      return
    }
    timer.current = window.setTimeout(() => {
      suggestAddress(text.trim())
        .then(setItems)
        .catch(() => setItems([]))
    }, 350)
    return () => window.clearTimeout(timer.current)
  }, [text])

  async function locate(query: string): Promise<{ lat: number; lon: number; address?: string } | null> {
    try {
      return await api.geocode(query)
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null
      if (!hasMapKey()) throw e
      return geocodeAddress(query)
    }
  }

  async function resolve(query: string, label: string) {
    if (query.trim().length < 3) return
    setBusy(true)
    setItems([])
    try {
      const point = await locate(query)
      if (!point) {
        onError('Не нашли такой адрес в Ростове — уточни улицу и дом')
        return
      }
      picked.current = label
      setText(label)
      onPick({ lat: point.lat, lon: point.lon, label })
    } catch {
      onError('Поиск адреса сейчас недоступен — используй геолокацию или район')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="address">
      <div className="row">
        <div className="address__field">
          <MapPin size={18} />
          <input
            className="input input--icon"
            placeholder="Адрес: дом, работа, учёба"
            value={text}
            enterKeyHint="search"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && resolve(text.trim(), text.trim())}
          />
        </div>
        <Btn variant="secondary" loading={busy} disabled={text.trim().length < 3} icon={<Search size={18} />} onClick={() => resolve(text.trim(), text.trim())} />
      </div>
      {items.length > 0 && (
        <div className="address__list">
          {items.map((i) => (
            <button key={i.value} className="address__item" onClick={() => resolve(i.value, i.title)}>
              {i.title}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
