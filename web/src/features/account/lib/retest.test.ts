import assert from 'node:assert/strict'
import { test } from 'node:test'
import { findCredential, normalizeBaseURL } from './credentials'
import { normalizeOptions, planRetest, requestsPerDay, scoreDelta } from './retest'
import type { Credential } from '../api'

test('Claude endpoints lose the request path, other prefixes stay', () => {
  assert.equal(normalizeBaseURL('claude', 'https://relay.example.com/v1/messages'), 'https://relay.example.com')
  assert.equal(normalizeBaseURL('claude', 'https://relay.example.com/v1/messages/count_tokens'), 'https://relay.example.com')
  assert.equal(normalizeBaseURL('claude', 'https://relay.example.com/v1/'), 'https://relay.example.com')
  assert.equal(normalizeBaseURL('claude', 'https://relay.example.com/api/anthropic/v1/messages'), 'https://relay.example.com/api/anthropic')
  assert.equal(normalizeBaseURL('claude', 'https://api.anthropic.com'), 'https://api.anthropic.com')
  assert.equal(normalizeBaseURL('claude', 'https://user:pw@relay.example.com'), null)
  assert.equal(normalizeBaseURL('claude', 'not a url'), null)
})

test('OpenAI and Gemini endpoints follow the server normalizers', () => {
  assert.equal(normalizeBaseURL('openai', 'https://x.example/v1/chat/completions'), 'https://x.example')
  assert.equal(normalizeBaseURL('image', 'https://x.example/v1'), 'https://x.example')
  assert.equal(normalizeBaseURL('gemini', 'https://g.example/v1beta/models/gemini-2.5-pro'), 'https://g.example')
})

test('options are normalized through the current form fields', () => {
  const values = normalizeOptions('openai', { suite: 'full', vision: true, unknown_flag: true, limit_param: 'bogus', key: 'sk-x' })
  assert.equal(values.suite, 'full')
  assert.equal(values.vision, true)
  assert.equal(values.limit_param, 'max_completion_tokens') // invalid value falls back to the default
  assert.ok(!('unknown_flag' in values))
  assert.ok(!('key' in values))
})

test('a Claude baseline that no longer exists is dropped', () => {
  const saved = { cache: false, baseline_id: 'gone', baseline_type: 'Official', veridrop: true }
  const dropped = normalizeOptions('claude', saved, [])
  assert.equal(dropped.baseline_id, '')
  assert.equal(dropped.baseline_type, '')
  assert.equal(dropped.cache, false)
  assert.ok(!('veridrop' in dropped))
  const kept = normalizeOptions('claude', { ...saved, baseline_id: 'b1' }, [
    { id: 'b1', report_id: 'r', type: 'Official', model: 'claude-x', channel_name: '', created_at: 0, fingerprint: false },
  ])
  assert.equal(kept.baseline_id, 'b1')
})

test('planRetest rebuilds provider, base URL, model and request budget', () => {
  const plan = planRetest(
    { transport: 'anthropic_proxy', endpoint: 'https://relay.example.com/v1/messages', model: 'claude-x' },
    { model: 'claude-x', endpoint: 'https://relay.example.com/v1/messages', options: { suite: 'focused', model: 'claude-x', cache: true, vision: true } }
  )
  assert.ok(plan)
  assert.equal(plan.provider, 'claude')
  assert.equal(plan.base_url, 'https://relay.example.com')
  assert.equal(plan.model, 'claude-x')
  assert.equal(plan.options.vision, true)
  assert.ok(plan.requests > 0)
  const image = planRetest({ transport: 'image_api', endpoint: 'https://i.example', model: 'gpt-image-1' }, { options: { suite: 'basic', provenance: true } })
  assert.equal(image?.provider, 'image')
  assert.equal(image?.options.provenance, false)
  assert.equal(planRetest({ transport: 'openai_api', endpoint: 'ftp://x', model: 'm' }, undefined), null)
})

test('a saved key matches only its own base URL and while usable', () => {
  const now = Date.now()
  const key: Credential = {
    id: 'k1', name: 'relay', provider: 'claude', base_url: 'https://relay.example.com', hint: 'sk-a••••wxyz',
    expires_at: now + 1000_000, acknowledged_at: now, created_at: now, last_used_at: 0, use_count: 0, paused_reason: '',
  }
  assert.equal(findCredential([key], 'claude', 'https://Relay.example.com/v1/messages', now)?.id, 'k1')
  assert.equal(findCredential([key], 'claude', 'https://other.example.com', now), undefined)
  assert.equal(findCredential([key], 'openai', 'https://relay.example.com', now), undefined)
  assert.equal(findCredential([{ ...key, paused_reason: 'auth_failed' }], 'claude', 'https://relay.example.com', now), undefined)
  assert.equal(findCredential([{ ...key, expires_at: now - 1 }], 'claude', 'https://relay.example.com', now), undefined)
})

test('schedule cost and score deltas', () => {
  assert.equal(requestsPerDay(20, 60), 480)
  assert.equal(requestsPerDay(10, 1440), 10)
  assert.equal(scoreDelta(90, 100), -10)
  assert.equal(scoreDelta(90, null), null)
})
