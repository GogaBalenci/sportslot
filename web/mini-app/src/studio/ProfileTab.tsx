import { useEffect, useState } from 'react'
import { api, type StudioSettings, type Venue } from '../api'
import { Btn, Toggle } from '../components'
import { useNav } from '../nav'

// Профиль студии — то, что видят клиенты в карточке и в сообщениях об отмене.
export function ProfileTab({ venue, readOnly, onSaved }: { venue: Venue; readOnly: boolean; onSaved: () => void }) {
  const nav = useNav()
  const [phone, setPhone] = useState(venue.phone ?? '')
  const [website, setWebsite] = useState(venue.website ?? '')
  const [bring, setBring] = useState(venue.what_to_bring ?? '')
  const [price, setPrice] = useState(venue.trial_price === undefined || venue.trial_price === null ? '' : String(venue.trial_price))
  const [description, setDescription] = useState(venue.description ?? '')
  const [busy, setBusy] = useState(false)
  const [settings, setSettings] = useState<StudioSettings | null>(null)
  useEffect(() => {
    api.studioSettings(venue.id).then(setSettings).catch(() => setSettings(null))
  }, [venue.id])

  async function toggle(key: 'show_reviews' | 'daily_digest', value: boolean) {
    try {
      setSettings(await api.saveStudioSettings(venue.id, { [key]: value }))
      nav.toast(key === 'show_reviews' ? (value ? 'Оценки и отзывы видны в карточке' : 'Отзывы скрыты из карточки') : value ? 'Сводка будет приходить в 9:00' : 'Сводка выключена')
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось сохранить')
    }
  }

  async function save() {
    setBusy(true)
    try {
      await api.studioProfile(venue.id, {
        phone, website, what_to_bring: bring, description, trial_price: price === '' ? null : Number(price),
      })
      nav.toast('Профиль сохранён')
      onSaved()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось сохранить')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="info-card">
      {readOnly && <div className="note">Демо-студии в демо-доступе только просматриваются.</div>}
      <label className="field"><span>Телефон для клиентов</span>
        <input className="input" disabled={readOnly} value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+7 863 000-00-00" /></label>
      <label className="field"><span>Сайт или группа</span>
        <input className="input" disabled={readOnly} value={website} onChange={(e) => setWebsite(e.target.value)} placeholder="https://vk.com/..." /></label>
      <label className="field"><span>Пробное занятие, ₽ (0 — бесплатно)</span>
        <input className="input" disabled={readOnly} type="number" min={0} value={price} onChange={(e) => setPrice(e.target.value)} /></label>
      <label className="field"><span>Что взять на первое занятие</span>
        <textarea className="input textarea" disabled={readOnly} value={bring} onChange={(e) => setBring(e.target.value)} rows={2} /></label>
      <label className="field"><span>О студии</span>
        <textarea className="input textarea" disabled={readOnly} value={description} onChange={(e) => setDescription(e.target.value)} rows={3} /></label>
      {!readOnly && <Btn size="lg" block loading={busy} onClick={save}>Сохранить</Btn>}
      {settings && (
        <>
          <Toggle checked={settings.show_reviews} disabled={readOnly} onChange={(v) => toggle('show_reviews', v)}
            label="Показывать оценки и отзывы" hint="Средняя оценка и последние отзывы клиентов появятся в карточке студии и в поиске" />
          <Toggle checked={settings.daily_digest} disabled={readOnly} onChange={(v) => toggle('daily_digest', v)}
            label="Утренняя сводка в 9:00" hint="Записи на сегодня, сообщения без ответа, заявки и оплаты — одним сообщением в бот" />
        </>
      )}
    </section>
  )
}
