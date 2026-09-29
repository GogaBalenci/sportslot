import { useEffect, useRef, useState } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import type { VenueCard } from './api'

const ROSTOV: [number, number] = [47.2357, 39.7015]
const ORANGE = '#FF5722'
const GREY = '#94A3B8'
const YANDEX_KEY = import.meta.env.VITE_YANDEX_MAPS_API_KEY ?? ''

type Props = { cards: VenueCard[]; me?: { lat: number; lon: number }; onSelect: (id: string) => void }

// Карта площадок: Яндекс Карты, если задан ключ JavaScript API,
// иначе запасная карта на Leaflet. Студии с онлайн-записью — оранжевые метки.
export function MapView(props: Props) {
  const [yandexFailed, setYandexFailed] = useState(false)
  if (YANDEX_KEY && !yandexFailed) return <YandexMap {...props} onError={() => setYandexFailed(true)} />
  return <FallbackMap {...props} />
}

let ymapsLoader: Promise<any> | null = null

function loadYandexMaps(): Promise<any> {
  if (window.ymaps) return new Promise((resolve) => window.ymaps.ready(() => resolve(window.ymaps)))
  ymapsLoader ??= new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = `https://api-maps.yandex.ru/2.1/?apikey=${encodeURIComponent(YANDEX_KEY)}&lang=ru_RU`
    script.onload = () => window.ymaps.ready(() => resolve(window.ymaps))
    script.onerror = () => {
      ymapsLoader = null
      reject(new Error('yandex maps are unavailable'))
    }
    document.head.appendChild(script)
  })
  return ymapsLoader
}

function YandexMap({ cards, me, onSelect, onError }: Props & { onError: () => void }) {
  const holder = useRef<HTMLDivElement>(null)
  const map = useRef<any>(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let cancelled = false
    loadYandexMaps()
      .then((ymaps) => {
        if (cancelled || !holder.current) return
        map.current = new ymaps.Map(holder.current, { center: ROSTOV, zoom: 12, controls: ['zoomControl'] }, { suppressMapOpenBlock: true })
        setReady(true)
      })
      .catch(onError)
    return () => {
      cancelled = true
      map.current?.destroy()
      map.current = null
    }
  }, [onError])

  useEffect(() => {
    const ymaps = window.ymaps
    if (!ready || !map.current || !ymaps) return
    map.current.geoObjects.removeAll()
    const collection = new ymaps.GeoObjectCollection()
    cards.forEach((c) => {
      const partner = c.venue.booking_mode === 'instant'
      const mark = new ymaps.Placemark(
        [c.venue.lat, c.venue.lon],
        { hintContent: c.venue.name },
        { preset: partner ? 'islands#circleDotIcon' : 'islands#circleIcon', iconColor: partner ? ORANGE : GREY },
      )
      mark.events.add('click', () => onSelect(c.venue.id))
      collection.add(mark)
    })
    if (me) collection.add(new ymaps.Placemark([me.lat, me.lon], { hintContent: 'Ты здесь' }, { preset: 'islands#geolocationIcon' }))
    map.current.geoObjects.add(collection)
    if (collection.getLength() > 1) map.current.setBounds(collection.getBounds(), { checkZoomRange: true, zoomMargin: 36 })
    else if (collection.getLength() === 1) map.current.setCenter(collection.get(0).geometry.getCoordinates(), 14)
  }, [ready, cards, me, onSelect])

  return <div className="map" ref={holder} />
}

function FallbackMap({ cards, me, onSelect }: Props) {
  const holder = useRef<HTMLDivElement>(null)
  const map = useRef<L.Map | null>(null)
  const layer = useRef<L.LayerGroup | null>(null)

  useEffect(() => {
    if (!holder.current || map.current) return
    map.current = L.map(holder.current, { zoomControl: false }).setView(ROSTOV, 12)
    map.current.attributionControl.setPrefix(false)
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', { maxZoom: 18, attribution: '© OpenStreetMap' }).addTo(map.current)
    L.control.zoom({ position: 'bottomright' }).addTo(map.current)
    layer.current = L.layerGroup().addTo(map.current)
    return () => {
      map.current?.remove()
      map.current = null
    }
  }, [])

  useEffect(() => {
    const group = layer.current
    if (!group || !map.current) return
    group.clearLayers()
    const points: L.LatLngExpression[] = []
    cards.forEach((c) => {
      const partner = c.venue.booking_mode === 'instant'
      L.circleMarker([c.venue.lat, c.venue.lon], {
        radius: partner ? 10 : 6, color: '#fff', weight: 2, fillColor: partner ? ORANGE : GREY, fillOpacity: 1,
      })
        .bindTooltip(c.venue.name, { direction: 'top', offset: [0, -8] })
        .on('click', () => onSelect(c.venue.id))
        .addTo(group)
      points.push([c.venue.lat, c.venue.lon])
    })
    if (me) {
      L.circleMarker([me.lat, me.lon], { radius: 7, color: '#fff', weight: 3, fillColor: '#1A237E', fillOpacity: 1 })
        .bindTooltip('Ты здесь')
        .addTo(group)
      points.push([me.lat, me.lon])
    }
    if (points.length > 1) map.current.fitBounds(L.latLngBounds(points), { padding: [28, 28], maxZoom: 15 })
    else if (points.length === 1) map.current.setView(points[0], 14)
  }, [cards, me, onSelect])

  return <div className="map" ref={holder} />
}
