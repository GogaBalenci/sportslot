/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
  readonly VITE_DEV_MAX_USER_ID?: string
}

interface MaxWebAppUser {
  id: number
  first_name?: string
  last_name?: string
}

interface MaxWebApp {
  initData?: string
  initDataUnsafe?: { user?: MaxWebAppUser; start_param?: string }
  platform?: 'ios' | 'android' | 'desktop' | 'web'
  colorScheme?: 'light' | 'dark'
  ready?: () => void
  expand?: () => void
  onEvent?: (event: string, handler: () => void) => void
  openLink?: (url: string) => void
  openMaxLink?: (url: string) => void
  openCodeReader?: (fileSelect?: boolean) => Promise<string | { value?: string; text?: string }>
  requestScreenMaxBrightness?: () => Promise<unknown>
  restoreScreenBrightness?: () => Promise<unknown>
  BackButton?: {
    show: () => void
    hide: () => void
    onClick: (cb: () => void) => void
    offClick: (cb: () => void) => void
  }
  HapticFeedback?: {
    impactOccurred: (style: 'light' | 'medium' | 'heavy' | 'rigid' | 'soft') => void
    notificationOccurred: (type: 'error' | 'success' | 'warning') => void
    selectionChanged: () => void
  }
}

interface Window {
  WebApp?: MaxWebApp
}
