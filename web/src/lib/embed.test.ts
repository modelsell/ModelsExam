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

import assert from 'node:assert/strict'
import { test } from 'node:test'
import { imageSnippet, markdownSnippet, normalizeDomain, scriptSnippet } from './embed'

test('normalizes what people paste', () => {
  assert.equal(normalizeDomain(' https://Relay.Example.com:8443/v1?x=1 '), 'relay.example.com')
  assert.equal(normalizeDomain('relay.example.com.'), 'relay.example.com')
  assert.equal(normalizeDomain('http://user:pw@a-b.example.org/'), 'a-b.example.org')
})

test('rejects anything that is not a public domain name', () => {
  for (const bad of ['', 'localhost', '127.0.0.1', '[::1]', 'a_b.example.com', '-x.example.com', 'exa mple.com', 'http://']) {
    assert.equal(normalizeDomain(bad), null, bad)
  }
})

test('snippets point at the domain-specific script and image', () => {
  const o = 'https://modelsexam.com/'
  assert.equal(scriptSnippet(o, 'relay.example.com'), '<script async src="https://modelsexam.com/embed/relay.example.com.js"></script>')
  assert.match(imageSnippet(o, 'relay.example.com'), /src="https:\/\/modelsexam\.com\/badge\/relay\.example\.com\.svg"/)
  assert.equal(
    markdownSnippet(o, 'relay.example.com'),
    '[![ModelsExam badge for relay.example.com](https://modelsexam.com/badge/relay.example.com.svg)](https://modelsexam.com/sites/relay.example.com)'
  )
})

test('theme is an option on every snippet and omitted when auto', () => {
  const o = 'https://modelsexam.com'
  assert.match(scriptSnippet(o, 'a.com', 'dark'), / data-theme="dark"/)
  assert.doesNotMatch(scriptSnippet(o, 'a.com'), /data-theme/)
  assert.match(imageSnippet(o, 'a.com', 'light'), /a\.com\.svg\?theme=light"/)
  assert.match(markdownSnippet(o, 'a.com', 'dark'), /a\.com\.svg\?theme=dark\)/)
})
