import ReactDOM from 'react-dom/client'
import { i18nReady } from './i18n/config'
import { App } from './app'
import './styles/index.css'

void i18nReady.then(() => {
  ReactDOM.createRoot(document.getElementById('root')!).render(<App />)
})
