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
import { siteLabel } from './site-label'

test('uses the stored name, url and description', () => {
  assert.deepEqual(
    siteLabel({ channel_name: ' Relay Co ', site_description: 'Fast.', endpoint: 'https://relay.example/v1' }),
    { name: 'Relay Co', url: 'https://relay.example/v1', host: 'relay.example', description: 'Fast.' }
  )
})

test('falls back to the host name for older runs', () => {
  const label = siteLabel({ channel_name: '', endpoint: 'https://api.example.com:8443/v1' })
  assert.equal(label.name, 'api.example.com:8443')
  assert.equal(label.description, '')
})

test('copes with a missing or invalid endpoint', () => {
  assert.equal(siteLabel({}).name, '—')
  assert.equal(siteLabel({ endpoint: 'not a url' }).host, '')
})
