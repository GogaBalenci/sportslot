import { useEffect, useRef, useState } from 'react'
import { MapPin, Search } from 'lucide-react'
import { Btn } from './components'
import { geocodeAddress, hasMapKey, hasSuggestKey, suggestAddress, type AddressSuggestion } from './ymaps'

type Props = { onPick: (point: { lat: number; lon: number; label: string }) => void; onError: (text: string) => void }

// Ввод адреса для расчёта времени в пути. С ключом Геосаджеста показываются
// подсказки; без него адрес ищется по кнопке. Запросы к подсказкам — не чаще
// чем после паузы в наборе и от трёх символов, чтобы не расходовать лимит.
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

  if (!hasMapKey()) return null

  async function resolve(query: string, label: string) {
    setBusy(true)
    setItems([])
    try {
      const point = await geocodeAddress(query)
      if (!point) {
        onError('Не нашли такой адрес в Ростове')
        return
      }
      picked.current = label
      setText(label)
      onPick({ ...point, label })
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
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && text.trim() && resolve(text.trim(), text.trim())}
          />
        </div>
        {!hasSuggestKey() && (
          <Btn variant="secondary" loading={busy} disabled={!text.trim()} icon={<Search size={18} />} onClick={() => resolve(text.trim(), text.trim())} />
        )}
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
