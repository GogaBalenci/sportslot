// Обёртка над MAX Bridge (https://dev.max.ru/docs/webapps/bridge).
// Вне MAX все методы тихо ничего не делают, и приложение работает в браузере.

const app = (): MaxWebApp | undefined => window.WebApp

export function initBridge(): void {
  app()?.ready?.()
  app()?.expand?.()
}

export function initData(): string {
  return app()?.initData ?? ''
}

export function insideMax(): boolean {
  return initData() !== ''
}

// Параметр запуска: из ссылки https://max.ru/<бот>?startapp=X, кнопки open_app
// или из ?start=X, когда приложение открыто обычной ссылкой.
export function startParam(): string {
  const fromMax = app()?.initDataUnsafe?.start_param
  if (fromMax) return fromMax
  return new URLSearchParams(window.location.search).get('start') ?? ''
}

export function userFirstName(): string {
  return app()?.initDataUnsafe?.user?.first_name ?? ''
}

// Тактильный отклик есть только в мобильных клиентах.
function mobile(): boolean {
  const p = app()?.platform
  return p === 'ios' || p === 'android'
}

export function haptic(type: 'success' | 'error' | 'warning' | 'tap'): void {
  if (!mobile()) return
  try {
    if (type === 'tap') app()?.HapticFeedback?.impactOccurred('light')
    else app()?.HapticFeedback?.notificationOccurred(type)
  } catch {
    /* клиент без поддержки */
  }
}

// Внешние ссылки (маршрут, сайт) открываются через Bridge: метод вызывается
// у объекта WebApp, иначе он теряет контекст и ссылка не открывается.
export function openExternal(url: string): void {
  const webApp = app()
  if (webApp?.openLink && insideMax()) {
    try {
      webApp.openLink(url)
      return
    } catch {
      /* откроем обычным способом */
    }
  }
  window.open(url, '_blank', 'noopener')
}

export function brightenScreen(): void {
  if (!mobile()) return
  app()?.requestScreenMaxBrightness?.().catch(() => undefined)
}

export function restoreBrightness(): void {
  if (!mobile()) return
  app()?.restoreScreenBrightness?.().catch(() => undefined)
}

export function canScanQR(): boolean {
  return typeof app()?.openCodeReader === 'function' && app()?.platform !== 'web'
}

export async function scanQR(): Promise<string> {
  const res = await app()!.openCodeReader!(false)
  if (typeof res === 'string') return res
  return res?.value ?? res?.text ?? ''
}

// Системная кнопка «Назад»: один обработчик на всё приложение.
let backHandler: (() => void) | null = null

export function setBackButton(handler: (() => void) | null): void {
  const button = app()?.BackButton
  if (!button) return
  if (backHandler) button.offClick(backHandler)
  backHandler = handler
  if (handler) {
    button.onClick(handler)
    button.show()
  } else {
    button.hide()
  }
}
