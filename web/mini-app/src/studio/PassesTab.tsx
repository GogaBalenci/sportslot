import { useEffect, useState } from 'react'
import { Check, Pencil, Plus, X } from 'lucide-react'
import { api, type Pass, type StudioCommerce, type UserPass } from '../api'
import { Btn, Empty, ErrorBlock, Loader, Sheet, Toggle } from '../components'
import { shortDay } from '../format'
import { useNav } from '../nav'

const rub = (n: number) => `${n.toLocaleString('ru-RU')} ₽`
const visits = (n: number) => `${n} ${n % 10 === 1 && n % 100 !== 11 ? 'занятие' : [2, 3, 4].includes(n % 10) && ![12, 13, 14].includes(n % 100) ? 'занятия' : 'занятий'}`

function PassForm({ venueId, pass, onClose, onSaved }: { venueId: string; pass: Partial<Pass>; onClose: () => void; onSaved: () => void }) {
  const nav = useNav()
  const [title, setTitle] = useState(pass.title ?? '')
  const [count, setCount] = useState(pass.visits ?? 8)
  const [days, setDays] = useState(pass.valid_days ?? 30)
  const [price, setPrice] = useState(pass.price ?? 3000)
  const [active, setActive] = useState(pass.active ?? true)
  const [busy, setBusy] = useState(false)
  async function save() {
    setBusy(true)
    try {
      await api.savePass(venueId, { id: pass.id, title, visits: count, valid_days: days, price, active })
      nav.toast('Абонемент сохранён')
      onSaved()
      onClose()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось сохранить')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Sheet onClose={onClose}>
      <h2 className="h2">{pass.id ? 'Изменить абонемент' : 'Новый абонемент'}</h2>
      <label className="field"><span>Название</span>
        <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="8 занятий на месяц" /></label>
      <div className="row">
        <label className="field"><span>Занятий</span>
          <input className="input" type="number" min={1} max={100} value={count} onChange={(e) => setCount(Number(e.target.value))} /></label>
        <label className="field"><span>Действует, дней</span>
          <input className="input" type="number" min={1} max={365} value={days} onChange={(e) => setDays(Number(e.target.value))} /></label>
      </div>
      <label className="field"><span>Цена, ₽</span>
        <input className="input" type="number" min={1} value={price} onChange={(e) => setPrice(Number(e.target.value))} /></label>
      <Toggle checked={active} onChange={setActive} label="Продаётся" hint="Выключите, чтобы скрыть из карточки студии: купленные абонементы продолжат работать" />
      <p className="muted small">Срок считается с момента оплаты. При записи визит списывается автоматически, при отмене записи — возвращается.</p>
      <Btn size="lg" block loading={busy} disabled={title.trim().length < 2} onClick={save}>Сохранить</Btn>
    </Sheet>
  )
}

function RequestRow({ venueId, up, onDone }: { venueId: string; up: UserPass; onDone: () => void }) {
  const nav = useNav()
  const [busy, setBusy] = useState(false)
  async function act(confirm: boolean) {
    setBusy(true)
    try {
      if (confirm) await api.confirmPass(venueId, up.id)
      else await api.declinePass(venueId, up.id)
      nav.toast(confirm ? 'Абонемент включён — клиенту пришло сообщение' : 'Заявка отклонена')
      onDone()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="list__row list__row--static">
      <div className="list__main">
        <div className="list__title">{up.user_name || 'Клиент'}</div>
        <div className="muted small">{up.title} · {rub(up.price)} · заявка {shortDay(up.created_at)}</div>
      </div>
      <button className="icon-round" disabled={busy} aria-label="Оплачен, включить" onClick={() => act(true)}><Check size={18} /></button>
      <button className="icon-round icon-round--danger" disabled={busy} aria-label="Отклонить" onClick={() => act(false)}><X size={18} /></button>
    </div>
  )
}

// Абонементы: что продаёт студия, заявки с оплатой на месте и активные абонементы клиентов.
export function PassesTab({ venueId }: { venueId: string }) {
  const [data, setData] = useState<StudioCommerce | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Partial<Pass> | null>(null)
  const load = () => {
    setError('')
    api.studioCommerce(venueId).then(setData).catch((e) => setError(e.message))
  }
  useEffect(load, [venueId])

  if (error) return <ErrorBlock text={error} onRetry={load} />
  if (!data) return <Loader />
  const requests = data.user_passes.filter((p) => p.status === 'requested')
  const active = data.user_passes.filter((p) => p.status === 'active')

  return (
    <>
      {requests.length > 0 && (
        <>
          <h2 className="h2">Заявки · {requests.length}</h2>
          <p className="muted small">Клиент оплатит в студии. Нажмите ✓, когда получите оплату — абонемент включится и клиенту придёт сообщение.</p>
          <div className="list">
            {requests.map((up) => <RequestRow key={up.id} venueId={venueId} up={up} onDone={load} />)}
          </div>
        </>
      )}

      <div className="studio-head">
        <h2 className="h2">Абонементы студии</h2>
        <Btn icon={<Plus size={16} />} onClick={() => setEditing({})}>Абонемент</Btn>
      </div>
      {data.passes.length === 0 ? (
        <Empty title="Абонементов пока нет" text="Добавьте абонемент — он появится в карточке студии. Если подключена оплата, клиент оплатит его онлайн." />
      ) : (
        <div className="stack">
          {data.passes.map((p) => (
            <div key={p.id} className={`tpl${p.active ? '' : ' list__row--muted'}`}>
              <div className="tpl__main">
                <b>{p.title}</b>
                <div className="muted small">{visits(p.visits)} · {p.valid_days} дн. · {rub(p.price)}{p.active ? '' : ' · не продаётся'}</div>
              </div>
              <button className="icon-round" aria-label="Изменить" onClick={() => setEditing(p)}><Pencil size={16} /></button>
            </div>
          ))}
        </div>
      )}

      {active.length > 0 && (
        <>
          <h2 className="h2 section">Активные у клиентов</h2>
          <div className="list">
            {active.map((up) => (
              <div key={up.id} className="list__row list__row--static">
                <div className="list__main">
                  <div className="list__title">{up.user_name || 'Клиент'}</div>
                  <div className="muted small">{up.title} · до {up.valid_until ? shortDay(up.valid_until) : '—'}{up.paid_by === 'online' ? ' · онлайн' : ''}</div>
                </div>
                <span className="status status--confirmed">{up.visits_left} из {up.visits_total}</span>
              </div>
            ))}
          </div>
        </>
      )}
      {editing && <PassForm venueId={venueId} pass={editing} onClose={() => setEditing(null)} onSaved={load} />}
    </>
  )
}
