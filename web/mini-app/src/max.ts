export function initialiseMaxBridge(): void {
  window.WebApp?.ready?.()
  window.WebApp?.expand?.()
  applyMaxTheme()

  // Слушаем смену темы в мессенджере MAX
  window.WebApp?.onEvent?.('themeChanged', applyMaxTheme)
}

export function applyMaxTheme(): void {
  if (typeof document === 'undefined') return
  const colorScheme = window.WebApp?.colorScheme
  if (colorScheme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

export function triggerHapticNotification(type: 'success' | 'error' | 'warning'): void {
  try {
    window.WebApp?.HapticFeedback?.notificationOccurred(type)
  } catch {
    // Graceful fallback вне платформы MAX
  }
}

export function triggerHapticImpact(style: 'light' | 'medium' | 'heavy' = 'light'): void {
  try {
    window.WebApp?.HapticFeedback?.impactOccurred(style)
  } catch {
    // Graceful fallback вне платформы MAX
  }
}

export function maxUserID(): string {
  const id = window.WebApp?.initDataUnsafe?.user?.id
  if (id) {
    return String(id)
  }
  if (typeof window !== 'undefined' && window.location?.search) {
    const param = new URLSearchParams(window.location.search).get('user')
    if (param) {
      return param
    }
  }
  // Используется только при открытии приложения вне MAX, чтобы локально
  // воспроизвести API-сценарий с тестовыми данными.
  return import.meta.env.VITE_DEMO_MAX_USER_ID || 'max-test-user-001'
}

export function maxUserName(): string | undefined {
  const user = window.WebApp?.initDataUnsafe?.user
  return user ? [user.first_name, user.last_name].filter(Boolean).join(' ') : undefined
}
