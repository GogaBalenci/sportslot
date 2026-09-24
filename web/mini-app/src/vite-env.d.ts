/// <reference types="vite/client" />

interface MaxWebAppUser {
  id: number | string
  first_name?: string
  last_name?: string
}

interface MaxWebApp {
  initDataUnsafe?: { user?: MaxWebAppUser }
  ready?: () => void
  expand?: () => void
}

interface Window {
  WebApp?: MaxWebApp
}
