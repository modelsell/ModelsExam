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
import type { Route } from './route'
import { routePath } from './route'

export const SITE_NAME = 'ModelsExam'

export type Seo = { title: string; description: string; noindex: boolean }

type Translate = (key: string) => string

// Per-page title and description. Report pages and unknown paths are never
// indexed: reports are user-generated and change while they run.
export function seoFor(route: Route, t: Translate): Seo {
  switch (route.name) {
    case 'home':
      return {
        title: `${SITE_NAME}: ${t('independent exams for model APIs')}`,
        description: t(
          'Open-source, independent conformance and authenticity tests for Claude, OpenAI-compatible and image model APIs. Run an exam on any endpoint and read the evidence.'
        ),
        noindex: false,
      }
    case 'baselines':
      return {
        title: `${t('Official baselines')} | ${SITE_NAME}`,
        description: t(
          'Reference exams run directly on the official Anthropic, AWS Bedrock and OpenAI endpoints. Compare any relay against the source.'
        ),
        noindex: false,
      }
    case 'records':
      return {
        title: `${t('My check records')} | ${SITE_NAME}`,
        description: t(
          'Check records run from this browser. They are private and not shown to anyone else.'
        ),
        noindex: true,
      }
    case 'method':
      return {
        title: `${t('Method and independence')} | ${SITE_NAME}`,
        description: t(
          'What ModelsExam checks, how scores are counted and why results are evidence about one run, not a ranking or certification.'
        ),
        noindex: false,
      }
    case 'integrate':
      return {
        title: `${t('Relay integration')} | ${SITE_NAME}`,
        description: t(
          'Link your users to ModelsExam with the Base URL, key and model already filled in. They review the form and start the check themselves.'
        ),
        noindex: false,
      }
    case 'getbadge':
      return {
        title: `${t('Get your badge')} | ${SITE_NAME}`,
        description: t(
          'Enter your domain to get a ModelsExam badge for your website: a script or image that shows your latest check result, only on your own domain.'
        ),
        noindex: false,
      }
    case 'site':
      return {
        title: `${t('Site badge')} | ${SITE_NAME}`,
        description: t('The latest ModelsExam check result for a website.'),
        noindex: true,
      }
    case 'report':
      return {
        title: `${t('Check report')} | ${SITE_NAME}`,
        description: t('A single ModelsExam check report.'),
        noindex: true,
      }
    default:
      return {
        title: `${t('Page not found')} | ${SITE_NAME}`,
        description: t('This page does not exist.'),
        noindex: true,
      }
  }
}

function upsert(selector: string, make: () => HTMLElement): HTMLElement {
  let el = document.head.querySelector<HTMLElement>(selector)
  if (!el) {
    el = make()
    document.head.appendChild(el)
  }
  return el
}

function setMeta(attr: 'name' | 'property', key: string, content: string) {
  const el = upsert(`meta[${attr}="${key}"]`, () => {
    const m = document.createElement('meta')
    m.setAttribute(attr, key)
    return m
  })
  el.setAttribute('content', content)
}

// i18next codes are zhCN/zhTW; the lang attribute needs BCP-47.
function htmlLang(code: string): string {
  return code === 'zhCN' ? 'zh-CN' : code === 'zhTW' ? 'zh-TW' : code || 'zh-CN'
}

// Keeps the head in step with the page while the app navigates client-side.
// The server renders the same tags for the first load (internal/seo).
export function applySeo(route: Route, seo: Seo, lang: string) {
  const url = `${window.location.origin}${routePath(route)}`
  document.title = seo.title
  document.documentElement.lang = htmlLang(lang)
  setMeta('name', 'description', seo.description)
  setMeta('name', 'robots', seo.noindex ? 'noindex, nofollow' : 'index, follow')
  setMeta('property', 'og:site_name', SITE_NAME)
  setMeta('property', 'og:type', 'website')
  setMeta('property', 'og:title', seo.title)
  setMeta('property', 'og:description', seo.description)
  setMeta('name', 'twitter:card', 'summary_large_image')
  setMeta('property', 'og:image', `${window.location.origin}/og-image.png`)
  setMeta('name', 'twitter:image', `${window.location.origin}/og-image.png`)
  const canonical = upsert('link[rel="canonical"]', () => {
    const l = document.createElement('link')
    l.rel = 'canonical'
    return l
  })
  if (seo.noindex) canonical.remove()
  else {
    canonical.setAttribute('href', url)
    setMeta('property', 'og:url', url)
  }
}
