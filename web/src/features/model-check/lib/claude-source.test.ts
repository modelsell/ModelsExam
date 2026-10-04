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
import { sourceOfEndpoint } from './claude-source'

test('tells the vendor, AWS and relays apart by endpoint host', () => {
  assert.equal(sourceOfEndpoint('https://api.anthropic.com'), 'official')
  assert.equal(sourceOfEndpoint('https://API.anthropic.com/v1'), 'official')
  assert.equal(sourceOfEndpoint('https://bedrock-mantle.us-east-1.api.aws'), 'aws')
  assert.equal(sourceOfEndpoint('https://bedrock-runtime.us-west-2.amazonaws.com'), 'aws')
  assert.equal(sourceOfEndpoint('https://relay.example.com'), 'relay')
  assert.equal(sourceOfEndpoint('https://api.anthropic.com.evil.example'), 'relay')
  assert.equal(sourceOfEndpoint('not a url'), 'relay')
  assert.equal(sourceOfEndpoint(undefined), 'relay')
})

test('a Bedrock runtime transport is AWS whatever the endpoint says', () => {
  assert.equal(sourceOfEndpoint('', 'bedrock_runtime'), 'aws')
})
