import { useEffect, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import type { VenueCard } from './api'

const ROSTOV: L.LatLngExpression = [47.2357, 39.7015]

// Карта площадок: партнёры с онлайн-записью — крупные акцентные точки,
// площадки из OpenStreetMap — серые. Подложка — тайлы OpenStreetMap.
export function MapView(props: { cards: VenueCard[]; me?: { lat: number; lon: number }; onSelect: (id: string) => void }) {
  const holder = useRef<HTMLDivElement>(null)
  const map = useRef<L.Map | null>(null)
  const layer = useRef<L.LayerGroup | null>(null)

  useEffect(() => {
    if (!holder.current || map.current) return
    map.current = L.map(holder.current, { zoomControl: false, attributionControl: true }).setView(ROSTOV, 12)
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 18,
      attribution: '© <a href="https://www.openstreetmap.org/copyright">участники OpenStreetMap</a>',
    }).addTo(map.current)
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
    const accent = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#0a66ff'
    props.cards.forEach((c) => {
      const partner = c.venue.booking_mode === 'instant'
      const marker = L.circleMarker([c.venue.lat, c.venue.lon], {
        radius: partner ? 10 : 6,
        color: '#ffffff',
        weight: 2,
        fillColor: partner ? accent : '#8a8f99',
        fillOpacity: 1,
      })
      marker.bindTooltip(c.venue.name, { direction: 'top', offset: [0, -8] })
      marker.on('click', () => props.onSelect(c.venue.id))
      marker.addTo(group)
      points.push([c.venue.lat, c.venue.lon])
    })
    if (props.me) {
      L.circleMarker([props.me.lat, props.me.lon], { radius: 7, color: '#fff', weight: 3, fillColor: '#1a1a1a', fillOpacity: 1 })
        .bindTooltip('Вы здесь')
        .addTo(group)
      points.push([props.me.lat, props.me.lon])
    }
    if (points.length > 1) map.current.fitBounds(L.latLngBounds(points), { padding: [28, 28], maxZoom: 15 })
    else if (points.length === 1) map.current.setView(points[0], 14)
  }, [props.cards, props.me, props.onSelect])

  return <div className="map" ref={holder} />
}
