const weekdays = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб']
const weekdaysLong = ['Воскресенье', 'Понедельник', 'Вторник', 'Среда', 'Четверг', 'Пятница', 'Суббота']
const months = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря']

// Все даты показываем по времени Ростова-на-Дону.
const TZ = 'Europe/Moscow'

function parts(iso: string) {
  const d = new Date(iso)
  const p = new Intl.DateTimeFormat('en-GB', {
    timeZone: TZ, year: 'numeric', month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', weekday: 'short', hour12: false,
  }).formatToParts(d)
  const get = (t: string) => p.find((x) => x.type === t)?.value ?? ''
  const wd = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].indexOf(get('weekday'))
  return { y: +get('year'), m: +get('month'), d: +get('day'), hm: `${get('hour')}:${get('minute')}`, wd }
}

export const time = (iso: string) => parts(iso).hm

export function dayKey(iso: string): string {
  const p = parts(iso)
  return `${p.y}-${p.m}-${p.d}`
}

export function dayTitle(iso: string): string {
  const p = parts(iso)
  const today = parts(new Date().toISOString())
  const tomorrow = parts(new Date(Date.now() + 864e5).toISOString())
  const prefix = p.d === today.d && p.m === today.m ? 'Сегодня, ' : p.d === tomorrow.d && p.m === tomorrow.m ? 'Завтра, ' : `${weekdaysLong[p.wd]}, `
  return `${prefix}${p.d} ${months[p.m - 1]}`
}

export function shortDay(iso: string): string {
  const p = parts(iso)
  return `${weekdays[p.wd]} ${p.d}.${String(p.m).padStart(2, '0')}`
}

export function seats(n: number): string {
  if (n <= 0) return 'мест нет'
  const w = n % 10 === 1 && n % 100 !== 11 ? 'место' : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20) ? 'места' : 'мест'
  return n <= 2 ? `осталось ${n} ${w}` : `${n} ${w}`
}

export function distance(km: number): string {
  return km < 1 ? `${Math.round((km * 1000) / 50) * 50} м` : `${km.toFixed(1).replace('.', ',')} км`
}

export function trial(price?: number): string {
  if (price === undefined || price === null) return ''
  return price === 0 ? 'Пробное бесплатно' : `Пробное — ${price} ₽`
}

export const code = (c: string) => (c.length === 6 ? `${c.slice(0, 3)} ${c.slice(3)}` : c)

export const routeUrl = (lat: number, lon: number) => `https://yandex.ru/maps/?rtext=~${lat},${lon}&rtt=auto`

const osmDays: [RegExp, string][] = [
  [/Mo/g, 'пн'], [/Tu/g, 'вт'], [/We/g, 'ср'], [/Th/g, 'чт'], [/Fr/g, 'пт'], [/Sa/g, 'сб'], [/Su/g, 'вс'],
  [/PH/g, 'праздники'], [/off/g, 'выходной'], [/24\/7/g, 'круглосуточно'], [/;/g, ','],
]

export function hours(raw?: string): string {
  return osmDays.reduce((s, [re, to]) => s.replace(re, to), raw ?? '')
}

export function catalogDate(iso?: string): string {
  if (!iso) return ''
  const p = parts(iso)
  return `${String(p.d).padStart(2, '0')}.${String(p.m).padStart(2, '0')}.${p.y}`
}

export function dayNumber(iso: string): { weekday: string; day: number; month: string } {
  const p = parts(iso)
  return { weekday: weekdays[p.wd], day: p.d, month: months[p.m - 1].slice(0, 3) }
}

// until — «через 40 мин», «через 3 ч 10 мин» или «завтра в 19:00».
export function until(iso: string): string {
  const minutes = Math.round((new Date(iso).getTime() - Date.now()) / 60000)
  if (minutes <= 0) return 'уже началась'
  if (minutes < 60) return `через ${minutes} мин`
  if (minutes < 12 * 60) {
    const m = minutes % 60
    return `через ${Math.floor(minutes / 60)} ч${m ? ` ${m} мин` : ''}`
  }
  return `${dayTitle(iso).toLowerCase()} в ${time(iso)}`
}

// nextDays — ключи и даты на count дней вперёд, начиная с сегодня.
export function nextDays(count: number): { key: string; iso: string }[] {
  return Array.from({ length: count }, (_, i) => {
    const iso = new Date(Date.now() + i * 864e5).toISOString()
    return { key: dayKey(iso), iso }
  })
}
