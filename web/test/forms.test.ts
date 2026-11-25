import test from 'node:test';
import assert from 'node:assert/strict';
import { emptyProfile, scalar, stringMap, telegramID, userName } from '../src/forms.js';
import { mergeMessages } from '../src/MessagesPage.js';
import type { Message } from '../src/types.js';

test('Telegram ID retains negative group IDs and rejects lossy input', () => {
  assert.equal(telegramID('-1002374518902'), -1002374518902);
  for (const value of ['', '0', '1.5', '9007199254740993', 'abc']) assert.throws(() => telegramID(value));
});
test('verification scalars preserve JSON types including false and null', () => {
  assert.equal(scalar('false'), false);
  assert.equal(scalar('0'), 0);
  assert.equal(scalar('"0"'), '0');
  assert.equal(scalar('null'), null);
  assert.throws(() => scalar('{"valid":true}'));
  assert.throws(() => scalar('[]'));
});
test('request maps require string values and retain credential template as data', () => {
  assert.deepEqual(stringMap('{"Authorization":"Bearer {{credential}}"}', '请求头'), { Authorization: 'Bearer {{credential}}' });
  assert.throws(() => stringMap('{"attempts":2}', '参数'));
  assert.throws(() => stringMap('[]', '参数'));
  assert.throws(() => stringMap('null', '参数'));
});
test('new profile does not contain secrets or example entitlements', () => {
  const profile = emptyProfile();
  assert.equal(profile.id, ''); assert.equal(profile.request.url, '');
  assert.deepEqual(profile.chatIds, []);
  assert.deepEqual(profile.request.rules, [{ path: '/data/valid', expected: true }]);
});
test('user display handles missing usernames', () => {
  assert.equal(userName({ id: 512397, firstName: '', lastName: '', username: '' }), '512397');
  assert.equal(userName({ id: 512397, firstName: '徐', lastName: '总', username: 'xuzong' }), '徐 总');
});
test('live message merge updates edits, deduplicates echoes and preserves older pages', () => {
  const message = (id: string, date: string, text = '') => ({ id, date, text }) as Message;
  const old = message('old', '2026-11-01T12:00:00Z');
  const current = message('current', '2026-11-02T12:00:00Z', 'before');
  const merged = mergeMessages([current, old], [message('new', '2026-11-03T12:00:00Z'), { ...current, text: 'edited' }]);
  assert.deepEqual(merged.map(item => item.id), ['new', 'current', 'old']);
  assert.equal(merged[1].text, 'edited');
});
