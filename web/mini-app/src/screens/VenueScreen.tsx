import { useEffect, useMemo, useState } from 'react'
import { Button, Typography } from '@maxhub/max-ui'
import { api, type SlotInfo, type VenueDetails } from '../api'
import { BookingSheet, ErrorBlock, Loader, SourceBadge, districtTitle } from '../components'
import { catalogDate, dayKey, dayTitle, hours, routeUrl, seats, time, trial } from '../format'
import { openExternal } from '../max'
import { useNav } from '../nav'

export function VenueScreen({ id, reschedule }: { id: string; reschedule?: string }) {
  const nav = useNav()
  const [venue, setVenue] = useState<VenueDetails | null>(null)
  const [error, setError] = useState('')
  const [picked, setPicked] = useState<SlotInfo | null>(null)

  const load = () => {
    setError('')
    api.venue(id).then(setVenue).catch((e) => setError(e.message))
  }
  useEffect(load, [id])

  const days = useMemo(() => {
    const groups: { key: string; title: string; slots: SlotInfo[] }[] = []
    venue?.all_slots.forEach((s) => {
      const key = dayKey(s.start_at)
      let g = groups.find((x) => x.key === key)
      if (!g) {
        g = { key, title: dayTitle(s.start_at), slots: [] }
        groups.push(g)
      }
      g.slots.push(s)
    })
    return groups
  }, [venue])

  if (error) return <div className="screen"><ErrorBlock text={error} onRetry={load} /></div>
  if (!venue) return <div className="screen"><Loader /></div>

  const website = venue.website && venue.website.startsWith('http') ? venue.website : ''
  return (
    <div className="screen">
      <header className="screen__header">
        <SourceBadge venue={venue} />
        <Typography.Headline variant="medium">{venue.name}</Typography.Headline>
        <div className="muted">{[districtTitle(venue.district, nav.catalog?.districts), venue.address].filter(Boolean).join(' · ')}</div>
      </header>

      {reschedule && <div className="notice">Выберите новое время — код входа останется прежним.</div>}

      <div className="actions-row">
        <Button variant="secondary" onClick={() => openExternal(routeUrl(venue.lat, venue.lon))}>Маршрут</Button>
        {website && <Button variant="secondary" onClick={() => openExternal(website)}>Сайт</Button>}
        {venue.phone && (
          <Button variant="secondary" asChild>
            <a href={`tel:${venue.phone.replace(/[^+\d]/g, '')}`}>Позвонить</a>
          </Button>
        )}
      </div>

      {venue.booking_mode === 'instant' ? (
        <>
          {trial(venue.trial_price) && <div className="accent-text">{trial(venue.trial_price)}</div>}
          {venue.what_to_bring && <p className="muted">Что взять: {venue.what_to_bring}</p>}
          {days.length === 0 && <div className="empty">На ближайшие две недели занятий нет.</div>}
          {days.map((d) => (
            <section key={d.key}>
              <h3 className="section">{d.title}</h3>
              <div className="list">
                {d.slots.map((s) => (
                  <button key={s.slot_id} className="slot-row" onClick={() => setPicked(s)}>
                    <b className="slot-row__time">{time(s.start_at)}</b>
                    <span className="slot-row__title">{s.title}</span>
                    <span className={s.quota_available === 0 ? 'slot-row__seats full' : 'slot-row__seats'}>
                      {s.quota_available === 0 ? 'в очередь' : seats(s.quota_available)}
                    </span>
                  </button>
                ))}
              </div>
            </section>
          ))}
          {venue.description && <p className="muted small">{venue.description}</p>}
        </>
      ) : (
        <div className="info">
          {venue.opening_hours && <p><b>Часы работы:</b> {hours(venue.opening_hours)}</p>}
          {venue.phone && <p><b>Телефон:</b> {venue.phone}</p>}
          <p className="muted">
            Онлайн-записи через СпортСлот здесь пока нет. Расписание и стоимость пробного занятия уточните у зала.
          </p>
          <p className="muted small">
            Источник: OpenStreetMap, выгрузка {catalogDate(venue.verified_at)}.{' '}
            {venue.source_url && <a href={venue.source_url} onClick={(e) => { e.preventDefault(); openExternal(venue.source_url!) }}>Объект на карте OSM</a>}
          </p>
        </div>
      )}

      {picked && <BookingSheet venue={venue} slot={picked} reschedule={reschedule} onClose={() => setPicked(null)} />}
    </div>
  )
}
