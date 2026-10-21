import { useEffect, useState } from 'react'
import { CheckCircle2, MapPin, MessageCircle, Search } from 'lucide-react'
import { api } from '../api'
import { AddressInput } from '../AddressInput'
import { Btn, Loader } from '../components'
import { closeApp, haptic, insideMax } from '../max'
import { useNav, type Point } from '../nav'

type State = { kind: 'locating' } | { kind: 'manual'; reason: string } | { kind: 'saving' } | { kind: 'done'; point: Point; sport?: string; when?: string }

// Экран для кнопки «Определить через приложение» в чате: на компьютере
// кнопка геолокации в чате не работает, а браузерная геолокация — работает.
// Точка сохраняется на сервере, и бот продолжает подбор с неё.
export function GeoScreen() {
  const nav = useNav()
  const [state, setState] = useState<State>({ kind: 'locating' })

  async function save(point: Point) {
    setState({ kind: 'saving' })
    try {
      const r = await api.saveLocation(point.lat, point.lon, point.label === 'Моё местоположение' ? '' : point.label)
      haptic('success')
      setState({ kind: 'done', point, sport: r.sport, when: r.when })
    } catch (e) {
      setState({ kind: 'manual', reason: e instanceof Error ? e.message : 'Не получилось сохранить точку' })
    }
  }

  function locate() {
    setState({ kind: 'locating' })
    if (!navigator.geolocation) {
      setState({ kind: 'manual', reason: 'Это устройство не отдаёт геолокацию. Введите адрес — так даже точнее.' })
      return
    }
    navigator.geolocation.getCurrentPosition(
      (pos) => void save({ lat: pos.coords.latitude, lon: pos.coords.longitude, label: 'Моё местоположение' }),
      () => setState({ kind: 'manual', reason: 'Доступ к геолокации не выдан. Разрешите его в браузере или введите адрес.' }),
      { timeout: 10000, maximumAge: 300000 },
    )
  }

  useEffect(locate, [])

  if (state.kind === 'locating' || state.kind === 'saving') {
    return (
      <div className="screen">
        <header className="hero">
          <h1 className="h1">{state.kind === 'locating' ? 'Определяем, где вы' : 'Сохраняем точку'}</h1>
          <p className="muted">Если браузер спросит разрешение — разрешите доступ к местоположению.</p>
        </header>
        <Loader />
      </div>
    )
  }

  if (state.kind === 'manual') {
    return (
      <div className="screen">
        <header className="hero">
          <h1 className="h1">Откуда добираться?</h1>
          <p className="muted">{state.reason}</p>
        </header>
        <AddressInput onPick={(p) => void save(p)} onError={(text) => nav.toast(text)} />
        <Btn variant="secondary" block icon={<MapPin size={18} />} onClick={locate}>Попробовать геолокацию ещё раз</Btn>
      </div>
    )
  }

  const { point, sport, when } = state
  return (
    <div className="screen">
      <section className="success success--hero">
        <CheckCircle2 size={48} />
        <h1 className="h1">Точка сохранена</h1>
        <p className="muted">{point.label === 'Моё местоположение' ? 'Геолокация передана в чат' : point.label}. Бот уже продолжил подбор — можно вернуться в чат или смотреть залы здесь.</p>
      </section>
      <div className="stack">
        <Btn size="lg" block icon={<Search size={18} />} onClick={() => nav.tab({ name: 'search', sport, when, point })}>Показать залы рядом</Btn>
        {insideMax() && <Btn size="lg" variant="secondary" block icon={<MessageCircle size={18} />} onClick={closeApp}>Вернуться в чат</Btn>}
      </div>
    </div>
  )
}
