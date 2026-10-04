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

export function useBedrockLabels() {
  const { t } = useTranslation()
  const codes: Record<string, string> = {
    access: t('Check IAM permissions, model access and organization policies.'),
    credentials: t(
      'Check the AWS key, temporary session token and request signature.'
    ),
    clock: t(
      'Synchronize the request signer clock and refresh expired credentials.'
    ),
    use_case: t('Submit the Anthropic model use-case form in AWS.'),
    marketplace: t(
      'Check the AWS Marketplace subscription and payment status; pending subscriptions may require waiting.'
    ),
    inference_profile: t(
      'Use a supported inference profile ID or ARN instead of an on-demand base model ID.'
    ),
    model_region: t(
      'Verify the model ID, inference profile and AWS region together.'
    ),
    quota: t(
      'The account service quota was exceeded. Check quotas before testing again.'
    ),
    throttling: t(
      'Reduce concurrency and use exponential backoff with jitter; check the account quota.'
    ),
    model_not_ready: t(
      'The model is not ready. Retry later with backoff; this is not a parameter incompatibility.'
    ),
    timeout: t(
      'The model timed out. Check request size, client timeouts and streaming connectivity.'
    ),
    service: t(
      'Temporary service capacity or internal error. Retry later with backoff and respect Retry-After.'
    ),
    model_error: t(
      'The model execution or stream was interrupted. Inspect the original status and request ID.'
    ),
    validation: t(
      'The request was rejected without a specific recognized cause. Inspect the sanitized evidence.'
    ),
    unknown: t(
      'The request was rejected without a specific recognized cause. Inspect the sanitized evidence.'
    ),
    beta: t(
      'Remove the unsupported beta flag and check support for this exact model and endpoint.'
    ),
    sampling: t(
      'Check model-specific sampling rules. Newer models may reject non-default sampling; Sonnet and Haiku 4.5 do not accept temperature with top_p.'
    ),
    thinking_signature: t(
      'Preserve the original thinking block and signature when replaying; do not modify or synthesize them.'
    ),
    thinking: t(
      'Check the thinking mode, token budget and tool-choice compatibility for this model.'
    ),
    server_tools: t(
      'Bedrock does not provide Anthropic server-side web search, web fetch, code execution or advisor tools. Client-executed tools are separate.'
    ),
    input_source: t(
      'Use inline base64 images or documents. URL sources and Anthropic Files API references are not supported by Bedrock.'
    ),
    message_role: t(
      'Check roles for this model and API. A top-level system prompt is supported; mid-conversation system support varies by model.'
    ),
    count_tokens: t(
      'CountTokens support varies by model and endpoint. Some models require the bedrock-mantle count_tokens endpoint; missing counts stay unscored.'
    ),
    document: t(
      'PDF is supported on compatible Bedrock models. Check document encoding and the 100-page request limit; Converse visual PDF requires citations.'
    ),
    image: t(
      'Check image format, dimensions and size against the selected Bedrock API limits.'
    ),
    token_limit: t(
      'Check the exact model context and output limits; the request byte limit is separate from token limits.'
    ),
  }
  return codes
}
