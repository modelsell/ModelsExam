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
import { useTranslation } from 'react-i18next'

// Same questions as the server-rendered FAQ (internal/seo/content.go), so what
// a crawler reads is what a visitor reads.
export const FAQ_KEYS = [
  ['What is ModelsExam?', 'ModelsExam is an independent, open-source (AGPL-3.0) checker. It sends a fixed set of requests to the Claude, OpenAI-compatible or image API you point it at, and checks whether the model behaves as claimed, the protocol matches the official one and token usage is honest, then writes a report that only your browser can see.'],
  ['How can I tell if a relay really serves the model it claims?', 'Asking a model who it is is not reliable. ModelsExam combines identity answers, behavior fingerprints and capability probes, compares against official baselines, and checks fields, streaming, tool calls, error formats and token usage against the official protocol. A cheaper model passed off as a premium one tends to show up there.'],
  ['Do I have to enter an API key? Is it exposed?', 'Yes. The key is used only for this check and is not written to the report. Check records are visible only to the browser that ran them. Use a key created just for testing with a small quota, and disable it afterwards.'],
  ['Does a score of 100 mean it is safe to buy?', 'No. The score counts scored assertions and describes one address at one moment. It is evidence, not a guarantee, a ranking or an endorsement. A badge expires after 30 days; go by the check date.'],
  ['Can other people see my check records?', 'No. A check record is tied to the browser that ran it: only that browser can list and open it under My check records. The site publishes nobody’s records and builds no boards. If you clear cookies or switch browsers, earlier records can no longer be opened.'],
  ['I run a site. How do I show the badge?', 'Enter your domain on the Get your badge page and copy the script, image or Markdown code. The badge appears only on your own domain, shows the latest check of an endpoint on that domain, expires after 30 days. It shows only the verdict, not the report.'],
  ['Does a check cost money?', 'A check sends about twenty requests to the upstream you enter, which may cost a little with that provider. Each request waits at most 90 seconds and a whole check at most 10 minutes.'],
] as const

export function FaqSection() {
  const { t } = useTranslation()
  return (
    <section aria-labelledby='faq-title' id='faq' className='scroll-mt-20'>
      <h2 id='faq-title' className='mb-6 text-2xl font-extrabold tracking-tight sm:text-3xl'>
        {t('Frequently asked questions')}
      </h2>
      <div className='flex flex-col divide-y rounded-lg border'>
        {FAQ_KEYS.map(([q, a]) => (
          <details key={q} className='group px-4 py-3'>
            <summary className='cursor-pointer list-none font-medium marker:hidden'>{t(q)}</summary>
            <p className='text-muted-foreground mt-2 text-sm leading-6'>{t(a)}</p>
          </details>
        ))}
      </div>
    </section>
  )
}
