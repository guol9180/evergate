import assert from 'node:assert/strict'
import test from 'node:test'
import { pathnameFromHref, withBasePath } from '../src/lib/runtime.ts'

test('runtime derives an empty root base path', () => {
  assert.equal(pathnameFromHref('/'), '')
  assert.equal(pathnameFromHref('./'), '')
  assert.equal(withBasePath('/admin/api/auth/me', ''), '/admin/api/auth/me')
})

test('runtime joins assets and APIs under a configured base path', () => {
  assert.equal(pathnameFromHref('/eg/', 'http://127.0.0.1:8787'), '/eg')
  assert.equal(withBasePath('/admin/api/auth/me', '/eg'), '/eg/admin/api/auth/me')
  assert.equal(withBasePath('logo.svg', '/eg'), '/eg/logo.svg')
})
