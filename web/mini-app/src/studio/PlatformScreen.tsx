import { useEffect, useState } from 'react'
import { Copy, Link2, Plus, Search, Share2, UserMinus } from 'lucide-react'
import { api, type PlatformStudio, type Venue } from '../api'
import { Btn, Chips, Empty, ErrorBlock, Loader, Sheet } from '../components'
import { shortDay } from '../format'
import { copyText } from '../max'
import { useNav } from '../nav'

type Invite = { venue: Venue; link: string; expires_at: string }

function InviteSheet({ invite, onClose }: { invite: Invite; onClose: () => void }) {
  const nav = useNav()
  const text = `Приглашение в кабинет студии «${invite.venue.name}» в СпортСлот: ${invite.link}`
  return (
    <Sheet onClose={onClose}>
      <h2 className="h2">Приглашение готово</h2>
      <p className="muted">Отправьте ссылку администратору студии. Он откроет её в MAX и сразу попадёт в кабинет. Ссылка одноразовая, действует до {shortDay(invite.expires_at)}.</p>
      <div className="invite-link">{invite.link}</div>
      <div className="stack">
        <Btn size="lg" block icon={<Copy size={18} />} onClick={async () => nav.toast((await copyText(text)) ? 'Скопировано' : 'Не получилось скопировать — выделите ссылку вручную')}>
          Скопировать
        </Btn>
        {typeof navigator.share === 'function' && (
          <Btn size="lg" variant="secondary" block icon={<Share2 size={18} />} onClick={() => navigator.share({ text }).catch(() => undefined)}>
            Поделиться
          </Btn>
        )}
      </div>
    </Sheet>
  )
}

function StudioRow({ item, onInvite, onChanged }: { item: PlatformStudio; onInvite: (v: Venue) => void; onChanged: () => void }) {
  const nav = useNav()
  const demo = item.venue.source === 'demo_partner'
  async function remove(maxUserId: string) {
    try {
      await api.platformRemoveAdmin(item.venue.id, maxUserId)
      nav.toast('Доступ отозван')
      onChanged()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось')
    }
  }
  return (
    <section className="info-card platform-studio">
      <div className="platform-studio__head">
        <div>
          <div className="list__title">{item.venue.name}</div>
          <div className="muted small">{item.venue.address}{demo ? ' · демо' : ''}</div>
        </div>
        <button className="icon-round" aria-label="Открыть кабинет" onClick={() => nav.push({ name: 'studio', venueId: item.venue.id })}>
          <Link2 size={18} />
        </button>
      </div>
      {item.admins.length > 0 ? (
        <ul className="admins">
          {item.admins.map((a) => (
            <li key={a.max_user_id}>
              <span>{a.name || `MAX ${a.max_user_id}`}</span>
              <button className="icon-round icon-round--danger" aria-label="Отозвать доступ" onClick={() => remove(a.max_user_id)}><UserMinus size={16} /></button>
            </li>
          ))}
        </ul>
      ) : (
        <div className="muted small">Администраторов пока нет{item.invites.length ? ` · ожидает приглашение до ${shortDay(item.invites[0].expires_at)}` : ''}</div>
      )}
      <Btn variant="secondary" block icon={<Plus size={16} />} onClick={() => onInvite(item.venue)}>Ссылка-приглашение</Btn>
    </section>
  )
}

