/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Suspense, lazy, useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Toaster } from 'sonner'
import { SiteLayout } from './components/site-layout'
import { applySeo, seoFor } from './lib/seo'
import { useLegacyReportRedirect, useRoute } from './lib/router'
import { HomePage } from './pages/home-page'

const BaselinesPage = lazy(() => import('./pages/baselines-page').then((m) => ({ default: m.BaselinesPage })))
const GetBadgePage = lazy(() => import('./pages/get-badge-page').then((m) => ({ default: m.GetBadgePage })))
const ModelPage = lazy(() => import('./pages/model-page').then((m) => ({ default: m.ModelPage })))
const MethodPage = lazy(() => import('./pages/method-page').then((m) => ({ default: m.MethodPage })))
const ModelsPage = lazy(() => import('./pages/models-page').then((m) => ({ default: m.ModelsPage })))
const NotFoundPage = lazy(() => import('./pages/not-found-page').then((m) => ({ default: m.NotFoundPage })))
const RecordsPage = lazy(() => import('./pages/records-page').then((m) => ({ default: m.RecordsPage })))
const ReportPage = lazy(() => import('./pages/report-page').then((m) => ({ default: m.ReportPage })))
const SitePage = lazy(() => import('./pages/site-page').then((m) => ({ default: m.SitePage })))

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
})

// The server writes the real head (title, canonical, robots, Open Graph) for the
// first load. The app must not rewrite it then: it knows nothing of the page
// number of /records?page=2 or whether a report has anything to index.
let serverRenderedHead =
  typeof document !== 'undefined' && document.querySelector('meta[property="og:image"]') !== null

function Shell() {
  const { t, i18n } = useTranslation()
  const route = useRoute()
  useLegacyReportRedirect(route)
  useEffect(() => {
    if (serverRenderedHead) {
      serverRenderedHead = false
      return
    }
    // Report, site and model pages get their head tags from the server, which
    // knows whether the page has anything to index; the app only keeps the
    // title in step and never overrides the robots tag.
    if (route.name === 'report') return
    if (route.name === 'site' || route.name === 'model') {
      document.title = seoFor(route, t).title
      return
    }
    applySeo(route, seoFor(route, t), i18n.resolvedLanguage ?? 'en')
  }, [route, t, i18n.resolvedLanguage])

  let page
  switch (route.name) {
    case 'home':
      page = <HomePage />
      break
    case 'records':
      page = <RecordsPage />
      break
    case 'report':
      page = <ReportPage id={route.id ?? ''} />
      break
    case 'baselines':
      page = <BaselinesPage />
      break
    case 'getbadge':
      page = <GetBadgePage />
      break
    case 'site':
      page = <SitePage domain={route.id ?? ''} />
      break
    case 'model':
      page = <ModelPage model={route.id ?? ''} />
      break
    case 'models':
      page = <ModelsPage />
      break
    case 'method':
      page = <MethodPage />
      break
    default:
      page = <NotFoundPage />
  }
  return (
    <SiteLayout current={route.name === 'report' ? 'records' : route.name}>
      <Suspense fallback={<div className='min-h-[60svh]' aria-busy='true' />}>{page}</Suspense>
      <Toaster />
    </SiteLayout>
  )
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Shell />
    </QueryClientProvider>
  )
}
