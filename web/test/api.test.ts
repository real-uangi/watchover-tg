import test from 'node:test';
import assert from 'node:assert/strict';
import { post, request, fileURL, ApiError } from '../src/api.js';

test('API posts JSON with cookie credentials and no browser storage', async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = async (input, init) => {
      assert.equal(input, '/api/v1/admin/profiles/save');
      assert.equal(init?.credentials, 'same-origin'); assert.equal(init?.cache, 'no-store');
      assert.equal(init?.body, '{"enabled":false,"chatIds":[]}');
      return new Response(JSON.stringify({ code: 200, message: 'OK', data: { saved: true } }), { status: 200 });
    };
    assert.deepEqual(await post('/profiles/save', { enabled: false, chatIds: [] }), { saved: true });
  } finally { globalThis.fetch = original; }
});
test('API rejects HTTP/envelope mismatch and hides raw network errors', async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = async () => new Response(JSON.stringify({ code: 202, message: '协议错误', data: {} }), { status: 200 });
    await assert.rejects(request('/settings'), /协议错误/);
    globalThis.fetch = async () => { throw new Error('https://secret.example/token-sensitive'); };
    await assert.rejects(request('/settings'), (cause: unknown) => cause instanceof ApiError && !cause.message.includes('sensitive') && cause.status === 0);
  } finally { globalThis.fetch = original; }
});
test('media URLs remain same-origin and quote opaque identifiers', () => {
  assert.equal(fileURL('record&token=unsafe'), '/api/v1/admin/messages/file?id=record%26token%3Dunsafe');
});
