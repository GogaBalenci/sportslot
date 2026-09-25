/// <reference types="vite/client" />

interface MaxWebAppUser {
  id: number | string
  first_name?: string
  last_name?: string
}

interface MaxThemeParams {
  bg_color?: string
  text_color?: string
  hint_color?: string
  link_color?: string
  button_color?: string
  button_text_color?: string
  secondary_bg_color?: string
}

interface MaxHapticFeedback {
  impactOccurred: (style: 'light' | 'medium' | 'heavy' | 'rigid' | 'soft') => void
  notificationOccurred: (type: 'error' | 'success' | 'warning') => void
  selectionChanged: () => void
}

interface MaxWebApp {
  initDataUnsafe?: { user?: MaxWebAppUser }
  colorScheme?: 'light' | 'dark'
  themeParams?: MaxThemeParams
  HapticFeedback?: MaxHapticFeedback
  ready?: () => void
  expand?: () => void
  onEvent?: (eventType: string, eventHandler: () => void) => void
  offEvent?: (eventType: string, eventHandler: () => void) => void
}

interface Window {
  WebApp?: MaxWebApp
}
