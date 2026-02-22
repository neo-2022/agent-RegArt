/**
 * Точка входа React-приложения (web-ui).
 *
 * Рендерит корневой компонент App внутри:
 *   - StrictMode — включает дополнительные проверки React в режиме разработки
 *   - ErrorBoundary — перехватывает ошибки рендеринга и показывает экран ошибки
 *
 * Монтируется в DOM-элемент с id="root" (определён в index.html).
 */
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import ErrorBoundary from './components/ErrorBoundary.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
