import { useCallback, useEffect, useState } from 'react'
import { List, Map as MapIcon, Navigation } from 'lucide-react'
import { api, type Booking, type SlotInfo, type Venue, type VenueCard } from '../api'
import { BookingSheet, Btn, Chips, Empty, ErrorBlock, Loader, Logo, NextWorkout, VenueCardView } from '../components'
import { MapView } from '../MapView'
import { userFirstName } from '../max'
import { useNav } from '../nav'

type Point = { lat: number; lon: number }

export function SearchScreen({ initialSport }: { initialSport?: string }) {
  const nav = useNav()
  const catalog = nav.catalog
  const [sport, setSport] = useState(initialSport ?? '')
  const [when, setWhen] = useState('any')
  const [district, setDistrict] = useState('')
  const [me, setMe] = useState<Point | undefined>()
  const [view, setView] = useState<'list' | 'map'>('list')
  const [cards, setCards] = useState<VenueCard[] | null>(null)
  const [error, setError] = useState('')
  const [picked, setPicked] = useState<{ venue: Venue; slot: SlotInfo } | null>(null)
  const [locating, setLocating] = useState(false)
  const [next, setNext] = useState<Booking | null>(null)

  useEffect(() => {
    api
      .me()
      .then((r) => {
        const upcoming = r.bookings
          .filter((b) => b.status === 'confirmed' && new Date(b.slot.end_at).getTime() > Date.now())
          .sort((a, b) => a.slot.start_at.localeCompare(b.slot.start_at))
        setNext(upcoming[0] ?? null)
      })
      .catch(() => setNext(null))
  }, [])

  const load = useCallback(() => {
    setCards(null)
    setError('')
    api
      .venues({ sport, when, district: me ? '' : district, lat: me?.lat, lon: me?.lon })
      .then((r) => setCards(r.venues))
      .catch((e) => setError(e.message))
  }, [sport, when, district, me])

  useEffect(load, [load])

  function locate() {
    if (!navigator.geolocation) {
      nav.toast('Геолокация недоступна — выбери район')
      return
    }
    setLocating(true)
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        setLocating(false)
        setMe({ lat: pos.coords.latitude, lon: pos.coords.longitude })
        setDistrict('')
      },
      () => {
        setLocating(false)
        nav.toast('Не удалось определить местоположение — выбери район')
      },
      { timeout: 8000, maximumAge: 300000 },
    )
  }

  const sports = [{ id: '', title: 'Все' }, ...(catalog?.sports.filter((s) => s.id !== 'multi').map((s) => ({ id: s.id, title: s.title })) ?? [])]
  const bookable = cards?.filter((c) => c.venue.booking_mode === 'instant') ?? []
  const other = cards?.filter((c) => c.venue.booking_mode !== 'instant') ?? []
  const onSelect = useCallback((id: string) => nav.push({ name: 'venue', id }), [nav])
  const name = userFirstName()

  return (
    <div className="screen">
      <header className="hero">
        <Logo />
        <h1 className="h1">{name ? `${name}, найдём тренировку рядом?` : 'Найдём тренировку рядом?'}</h1>
        <p className="muted">Пробное занятие в Ростове — запись в пару касаний</p>
      </header>

      {next && <NextWorkout booking={next} />}

      <Chips items={sports} value={sport} onChange={setSport} />
      <Chips items={catalog?.whens ?? []} value={when} onChange={setWhen} tone="navy" />

      <div className="row">
        <select
          className="select"
          value={me ? 'geo' : district}
          onChange={(e) => {
            setMe(undefined)
            setDistrict(e.target.value === 'geo' ? '' : e.target.value)
          }}
        >
          <option value="">Весь город</option>
          {me && <option value="geo">Рядом со мной</option>}
          {catalog?.districts.map((d) => (
            <option key={d.id} value={d.id}>{d.title} район</option>
          ))}
        </select>
        <Btn variant="secondary" loading={locating} icon={<Navigation size={18} />} onClick={locate}>Рядом</Btn>
      </div>

      <div className="segmented">
        <button className={view === 'list' ? 'active' : ''} onClick={() => setView('list')}><List size={16} /> Список</button>
        <button className={view === 'map' ? 'active' : ''} onClick={() => setView('map')}><MapIcon size={16} /> Карта</button>
      </div>

      {error && <ErrorBlock text={error} onRetry={load} />}
      {!error && !cards && <Loader />}

      {cards && view === 'map' && (
        <>
          <MapView cards={cards} me={me} onSelect={onSelect} />
          <div className="legend">
            <span><i className="dot dot--orange" /> онлайн-запись</span>
            <span><i className="dot" /> запись в зале</span>
          </div>
        </>
      )}

      {cards && view === 'list' && (
        <>
          {cards.length === 0 && (
            <Empty
              title="Здесь пока пусто"
              text="Попробуй другое время или весь город."
              action={<Btn variant="secondary" onClick={() => { setWhen('any'); setDistrict(''); setMe(undefined) }}>Сбросить фильтры</Btn>}
            />
          )}
          {bookable.length > 0 && <h2 className="h2 section">Запишись онлайн</h2>}
          {bookable.map((c) => (
            <VenueCardView key={c.venue.id} card={c} sport={sport} onSlot={(venue, slot) => setPicked({ venue, slot })} />
          ))}
          {other.length > 0 && <h2 className="h2 section">Ещё залы рядом</h2>}
          {other.slice(0, 30).map((c) => (
            <VenueCardView key={c.venue.id} card={c} sport={sport} onSlot={() => undefined} />
          ))}
        </>
      )}

      {picked && <BookingSheet venue={picked.venue} slot={picked.slot} onClose={() => setPicked(null)} />}
    </div>
  )
}
