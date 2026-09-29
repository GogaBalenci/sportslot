import { useCallback, useEffect, useState } from 'react'
import { Button, Typography } from '@maxhub/max-ui'
import { api, type SlotInfo, type Venue, type VenueCard } from '../api'
import { BookingSheet, Chips, ErrorBlock, Loader, VenueCardView } from '../components'
import { catalogDate } from '../format'
import { MapView } from '../MapView'
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
      nav.toast('Геолокация недоступна — выберите район')
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
        nav.toast('Не удалось определить местоположение — выберите район')
      },
      { timeout: 8000, maximumAge: 300000 },
    )
  }

  const sports = [{ id: '', title: 'Все' }, ...(catalog?.sports.filter((s) => s.id !== 'multi').map((s) => ({ id: s.id, title: s.short })) ?? [])]
  const bookable = cards?.filter((c) => c.venue.booking_mode === 'instant') ?? []
  const external = cards?.filter((c) => c.venue.booking_mode !== 'instant') ?? []
  const onSelect = useCallback((id: string) => nav.push({ name: 'venue', id }), [nav])

  return (
    <div className="screen">
      <header className="screen__header">
        <Typography.Headline variant="medium">Тренировки в Ростове</Typography.Headline>
        <div className="muted">Пробное занятие рядом с домом — запись в пару касаний</div>
      </header>

      <Chips items={sports} value={sport} onChange={setSport} />
      <Chips items={catalog?.whens ?? []} value={when} onChange={setWhen} />

      <div className="filters">
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
            <option key={d.id} value={d.id}>
              {d.title} район
            </option>
          ))}
        </select>
        <Button variant="secondary" size="medium" loading={locating} onClick={locate}>
          Рядом со мной
        </Button>
      </div>

      <div className="segmented">
        <button className={view === 'list' ? 'active' : ''} onClick={() => setView('list')}>Список</button>
        <button className={view === 'map' ? 'active' : ''} onClick={() => setView('map')}>Карта</button>
      </div>

      {error && <ErrorBlock text={error} onRetry={load} />}
      {!error && !cards && <Loader />}

      {cards && view === 'map' && (
        <>
          <MapView cards={cards} me={me} onSelect={onSelect} />
          <div className="legend">
            <span><i className="dot dot--accent" /> онлайн-запись</span>
            <span><i className="dot" /> каталог OpenStreetMap</span>
          </div>
        </>
      )}

      {cards && view === 'list' && (
        <>
          {cards.length === 0 && (
            <div className="empty">
              <div>Ничего не нашлось. Попробуйте другое время или весь город.</div>
              <Button variant="secondary" onClick={() => { setWhen('any'); setDistrict(''); setMe(undefined) }}>
                Сбросить фильтры
              </Button>
            </div>
          )}
          {bookable.length > 0 && <h3 className="section">Записаться онлайн</h3>}
          {bookable.map((c) => (
            <VenueCardView key={c.venue.id} card={c} onSlot={(venue, slot) => setPicked({ venue, slot })} />
          ))}
          {external.length > 0 && <h3 className="section">Залы из открытого каталога</h3>}
          {external.slice(0, 30).map((c) => (
            <VenueCardView key={c.venue.id} card={c} onSlot={() => undefined} />
          ))}
        </>
      )}

      <footer className="sources">
        Каталог залов: © участники OpenStreetMap, выгрузка {catalogDate(catalog?.data_sources.catalog_date)}.
        {catalog?.data_sources.demo_partners && ' Студии с онлайн-записью — демо-партнёры с модельным расписанием.'}
      </footer>

      {picked && <BookingSheet venue={picked.venue} slot={picked.slot} onClose={() => setPicked(null)} />}
    </div>
  )
}
