import { useState } from 'react';
import { post } from './api.js';
import { Empty, Notice, useAction } from './components.js';
import { formatDate, memberStatus } from './forms.js';
import type { Chat, Member } from './types.js';

export function MembersTable({ members, chats, onChange }: { members: Member[]; chats: Chat[]; onChange: () => Promise<unknown> }) {
  if (!members.length) return <Empty>尚未观察到成员状态。新成员变更会在机器人接收到更新后显示。</Empty>;
  return <div className="table-scroll"><table><thead><tr><th>用户 ID</th><th>群组／频道</th><th>状态</th><th>更新时间</th><th>成员操作</th></tr></thead><tbody>{members.map(member => <MemberRow key={`${member.chatId}:${member.userId}`} member={member} chat={chats.find(chat => chat.id === member.chatId)} onChange={onChange} />)}</tbody></table></div>;
}
function MemberRow({ member, chat, onChange }: { member: Member; chat?: Chat; onChange: () => Promise<unknown> }) {
  const action = useAction();
  const [operation, setOperation] = useState('refresh');
  const [duration, setDuration] = useState('0');
  const title = chat?.title || String(member.chatId);
  const labels: Record<string, string> = { refresh: '刷新状态', ban: '封禁', unban: '解封', kick: '移出', mute: '禁言', unmute: '解除禁言' };
  function execute() {
    if (operation !== 'refresh' && !window.confirm(`确认${labels[operation]}用户 ${member.userId}？\n目标：${title}\n群组 ID：${member.chatId}${operation === 'kick' ? '\n移出后允许重新加入。' : ''}`)) return;
    void action.run(async () => {
      await post<Member>('/members/action', { chatId: member.chatId, userId: member.userId, action: operation, untilDate: ['ban', 'mute'].includes(operation) && duration !== '0' ? Math.floor(Date.now() / 1000) + Number(duration) : 0 });
      await onChange();
    }, `${labels[operation]}已完成。`);
  }
  return <tr><td className="mono">{member.userId}</td><td><strong>{title}</strong><small className="mono">{member.chatId}</small></td><td>{memberStatus[member.status] || member.status}{member.untilDate > 0 && <small>至 {formatDate(member.untilDate)}</small>}</td><td className="muted nowrap">{formatDate(member.updatedAt)}</td><td><div className="member-actions"><select aria-label={`用户 ${member.userId} 的成员操作`} value={operation} onChange={e => setOperation(e.target.value)}><option value="refresh">刷新状态</option>{chat?.canRestrict && <><option value="ban">封禁</option>{chat.type !== 'group' && <><option value="unban">解封</option><option value="kick">移出</option></>}{chat.type === 'supergroup' && <><option value="mute">禁言</option><option value="unmute">解除禁言</option></>}</>}</select>{['ban', 'mute'].includes(operation) && <select aria-label="持续时间" value={duration} onChange={e => setDuration(e.target.value)}><option value="0">永久</option><option value="3600">1 小时</option><option value="86400">1 天</option><option value="604800">7 天</option></select>}<button disabled={action.busy} onClick={execute}>{action.busy ? '处理中…' : '执行'}</button></div><Notice error={action.error} success={action.success} /></td></tr>;
}
