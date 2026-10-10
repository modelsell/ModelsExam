import assert from 'node:assert/strict'
import { test } from 'node:test'
import { passwordProblems, USERNAME_PATTERN } from './password'

test('letters and digits are enough', () => {
  assert.deepEqual(passwordProblems('kettle92violet'), [])
  assert.deepEqual(passwordProblems('bob7kettle'), [])
  assert.ok(passwordProblems('abc1234').includes('Password must be at least 8 characters'))
  assert.ok(passwordProblems('12345678901').includes('Password must contain both letters and digits'))
  assert.ok(passwordProblems('onlyletters').includes('Password must contain both letters and digits'))
})

test('usernames', () => {
  assert.ok(USERNAME_PATTERN.test('Alice_01'))
  assert.ok(!USERNAME_PATTERN.test('ab'))
  assert.ok(!USERNAME_PATTERN.test('has space'))
})
