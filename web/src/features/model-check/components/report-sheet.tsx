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
import { useTokenAuditLabels } from '../hooks/use-token-audit-labels'
import { useClaudeCheckLabels } from '../labels'
import { selectedReportBaseline } from '../lib/baseline-selection'
import { SCORED_CHECK_IDS } from '../lib/check-plan'
import { presentReport } from '../lib/report-presentation'
import { reportRemark } from '../lib/report-remark'
import { checkScore } from '../lib/report-score'
import { sheetAssessment } from '../lib/report-sheet'
import { REPORT_SHEET_CSS } from '../lib/report-sheet-style'
import { sourceOfEndpoint } from '../lib/claude-source'
import type { ClaudeCheckReport } from '../types'
import { BedrockDiagnostics } from './bedrock-diagnostics'
import { CacheAssessmentDetails } from './cache-assessment-details'
import { CapabilityAssessment } from './capability-assessment'
import { PromptIntegrityDetails } from './prompt-integrity-details'
import { ReportRadar, ReportRing } from './report-sheet-charts'
import { ReportSheetLedger } from './report-sheet-ledger'
import { useSourceLabels } from './source-badge'
import { UsageTokenIntegrity } from './usage-token-integrity'

export function ReportSheet(props: {
  report: ClaudeCheckReport
  running: boolean
  phaseLabel: string
  durationMS?: number
  activeProbe?: string | null
}) {
  const { t } = useTranslation()
  const sourceLabels = useSourceLabels()
  const labels = useClaudeCheckLabels()
  const auditLabels = useTokenAuditLabels(props.report.token_audit?.version)
  const report = presentReport(props.report),
    data = sheetAssessment(report, props.running)
  const baseline = selectedReportBaseline(report)
  const comparison = baseline
    ? data.comparisons.find((item) => item.baseline.id === baseline.id)
    : undefined
  let baselineSummary = t('No baseline selected for this run.')
  if (report.options?.baseline_id && !baseline)
    baselineSummary = t('Selected baseline snapshot unavailable')
  if (!report.options?.baseline_id && report.baselines?.length)
    baselineSummary = t('Historical comparison snapshots: {{count}}', {
      count: report.baselines.length,
    })
  if (baseline)
    baselineSummary = `${t('Baseline: {{type}} · {{model}} · {{channel}}', {
      type: baseline.type,
      model: baseline.model,
      channel: baseline.channel_name || '—',
    })} · ${new Date(baseline.created_at).toLocaleString()}`
  let promptTitle = t('Prompt injection token check')
  if ((report.token_audit?.version ?? 0) >= 2)
    promptTitle = t('Prompt integrity evidence')
  if (report.token_audit?.prompt_assessment?.scoring === 'measured_checks')
    promptTitle = t('Prompt consistency score')
  if (report.token_audit?.prompt_assessment?.scoring === 'input_consistency')
    promptTitle = t('Input consistency score')
  if (report.token_audit?.prompt_assessment?.scoring === 'input_budget')
    promptTitle = t('Prompt injection score')
  const points = (value: number | null | undefined) =>
    value == null ? '—' : `${Math.round(value)}/100`
  const number = (value: number | null | undefined, digits = 0) =>
    value == null ? '—' : value.toFixed(digits)
  const dimensionNames: Record<string, string> = {
    model: t('Model fields'),
    capability: t('Capability'),
    prompt: t('Prompt'),
    cache: t('Cache'),
    protocol: t('Protocol'),
  }
  const identity = {
    mismatch: t('Declared model mismatch'),
    consistent: t('Declared model IDs match'),
    unresolved: t('Insufficient identity evidence'),
    pending: t('Collecting model evidence'),
    legacy: t('Not checked in this historical run'),
  }
  const dimensions = data.dimensions.map((d) => ({
    ...d,
    label: dimensionNames[d.id],
  }))
  const scoredPlan = data.selected.filter((item) =>
    SCORED_CHECK_IDS.has(item.id)
  )
  const scoredExecuted = scoredPlan.filter((item) =>
    report.checks.some(
      (check) => check.id === item.id && check.status !== 'skipped'
    )
  ).length
  const sections = [{ title: t('Scored checks'), rows: scoredPlan }]
  const resultScore = (id: string, kind: string) => {
    if (id === 'capability_benchmark')
      return data.dimensions.find((d) => d.id === 'capability')?.score ?? null
    if (id === 'usage_token_integrity') return data.usageTokens.score
    if (id === 'cache_token_audit') return data.cache.score
    if (id === 'prompt_integrity') return data.prompt.score
    if (id === 'performance_sampling')
      return comparison?.performance.score ?? null
    if (kind === 'boundary') return null
    return checkScore(report.checks.find((c) => c.id === id))
  }
  const showPrompt =
    !!report.options?.prompt_audit || !!report.token_audit?.prompt?.length
  const showCache =
    !!report.options?.cache || !!report.token_audit?.cache?.length
  const pending = props.running ? t('Waiting to run') : t('Not collected')
  return (
    <article className='mc-report' aria-label={t('Model check report')}>
      <style>{REPORT_SHEET_CSS}</style>
      <header className='mc-title'>
        <div>
          <small>MODELSEXAM / v{report.version}</small>
          <h2>{t('Model check report')}</h2>
          <p>
            {reportRemark(report) || '—'} · {report.model}
          </p>
        </div>
        <div className='mc-status'>
          <p>{props.phaseLabel}</p>
          <small>{new Date(report.started_at).toLocaleString()}</small>
          <small>
            {sourceLabels[sourceOfEndpoint(report.endpoint, report.transport)]}
            {' · '}
            {report.transport === 'bedrock_runtime'
              ? 'AWS Bedrock Runtime'
              : 'Anthropic Messages'}
          </small>
        </div>
      </header>
      <p className='mc-note'>{baselineSummary}</p>
      {props.running && (
        <p className='mc-provisional'>
          {t('Live results are provisional until collection finishes.')}
        </p>
      )}
      {(report.cancelled || report.stop_reason) && (
        <p className='mc-provisional'>
          {t('Stopped before all checks ran')} ·{' '}
          {labels.codes[report.stop_reason || 'cancelled'] ||
            report.stop_reason}
        </p>
      )}
      <div className='mc-hero'>
        <div className='mc-score'>
          <ReportRing
            score={data.score}
            label={`${t('Measured dimension average')}: ${points(data.score)}`}
          />
          <strong>{t('Measured dimension average')}</strong>
          <p className='mc-muted'>
            {t('{{count}} / 5 dimensions scored', { count: data.measured })}
          </p>
        </div>
        <ReportRadar
          dimensions={dimensions}
          title={t('Measured dimensions')}
          partialLabel={t(
            'Only scored vertices are connected. Dashed edges span unscored dimensions.'
          )}
        />
        <div className='mc-findings'>
          <span
            className={`mc-chip ${data.identity === 'mismatch' ? 'mc-review' : ''}`}
          >
            {identity[data.identity]}
          </span>
          <p className='mc-muted'>
            {t(
              'Declared names and capability answers provide clues. They cannot certify model authenticity.'
            )}
          </p>
          <p>
            {t('Evidence coverage')}: {scoredExecuted}/{scoredPlan.length}
          </p>
          <p className='mc-muted'>
            {t(
              'Equal-weight average of measured dimensions; unavailable dimensions are excluded.'
            )}
          </p>
        </div>
      </div>
      <div className='mc-dimension-list'>
        {dimensions.map((d) => (
          <span className='mc-chip' key={d.id}>
            {d.label} {points(d.score)}
          </span>
        ))}
      </div>
      <div className='mc-metrics'>
        <div className='mc-metric'>
          <span className='mc-muted'>{t('Median TTFT')}</span>
          <strong>
            {number(
              data.timing.median == null ? null : data.timing.median / 1000,
              2
            )}{' '}
            s
          </strong>
          <span className='mc-muted'>
            {t('Sample standard deviation')}: {number(data.timing.deviation)} ms
            · n={data.timing.count}
          </span>
        </div>
        <div className='mc-metric'>
          <span className='mc-muted'>{t('End-to-end output rate')}</span>
          <strong>{number(data.performance.rate, 1)}</strong>
          <span className='mc-muted'>tok/s · n={data.performance.count}</span>
        </div>
        <div className='mc-metric'>
          <span className='mc-muted'>{t('Cache read hit rate')}</span>
          <strong>{number(data.warm.rate)}%</strong>
          <span className='mc-muted'>
            {data.warm.hits}/{data.warm.measured} · {t('Measured reads')}{' '}
            {data.warm.measured}/{data.warm.attempts}
          </span>
        </div>
      </div>
      <p className='mc-muted'>
        {t(
          'Timing uses qualified performance requests only. First event is not TTFB; missing timings remain unavailable.'
        )}{' '}
        {t('First event')}: {number(data.firstEvent.median)} ms · n=
        {data.firstEvent.count}
      </p>
      <section>
        <div className='mc-section-title'>
          <h3>{t('Observed model route')}</h3>
          <span className='mc-muted'>{t('Clues only')}</span>
        </div>
        <div className='mc-flow'>
          <div>
            <small>{t('Requested model')}</small>
            <code>{report.model}</code>
          </div>
          <div>
            <small>{t('Mapped upstream model')}</small>
            <code>{data.upstreamModels.join(' / ') || '—'}</code>
          </div>
          <div>
            <small>{t('Returned model')}</small>
            <code>{data.returnedModels.join(' / ') || '—'}</code>
          </div>
        </div>
        <p className='mc-note'>
          {data.evidence.join(' · ') || t('No response-header clues observed')}.{' '}
          {t('Headers and model names can be rewritten by gateways.')}
        </p>
      </section>
      {(showPrompt || showCache) && (
        <section>
          <div className='mc-section-title'>
            <h3>{t('Prompt and cache token audit')}</h3>
            <span className='mc-muted'>{t('Counter consistency')}</span>
          </div>
          <div className='mc-audit-grid'>
            {[
              {
                selected: showPrompt,
                title: promptTitle,
                cacheAssessment: undefined,
                assessment: data.prompt.assessment,
                injection: report.token_audit?.injection,
                value: data.prompt,
                items: report.token_audit?.prompt ?? [],
              },
              {
                selected: showCache,
                title: t('Cache write/read token consistency'),
                cacheAssessment: report.token_audit?.cache_assessment,
                assessment: undefined,
                injection: undefined,
                value: data.cache,
                items: report.token_audit?.cache ?? [],
              },
            ]
              .filter((group) => group.selected)
              .map((group) => (
                <div className='mc-audit-box' key={group.title}>
                  <h3>{group.title}</h3>
                  <strong>{points(group.value.score)}</strong>
                  {group.assessment && (
                    <PromptIntegrityDetails
                      assessment={group.assessment}
                      injection={group.injection}
                      samples={group.items}
                      paper
                    />
                  )}
                  {group.cacheAssessment && (
                    <CacheAssessmentDetails
                      assessment={group.cacheAssessment}
                      paper
                    />
                  )}
                  {!['input_consistency', 'input_budget'].includes(
                    group.assessment?.scoring ?? ''
                  ) && (
                    <p className='mc-muted'>
                      {t(
                        group.injection
                          ? 'Same-endpoint exact matches {{equal}} / {{measured}}; measured {{measured}} / {{total}}'
                          : 'Exact matches {{equal}} / {{measured}}; measured {{measured}} / {{total}}',
                        group.value
                      )}
                    </p>
                  )}
                  {!['input_consistency', 'input_budget'].includes(
                    group.assessment?.scoring ?? ''
                  ) &&
                    group.items.map((item) => (
                      <p className='mc-muted' key={item.id}>
                        {auditLabels.names[item.id] || item.id}:{' '}
                        {auditLabels.codes[item.code] ||
                          labels.codes[item.code] ||
                          item.code}
                        {item.behavior && (
                          <>
                            {' '}
                            · {t('Instruction preservation')}{' '}
                            {points(item.behavior.score)} ·{' '}
                            {auditLabels.codes[item.behavior.code] ||
                              item.behavior.code}
                          </>
                        )}
                      </p>
                    ))}
                </div>
              ))}
          </div>
          <p className='mc-note'>
            {t(
              'Equal token counts are not proof of a clean prompt. Cache hits are separate from write/read equality.'
            )}
          </p>
        </section>
      )}
      <CapabilityAssessment
        benchmark={report.benchmark}
        running={props.running}
      />
      <BedrockDiagnostics
        report={report}
        running={props.running}
        activeProbe={props.activeProbe ?? null}
        paper
      />
      <UsageTokenIntegrity
        report={report}
        running={props.running}
        activeProbe={props.activeProbe}
      />
      <ReportSheetLedger report={report} />
      <div className='mc-results'>
        {sections
          .filter((s) => s.rows.length)
          .map((section) => (
            <section key={section.title}>
              <h3>{section.title}</h3>
              {section.rows.map((item) => {
                const check = report.checks.find((c) => c.id === item.id)
                let description = check
                  ? labels.codes[check.code] || check.code
                  : pending
                if (item.id === 'usage_token_integrity')
                  description = labels.codes[data.usageTokens.code]
                return (
                  <div className='mc-result' key={item.id}>
                    <div>
                      <p>{labels.names[item.id] || item.id}</p>
                      <p className='mc-muted'>{description}</p>
                    </div>
                    <b
                      className={
                        resultScore(item.id, item.kind) === 100
                          ? 'mc-good'
                          : 'mc-review'
                      }
                    >
                      {points(resultScore(item.id, item.kind))}
                    </b>
                  </div>
                )
              })}
            </section>
          ))}
      </div>
      <footer className='mc-foot'>
        {report.id} · {t('Duration')}{' '}
        {((props.durationMS ?? report.duration_ms) / 1000).toFixed(1)} s ·{' '}
        {report.samples.length} {t('Requests')}
        <br />
        {t(
          'Missing values remain unavailable; this report does not independently verify model identity or billing.'
        )}
      </footer>
    </article>
  )
}
