import test from 'node:test';
import assert from 'node:assert/strict';
import { renderToStaticMarkup } from 'react-dom/server';
import { MembersTable } from '../src/MembersTable.js';
import type { Chat, Member } from '../src/types.js';

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