function Connect({ onDone }: { onDone: (v: Venue) => void }) {
  const nav = useNav()
  const [mode, setMode] = useState<'catalog' | 'new'>('catalog')
  const [q, setQ] = useState('')
  const [found, setFound] = useState<Venue[] | null>(null)
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [phone, setPhone] = useState('')
  const [sports, setSports] = useState<string[]>([])
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (mode !== 'catalog' || q.trim().length < 2) {
      setFound(null)
      return
    }
    const t = window.setTimeout(() => {
      api.platformCatalog(q).then((r) => setFound(r.venues)).catch(() => setFound([]))
    }, 300)
    return () => window.clearTimeout(t)
  }, [q, mode])

  async function run(action: () => Promise<Venue>) {
    setBusy(true)
    try {
      const v = await action()
      nav.toast(`«${v.name}» подключена к онлайн-записи`)
      onDone(v)
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось')
    } finally {
      setBusy(false)
    }
  }

  const allSports = nav.catalog?.sports.filter((s) => s.id !== 'multi') ?? []
  return (
    <section className="info-card">
      <h2 className="h2">Подключить студию</h2>
      <Chips items={[{ id: 'catalog', title: 'Из каталога' }, { id: 'new', title: 'Новая' }]} value={mode} onChange={setMode} tone="navy" />
      {mode === 'catalog' ? (
        <>
          <div className="search-input">
            <Search size={18} />
            <input className="input" placeholder="Название или адрес" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
          {found && found.length === 0 && <div className="muted small">В каталоге не нашлось — добавьте как новую.</div>}
          {found && found.length > 0 && (
            <div className="list">
              {found.map((v) => (
                <button key={v.id} className="list__row" disabled={busy} onClick={() => run(() => api.platformConnect(v.id))}>
                  <div className="list__main">
                    <div className="list__title">{v.name}</div>
                    <div className="muted small">{v.address}</div>
                  </div>
                  <Plus size={18} />
                </button>
              ))}
            </div>
          )}
        </>
      ) : (
        <>
          <label className="field"><span>Название</span><input className="input" value={name} onChange={(e) => setName(e.target.value)} /></label>
          <label className="field"><span>Адрес в Ростове</span><input className="input" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="ул. Большая Садовая, 1" /></label>
          <label className="field"><span>Телефон</span><input className="input" value={phone} onChange={(e) => setPhone(e.target.value)} /></label>
          <div className="field"><span>Направления</span>
            <div className="chips chips--wrap">
              {allSports.map((s) => (
                <button key={s.id} className={`chip chip--orange${sports.includes(s.id) ? ' chip--active' : ''}`}
                  onClick={() => setSports((prev) => (prev.includes(s.id) ? prev.filter((x) => x !== s.id) : [...prev, s.id]))}>
                  {s.title}
                </button>
              ))}
            </div>
          </div>
          <Btn size="lg" block loading={busy} disabled={!name.trim() || !address.trim() || sports.length === 0}
            onClick={() => run(() => api.platformCreate({ name, address, phone, sports }))}>
            Добавить студию
          </Btn>
        </>
      )}
    </section>
  )
}

// Панель команды СпортСлот: подключение студий и приглашения администраторов.
export function PlatformScreen() {
  const [items, setItems] = useState<PlatformStudio[] | null>(null)
  const [error, setError] = useState('')
  const [invite, setInvite] = useState<Invite | null>(null)
  const [adding, setAdding] = useState(false)
  const nav = useNav()

  const load = () => {
    setError('')
    api.platformStudios().then((r) => setItems(r.studios)).catch((e) => setError(e.message))
  }
  useEffect(load, [])

  async function makeInvite(v: Venue) {
    try {
      const r = await api.platformInvite(v.id)
      setInvite({ venue: v, link: r.link, expires_at: r.expires_at })
      load()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось создать приглашение')
    }
  }

  return (
    <div className="screen">
      <header className="hero">
        <h1 className="h1">Студии-партнёры</h1>
        <p className="muted">Подключите студию к онлайн-записи и отправьте администратору ссылку-приглашение.</p>
      </header>
      {adding ? (
        <Connect onDone={(v) => { setAdding(false); load(); void makeInvite(v) }} />
      ) : (
        <Btn size="lg" block icon={<Plus size={18} />} onClick={() => setAdding(true)}>Подключить студию</Btn>
      )}
      {error && <ErrorBlock text={error} onRetry={load} />}
      {!error && !items && <Loader />}
      {items && items.length === 0 && <Empty title="Студий пока нет" />}
      {items?.map((it) => <StudioRow key={it.venue.id} item={it} onInvite={makeInvite} onChanged={load} />)}
      {invite && <InviteSheet invite={invite} onClose={() => setInvite(null)} />}
    </div>
  )
}
