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

export function useCapabilityLabels() {
  const { t } = useTranslation()
  const groups: Record<string, string> = {
    reasoning: t('Reasoning'),
    code: t('Code'),
    instruction: t('Instruction following'),
    structured: t('Structured output'),
    context: t('Context'),
    tools: t('Tool use'),
  }
  const graders: Record<string, string> = {
    'exact-v1': t('Exact reference answer'),
    'line-constraints-v1': t('Line count, order and format constraints'),
    'json-order-v1': t('JSON structure, field values and types'),
    'json-null-types-v1': t('JSON structure, field values and types'),
    'context-join-v1': t('Cross-record retrieval and JSON fields'),
    'tool-selection-v1': t('Tool name, arguments and stop reason'),
    'tool-roundtrip-v1': t('Tool call and answer grounded in its result'),
  }
  const codes: Record<string, string> = {
    pending: t('Waiting to run'),
    cancelled: t('Check stopped'),
    endpoint_not_found: t('Endpoint unavailable; no capability score'),
    route_unavailable: t('Endpoint unavailable; no capability score'),
    upstream_unavailable: t('Upstream unavailable; no capability score'),
    transport_error: t('Connection error; no capability score'),
    not_collected: t('Not collected'),
    scored: t('Scored'),
    request_rejected: t('Request rejected; no capability score'),
    access_denied: t('Access denied; no capability score'),
    rate_limited: t('Rate limited; no capability score'),
    request_refused: t('Request refused; no capability score'),
    probe_timeout: t('Request timed out; no capability score'),
    invalid_response: t('Invalid response; no capability score'),
    response_truncated: t('Truncated response; no capability score'),
    request_profile_changed: t(
      'Request parameters changed; no capability score'
    ),
    unexpected_stop: t('Unexpected stop reason; no capability score'),
  }
  const assertions: Record<string, string> = {
    exact_answer: t('Exact reference answer'),
    three_lines: t('Three lines'),
    report_heading: t('Required heading'),
    sorted_values: t('Sorted values'),
    end_marker: t('Required end marker'),
    json_object: t('Valid JSON object'),
    exact_fields: t('Exact field set'),
    one_tool_call: t('One tool call'),
    tool_name: t('Correct tool name'),
    tool_arguments: t('Correct tool arguments'),
    tool_stop: t('Tool stop reason'),
    correct_tool_call: t('Correct tool call'),
    final_answer: t('Completed final answer'),
  }
  return { groups, graders, codes, assertions }
}
