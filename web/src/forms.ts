import type { Profile, Rule } from './types.js';

export function telegramID(value: string): number {
  const id = Number(value);
  if (!value.trim() || !Number.isSafeInteger(id) || id === 0) throw new Error('请输入有效的 Telegram 数字 ID。');
  return id;
}
export function stringMap(raw: string, label: string): Record<string, string> {
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.values(value).some(item => typeof item !== 'string')) {
    throw new Error(`${label}需要 JSON 对象，字段值均为字符串。`);
  }
  return value as Record<string, string>;
}
export function scalar(raw: string): Rule['expected'] {
  const value: unknown = JSON.parse(raw);
  if (value !== null && !['string', 'boolean', 'number'].includes(typeof value)) throw new Error('期望值只能是 JSON 字符串、数字、布尔值或 null。');
  return value as Rule['expected'];
}
export function emptyProfile(): Profile {
  return { id: '', name: '', enabled: true, prompt: '请发送您的验证凭据。', chatIds: [], request: { method: 'GET', url: '', headers: {}, query: { credential: '{{credential}}' }, rules: [{ path: '/data/valid', expected: true }] } };
}
export function formatDate(value: string | number) {
  if (!value) return '—';
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short', timeZone: 'Asia/Shanghai' }).format(new Date(typeof value === 'number' ? value * 1000 : value));
}
export function userName(user: { firstName: string; lastName: string; username: string; id: number }) {
  return [user.firstName, user.lastName].filter(Boolean).join(' ') || (user.username ? `@${user.username}` : String(user.id));
}
export const chatType: Record<string, string> = { group: '群组', supergroup: '超级群', channel: '频道', private: '私聊' };
export const memberStatus: Record<string, string> = { creator: '群主', administrator: '管理员', member: '成员', restricted: '受限', left: '已离开', kicked: '已封禁' };
export const messageStatus: Record<string, string> = { received: '已接收', sent: '已发送', failed: '发送失败', unknown: '结果未知' };
