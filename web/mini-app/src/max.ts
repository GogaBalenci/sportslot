export function initialiseMaxBridge(): void {
  window.WebApp?.ready?.()
  window.WebApp?.expand?.()
}

export function maxUserID(): string {
  const id = window.WebApp?.initDataUnsafe?.user?.id
  // Используется только при открытии приложения вне MAX, чтобы локально
  // воспроизвести API-сценарий с тестовыми данными.
  return id ? String(id) : import.meta.env.VITE_DEMO_MAX_USER_ID || 'max-test-user-001'
}

export function maxUserName(): string | undefined {
  const user = window.WebApp?.initDataUnsafe?.user
  return user ? [user.first_name, user.last_name].filter(Boolean).join(' ') : undefined
}
