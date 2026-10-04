import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'
import { convertDetectedLanguage } from './languages'
import zhCN from './locales/zh.json'

// Chinese is the default and ships in the main bundle: the server-rendered HTML
// that crawlers read is Chinese, so the app must start in Chinese too. Other
// locales are separate chunks, loaded only when someone picked them.
type Locale = { translation: Record<string, string> }
const loaders: Record<string, () => Promise<{ default: Locale }>> = {
  en: () => import('./locales/en.json'),
  fr: () => import('./locales/fr.json'),
  ja: () => import('./locales/ja.json'),
  ru: () => import('./locales/ru.json'),
  vi: () => import('./locales/vi.json'),
  zhTW: () => import('./locales/zh-TW.json'),
}
export const DEFAULT_LANGUAGE = 'zhCN'
const STORAGE_KEY = 'i18nextLng'

function savedLanguage(): string {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    const code = v ? convertDetectedLanguage(v) : ''
    return code in loaders ? code : DEFAULT_LANGUAGE
  } catch {
    return DEFAULT_LANGUAGE
  }
}

async function bundle(code: string): Promise<Locale> {
  return (await loaders[code]()).default
}

// Resolves once the starting language is loaded; render after it.
export const i18nReady: Promise<void> = (async () => {
  const lng = savedLanguage()
  const resources: Record<string, Locale> = { [DEFAULT_LANGUAGE]: zhCN as Locale }
  if (lng !== DEFAULT_LANGUAGE) resources[lng] = await bundle(lng)
  await i18n
    .use(new LanguageDetector())
    .use(initReactI18next)
    .init({
      resources,
      lng,
      fallbackLng: false, // keys are the English sentences, so a missing key shows English
      supportedLngs: [DEFAULT_LANGUAGE, ...Object.keys(loaders)],
      load: 'currentOnly',
      nsSeparator: false, // keys are literal sentences and may contain colons
      interpolation: { escapeValue: false },
      detection: { order: ['localStorage'], caches: ['localStorage'], convertDetectedLanguage },
    })
})()

// Loads the language's strings, then switches to it.
export async function setLanguage(code: string): Promise<void> {
  if (code in loaders && !i18n.hasResourceBundle(code, 'translation')) {
    i18n.addResourceBundle(code, 'translation', (await bundle(code)).translation)
  }
  await i18n.changeLanguage(code)
}

export default i18n
