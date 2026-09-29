import { createRoot } from 'react-dom/client'
import { MaxUI } from '@maxhub/max-ui'
import '@maxhub/max-ui/dist/styles.css'
import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/montserrat/600.css'
import '@fontsource/montserrat/700.css'
import '@fontsource/montserrat/800.css'
import App from './App'
import './styles.css'
import { initBridge } from './max'

initBridge()

createRoot(document.getElementById('root')!).render(
  <MaxUI colorScheme="light">
    <App />
  </MaxUI>,
)
