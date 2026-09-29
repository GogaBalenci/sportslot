import { createRoot } from 'react-dom/client'
import { MaxUI } from '@maxhub/max-ui'
import '@maxhub/max-ui/dist/styles.css'
import App from './App'
import './styles.css'
import { initBridge } from './max'

initBridge()

createRoot(document.getElementById('root')!).render(
  <MaxUI>
    <App />
  </MaxUI>,
)
