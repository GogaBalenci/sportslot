import { createRoot } from 'react-dom/client'
import { MaxUI } from '@maxhub/max-ui'
import '@maxhub/max-ui/dist/styles.css'
import App from './App'
import './styles.css'
import { initialiseMaxBridge } from './max'

initialiseMaxBridge()

createRoot(document.getElementById('root')!).render(
  <MaxUI>
    <App />
  </MaxUI>,
)
