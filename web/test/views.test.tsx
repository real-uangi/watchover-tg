import test from 'node:test';
import assert from 'node:assert/strict';
import { renderToStaticMarkup } from 'react-dom/server';
import { MembersTable } from '../src/MembersTable.js';
import { ProfileEditor, VerificationTestDetails } from '../src/ProfilesPage.js';
import { emptyProfile } from '../src/forms.js';
import type { Chat, Member, VerificationTestResult } from '../src/types.js';

const member: Member = { chatId: -10023456789, userId: 68219043, status: 'member', untilDate: 0, updatedAt: '2026-11-25T01:00:00Z' };
const chat: Chat = { id: member.chatId, type: 'channel', title: '成员频道', username: '', enabled: true, botStatus: 'administrator', canInvite: true, canRestrict: true, canPost: true };

test('channel controls omit unsupported mute and keep explicit user target', () => {
  const html = renderToStaticMarkup(<MembersTable members={[member]} chats={[chat]} onChange={async () => {}} />);
  assert.ok(html.includes('68219043')); assert.ok(html.includes('成员频道'));
  assert.ok(html.includes('value="kick"')); assert.ok(html.includes('value="unban"'));
  assert.ok(!html.includes('value="mute"'));
});
test('supergroup controls offer mute while missing restrict permission removes mutations', () => {
  const html = renderToStaticMarkup(<MembersTable members={[member]} chats={[{ ...chat, type: 'supergroup' }]} onChange={async () => {}} />);
  assert.ok(html.includes('value="mute"'));
  const readonly = renderToStaticMarkup(<MembersTable members={[member]} chats={[{ ...chat, canRestrict: false }]} onChange={async () => {}} />);
  assert.ok(!readonly.includes('value="ban"')); assert.ok(readonly.includes('value="refresh"'));
});

test('saved profile starts collapsed with its draft form mounted and testing outside details', () => {
  const profile = { ...emptyProfile(), id: 'saved-profile', name: '核验方案草稿' };
  const html = renderToStaticMarkup(<ProfileEditor initial={profile} chats={[]} onClose={() => {}} onSaved={async () => {}} onDeleted={async () => {}} />);
  const details = html.match(/<details\b[^>]*>/)?.[0];
  assert.ok(details);
  assert.ok(!details.includes('open='));
  assert.ok(html.includes('value="核验方案草稿"'));
  assert.ok(html.indexOf('<form') < html.indexOf('</details>'));
  assert.ok(html.indexOf('class="test-area"') > html.indexOf('</details>'));
  assert.ok(html.includes('展开配置')); assert.ok(html.includes('关闭面板'));
  assert.ok(html.includes('执行测试'));
});

test('new profile starts with configuration expanded', () => {
  const html = renderToStaticMarkup(<ProfileEditor initial={emptyProfile()} chats={[]} onClose={() => {}} onSaved={async () => {}} onDeleted={async () => {}} />);
  assert.match(html, /<details\b[^>]*open=""/);
  assert.ok(html.includes('收起配置'));
});

const verificationTest: VerificationTestResult = {
  passed: false, message: '外部接口返回 HTTP 403', durationMs: 18,
  request: { method: 'POST', url: 'https://verify.example/check?credential=test-secret', headers: { Authorization: ['Bearer external-key'] }, body: '{"credential":"test-secret"}' },
  response: { statusCode: 403, headers: { 'X-Trace': ['first', 'second'] }, body: '<script>alert("upstream")</script>', truncated: false },
};

test('verification details retain full request and repeated response headers while escaping upstream HTML', () => {
  const html = renderToStaticMarkup(<VerificationTestDetails result={verificationTest} />);
  assert.ok(html.includes('POST https://verify.example/check?credential=test-secret'));
  assert.ok(html.includes('Authorization: Bearer external-key'));
  assert.ok(html.includes('credential')); assert.ok(html.includes('test-secret'));
  assert.ok(html.includes('HTTP 403')); assert.ok(html.includes('18 ms'));
  assert.ok(html.includes('X-Trace: first\nX-Trace: second'));
  assert.ok(html.includes('&lt;script&gt;')); assert.ok(!html.includes('<script>'));
  assert.equal((html.match(/<pre\b/g) ?? []).length, 2);
});

test('verification details distinguish no response from a truncated response', () => {
  const missing = renderToStaticMarkup(<VerificationTestDetails result={{ ...verificationTest, response: null, message: '网络请求失败' }} />);
  assert.ok(missing.includes('未收到响应'));
  assert.ok(missing.includes('网络请求失败'));
  assert.ok(!missing.includes('HTTP 403'));
  const truncated = renderToStaticMarkup(<VerificationTestDetails result={{ ...verificationTest, response: { ...verificationTest.response!, truncated: true } }} />);
  assert.ok(truncated.includes('响应正文超过 1 MiB，已截断。'));
});
