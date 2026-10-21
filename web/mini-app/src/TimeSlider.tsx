import { haptic } from './max'

export const TIME_STOPS = [10, 15, 20, 30, 45, 60]

// Ползунок лимита времени в пути с засечками на удобных значениях.
export function TimeSlider({ value, onChange, label }: { value: number; onChange: (v: number) => void; label: string }) {
  const index = Math.max(0, TIME_STOPS.indexOf(value))
  const percent = (index / (TIME_STOPS.length - 1)) * 100
  return (
    <div className="slider">
      <div className="slider__head">
        <span>Время в пути</span>
        <b>до {value} мин {label}</b>
      </div>
      <input
        type="range"
        min={0}
        max={TIME_STOPS.length - 1}
        step={1}
        value={index}
        style={{ '--fill': `${percent}%` } as React.CSSProperties}
        onChange={(e) => {
          haptic('tap')
          onChange(TIME_STOPS[Number(e.target.value)])
        }}
        aria-label="Время в пути, минут"
      />
      <div className="slider__ticks">
        {TIME_STOPS.map((m, i) => (
          <button key={m} className={i === index ? 'active' : ''} onClick={() => onChange(m)}>
            {m}
          </button>
        ))}
      </div>
    </div>
  )
}
