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
// Display metadata for the image checks. Values are i18n keys (English source
// text) rendered through t(); unknown ids and codes fall back to the raw value.

export const IMAGE_STAGES: ReadonlyArray<{ id: string; title: string }> = [
  { id: 'generate', title: 'Image generation basics' },
  { id: 'content', title: 'Image content fidelity' },
  { id: 'params', title: 'Parameters' },
  { id: 'edits', title: 'Image edits' },
  { id: 'provenance', title: 'Provenance (OpenAI Verify)' },
  { id: 'reliability', title: 'Reliability observations' },
]

export const IMAGE_CHECK_TITLES: Record<string, string> = {
  img_generate_basic: 'Generation request',
  img_response_shape: 'Response envelope',
  img_size_exact: 'Exact output size',
  img_error_shape: 'Error response shape',
  img_solid_color: 'Solid color prompt',
  img_split_layout: 'Split layout prompt',
  img_centered_shape: 'Centered shape prompt',
  img_count_shapes: 'Object count prompt',
  img_output_format: 'Output formats',
  img_size_matrix: 'Size matrix',
  img_background: 'Transparent background',
  img_n: 'Multiple images (n)',
  img_quality_levels: 'Quality levels',
  img_stream_partial: 'Partial image streaming',
  img_size_invalid: 'Invalid size handling',
  img_edit_basic: 'Image edit',
  img_edit_mask: 'Mask edit',
  img_prov_control: 'Verify negative control',
  img_prov_c2pa: 'C2PA credential',
  img_prov_synthid: 'SynthID watermark',
  img_prov_model_match: 'Credential model match',
  img_prov_format_matrix: 'Provenance across formats',
  img_prov_baseline: 'Official baseline comparison',
  img_usage_fields: 'Usage fields',
  img_performance: 'Latency',
}

export const IMAGE_CHECK_DETAILS: Record<string, string> = {
  img_generate_basic: 'POST /v1/images/generations returns a decodable image.',
  img_response_shape:
    'b64_json or url entries, created timestamp, valid image bytes.',
  img_size_exact: 'The decoded image has exactly the requested size.',
  img_error_shape: 'A deliberately invalid request returns an OpenAI-style error.',
  img_solid_color:
    'A randomly chosen color is requested; the average pixel color must match.',
  img_split_layout:
    'Two random colors on left and right halves must appear in the right places.',
  img_centered_shape:
    'A dark shape on a light background must sit in the center region.',
  img_count_shapes:
    'A random number of shapes is requested and counted locally by pixel analysis.',
  img_output_format: 'jpeg and webp requests must return that actual format.',
  img_size_matrix: 'Other sizes must come back at exactly that size.',
  img_background: 'A transparent background must carry real alpha.',
  img_n: 'n=2 must return two different images.',
  img_quality_levels: 'Observation: output size per quality level.',
  img_stream_partial:
    'stream=true must emit partial_image events and a completed event.',
  img_size_invalid: 'Observation: how an invalid size is handled.',
  img_edit_basic: 'POST /v1/images/edits with an uploaded image.',
  img_edit_mask:
    'Observation: whether the masked area changes and the rest stays intact.',
  img_prov_control:
    'A locally made image must NOT be flagged, otherwise the verifier is unreliable.',
  img_prov_c2pa:
    'OpenAI Verify looks for a trusted C2PA credential issued by OpenAI.',
  img_prov_synthid: 'OpenAI Verify looks for a SynthID watermark.',
  img_prov_model_match:
    'The model named in the credential is compared with the requested model.',
  img_prov_format_matrix: 'Whether signals survive jpeg and webp re-encoding.',
  img_prov_baseline:
    'The same prompt is sent to the official API and compared as a reference.',
  img_usage_fields: 'Usage totals must be consistent when present.',
  img_performance: 'Request latency measured by this server.',
}

export const IMAGE_CODE_NOTES: Record<string, string> = {
  ok: 'Passed',
  accepted_invalid_request: 'An invalid request was accepted',
  accepted_invalid_size: 'An invalid size was accepted',
  all_formats_retain: 'Signals survive every format',
  baseline_failed: 'Baseline generation failed',
  baseline_no_signal: 'The official baseline carried no signal',
  baseline_not_configured: 'Baseline not configured',
  both_signed: 'Both images carry OpenAI signals',
  c2pa_issuer_not_openai: 'Credential not issued by OpenAI',
  c2pa_not_detected: 'No C2PA credential detected',
  cancelled: 'Cancelled',
  color_mismatch: 'Color does not match the prompt',
  control_clean: 'Negative control is clean',
  control_detected: 'Negative control wrongly flagged',
  control_failed: 'Negative control failed to run',
  duplicate_images: 'Images are duplicates',
  endpoint_signal_missing: 'Official image has a signal, endpoint image does not',
  format_mismatch: 'Returned format differs from requested',
  invalid_error_shape: 'Error body is not OpenAI-shaped',
  invalid_response: 'Invalid response',
  layout_mismatch: 'Layout does not match the prompt',
  mask_not_respected: 'Mask was not respected',
  model_match: 'Credential model matches',
  model_mismatch: 'Credential names a different model',
  n_ignored: 'n was ignored',
  no_c2pa_entry: 'No C2PA entry in the response',
  no_format_images: 'No image available for this format',
  no_model_claim: 'The credential names no model',
  no_samples: 'No samples',
  no_synthid_entry: 'No SynthID entry in the response',
  not_transparent: 'Background is not transparent',
  pixels_unavailable: 'Pixels could not be analysed for this format',
  rejected: 'Rejected by the verifier',
  relay_unverified: 'The relay cannot be verified this way',
  request_rejected: 'Request rejected',
  run_stopped: 'Run stopped earlier',
  signal_lost: 'Signal lost after re-encoding',
  silently_downscaled: 'Image was silently downscaled',
  size_mismatch: 'Size does not match',
  stream_not_supported: 'Streaming not supported',
  synthid_detected: 'SynthID detected',
  synthid_not_detected: 'No SynthID detected',
  too_many_partials: 'Too many partial images',
  trusted: 'Trusted OpenAI credential',
  unauthorized: 'Unauthorized',
  usage_absent: 'No usage reported',
  usage_inconsistent: 'Usage totals are inconsistent',
  verifier_not_configured: 'Official key not provided',
  access_denied: 'Access denied',
  forbidden: 'Forbidden',
  network_error: 'Network error',
  rate_limited: 'Rate limited',
  timeout: 'Timed out',
  upstream_error: 'Upstream error',
  no_access: 'The official key has no access to Verify',
  interrupted: 'Interrupted',
  not_requested: 'Not requested',
}

export const PROVENANCE_LEVELS: Record<string, string> = {
  trusted: 'Trusted OpenAI credential found',
  synthid: 'SynthID watermark found',
  untrusted: 'Credential present but not trusted',
  none: 'No OpenAI signal detected',
  unavailable: 'Verification unavailable',
  off: 'Not requested',
}

export const PROVENANCE_LEVEL_NOTES: Record<string, string> = {
  trusted:
    'Strong evidence the image was generated by OpenAI. It does not prove which model.',
  synthid: 'The image carries an OpenAI watermark.',
  untrusted: 'A credential exists but could not be trusted as OpenAI’s.',
  none: 'This proves nothing: signals are removed by re-encoding and many relays strip them.',
  unavailable: 'The verifier could not be reached; no conclusion was drawn.',
  off: 'Enable provenance and provide an official OpenAI key to run it.',
}
