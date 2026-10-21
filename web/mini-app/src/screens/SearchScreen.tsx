import { useCallback, useEffect, useState } from 'react'
import { List, Map as MapIcon, Navigation, X } from 'lucide-react'
import { api, type Booking, type SlotInfo, type TravelMode, type Venue, type VenueCard } from '../api'
import { AddressInput } from '../AddressInput'
import { BookingSheet, Btn, Chips, Empty, ErrorBlock, Loader, Logo, NextWorkout, VenueCardView, modeIcon } from '../components'
import { travelModeTitle } from '../format'
import { MapView } from '../MapView'
import { userFirstName } from '../max'
import { useNav } from '../nav'

type Point = { lat: number; lon: number; label: string }

const modes: TravelMode[] = ['transit', 'walking', 'driving']
const limits = [
  { id: '10', title: 'до 10 мин' },
  { id: '15', title: 'до 15 мин' },
  { id: '20', title: 'до 20 мин' },
  { id: '30', title: 'до 30 мин' },
  { id: '0', title: 'Не важно' },
]

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
  const [mode, setMode] = useState<TravelMode>('transit')
  const [limit, setLimit] = useState('20')
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
      .venues({
        sport,
        when,
        district: me ? '' : district,
        lat: me?.lat,
        lon: me?.lon,
        mode: me ? mode : undefined,
        max_minutes: me ? Number(limit) : undefined,
      })
      .then((r) => setCards(r.venues))
      .catch((e) => setError(e.message))
  }, [sport, when, district, me, mode, limit])

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
        setMe({ lat: pos.coords.latitude, lon: pos.coords.longitude, label: 'Моё местоположение' })
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
  const canWiden = !!me && limit !== '0' && Number(limit) < 60

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

      <section className="place">
        <div className="place__title">Откуда добираться</div>
        {me ? (
          <div className="place__current">
            <Navigation size={16} />
            <span>{me.label}</span>
            <button aria-label="Сбросить точку" onClick={() => setMe(undefined)}><X size={16} /></button>
          </div>
        ) : (
          <>
            <AddressInput onPick={setMe} onError={nav.toast} />
            <div className="row">
              <select className="select" value={district} onChange={(e) => setDistrict(e.target.value)}>
                <option value="">Весь город</option>
                {catalog?.districts.map((d) => (
                  <option key={d.id} value={d.id}>{d.title} район</option>
                ))}
              </select>
              <Btn variant="secondary" loading={locating} icon={<Navigation size={18} />} onClick={locate}>Рядом</Btn>
            </div>
          </>
        )}
        {me && (
          <>
            <div className="modes">
              {modes.map((m) => {
                const Icon = modeIcon[m]
                return (
                  <button key={m} className={m === mode ? 'active' : ''} onClick={() => setMode(m)}>
                    <Icon size={16} /> {travelModeTitle(m)}
                  </button>
                )
              })}
            </div>
            <Chips items={limits} value={limit} onChange={setLimit} tone="navy" />
          </>
        )}
      </section>

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
          {cards.length === 0 && canWiden && (
            <Empty
              title={`За ${limit} мин ${travelModeTitle(mode)} ничего не нашлось`}
              text="Попробуй увеличить время в пути или выбрать другой способ."
              action={<Btn onClick={() => setLimit(String(Math.min(Number(limit) * 2, 60)))}>Искать до {Math.min(Number(limit) * 2, 60)} мин</Btn>}
            />
          )}
          {cards.length === 0 && !canWiden && (
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
