import { useEffect, useState } from 'react'
import { api, type StudioSettings, type Venue } from '../api'
import { Btn, Toggle, VenueLogo } from '../components'
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
      <LogoField venue={venue} readOnly={readOnly} onSaved={onSaved} />
      <label className="field"><span>Телефон для клиентов</span>
        <input className="input" disabled={readOnly} value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+7 863 000-00-00" /></label>
      <label className="field"><span>Сайт или группа</span>
        <input className="input" disabled={readOnly} value={website} onChange={(e) => setWebsite(e.target.value)} placeholder="https://vk.com/..." /></label>
      <label className="field"><span>Пробное занятие, ₽ (0 — бесплатно)</span>
        <input className="input" disabled={readOnly} type="number" min={0} value={price} onChange={(e) => setPrice(e.target.value)} /></label>
      <label className="field"><span>Что взять на первое занятие</span>
        <textarea className="input textarea" disabled={readOnly} value={bring} onChange={(e) => setBring(e.target.value)} rows={2} /></label>
      <label className="field"><span>Коротко о студии — показывается под названием</span>
        <textarea className="input textarea" disabled={readOnly} value={description} maxLength={300} onChange={(e) => setDescription(e.target.value)} rows={3}
          placeholder="Например: «Хатха-йога для начинающих утром и вечером, коврики есть»" />
        <small className="muted">{description.length}/300 · одно-два предложения: что за занятия и для кого</small></label>
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

// Логотип: картинка обрезается до квадрата и уменьшается до 512×512 прямо на телефоне,
// сервер ещё раз проверяет и перекодирует файл.
function LogoField({ venue, readOnly, onSaved }: { venue: Venue; readOnly: boolean; onSaved: () => void }) {
  const nav = useNav()
  const [busy, setBusy] = useState(false)
  const [logo, setLogo] = useState(venue.logo_url)

  async function pick(file: File | undefined) {
    if (!file) return
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      nav.toast('Нужна картинка PNG, JPG или WebP')
      return
    }
    if (file.size > 5 * 1024 * 1024) {
      nav.toast('Файл больше 5 МБ')
      return
    }
    setBusy(true)
    try {
      const blob = await squareLogo(file)
      const v = await api.uploadLogo(venue.id, blob)
      setLogo(v.logo_url)
      nav.toast('Логотип обновлён')
      onSaved()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось загрузить логотип')
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    setBusy(true)
    try {
      await api.deleteLogo(venue.id)
      setLogo(undefined)
      onSaved()
    } catch (e) {
      nav.toast(e instanceof Error ? e.message : 'Не получилось удалить')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="logo-field">
      <VenueLogo venue={{ name: venue.name, logo_url: logo }} size={72} />
      <div className="logo-field__main">
        <b>Логотип</b>
        <small className="muted">Квадратная картинка PNG, JPG или WebP от 256×256 пикселей (лучше 512×512), до 5 МБ. Показывается в круге: важное — в центре, без мелкого текста и рамок. Подойдёт прозрачный или однотонный фон.</small>
        {!readOnly && (
          <div className="row">
            <label className={`btn btn--secondary btn--md${busy ? ' btn--disabled' : ''}`}>
              {busy ? 'Загружаем…' : logo ? 'Заменить' : 'Загрузить'}
              <input type="file" accept="image/png,image/jpeg,image/webp" hidden disabled={busy} onChange={(e) => pick(e.target.files?.[0])} />
            </label>
            {logo && <Btn variant="ghost" disabled={busy} onClick={remove}>Удалить</Btn>}
          </div>
        )}
      </div>
    </div>
  )
}

// squareLogo — центральный квадрат, не больше 512×512, PNG (с прозрачностью) или JPEG.
async function squareLogo(file: File): Promise<Blob> {
  const url = URL.createObjectURL(file)
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => reject(new Error('Не получилось открыть картинку'))
      el.src = url
    })
    const side = Math.min(img.naturalWidth, img.naturalHeight)
    if (side < 256) throw new Error('Картинка меньше 256×256 пикселей — логотип будет размытым')
    const out = Math.min(512, side)
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = out
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('Браузер не поддерживает обработку картинок')
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(img, (img.naturalWidth - side) / 2, (img.naturalHeight - side) / 2, side, side, 0, 0, out, out)
    const type = file.type === 'image/jpeg' ? 'image/jpeg' : 'image/png'
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('Не получилось подготовить картинку'))), type, 0.9),
    )
  } finally {
    URL.revokeObjectURL(url)
  }
}
