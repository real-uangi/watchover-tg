import { useState, type FormEvent } from 'react';
import { post, request } from './api.js';
import { Badge, Empty, Field, Loading, Notice, Section, useAction, useLoad } from './components.js';
import { chatType, formatDate, telegramID } from './forms.js';
import { MembersTable } from './MembersTable.js';
import type { Chat, Invite, Member } from './types.js';

export function ChatsPage() {
  const resource = useLoad(() => request<{ items: Chat[] }>('/chats/list'), []);
  const action = useAction();
  const [addID, setAddID] = useState('');
  const [selected, setSelected] = useState<number>();
  const [version, setVersion] = useState(0);
  async function changed() { await resource.reload(); setVersion(value => value + 1); }
  function add(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => { const chat = await post<Chat>('/chats/add', { id: telegramID(addID) }); setAddID(''); setSelected(chat.id); await changed(); }, '群组已接入。');
  }
  const chat = resource.data?.items.find(item => item.id === selected);
  return <><Notice error={resource.error || action.error} success={action.success} />
    <Section title="接入群组" description="先将机器人添加为管理员。机器人发现的群组也会显示在此。">
      <form className="inline-form" onSubmit={add}><Field label="Chat ID"><input inputMode="numeric" value={addID} onChange={e => setAddID(e.target.value)} placeholder="输入群组或频道数字 ID" required /></Field><button className="primary" disabled={action.busy}>添加并检查</button></form>
      {resource.loading ? <Loading /> : resource.data?.items.length ? <div className="table-scroll"><table><thead><tr><th>群组／频道</th><th>类型</th><th>状态</th><th>机器人权限</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(item => <tr className={selected === item.id ? 'selected-row' : ''} key={item.id}><td><button className="text-button strong" onClick={() => setSelected(item.id)}>{item.title || item.username || item.id}</button><small className="mono">{item.id}</small></td><td>{chatType[item.type] || item.type}</td><td><Badge on={item.enabled}>{item.enabled ? '启用' : '停用'}</Badge></td><td><div className="permission-list"><span className={item.canInvite ? 'permission-ok' : ''}>邀请</span><span className={item.canRestrict ? 'permission-ok' : ''}>成员管理</span>{item.type === 'channel' && <span className={item.canPost ? 'permission-ok' : ''}>发帖</span>}</div></td><td><div className="button-row compact"><button onClick={() => setSelected(item.id)}>查看</button><button disabled={action.busy} onClick={() => void action.run(async () => { await post('/chats/check', { id: item.id }); await changed(); }, '权限已更新。')}>检查权限</button><button disabled={action.busy} onClick={() => void action.run(async () => { await post('/chats/save', { id: item.id, enabled: !item.enabled }); await changed(); }, item.enabled ? '已停用后续准入。' : '已启用。')}>{item.enabled ? '停用' : '启用'}</button></div></td></tr>)}</tbody></table></div> : <Empty>尚未接入群组。添加机器人为管理员后，在上方输入 Chat ID。</Empty>}
    </Section>
    {chat && <ChatDetail key={`${chat.id}:${version}`} chat={chat} chats={resource.data!.items} />}
    {resource.error && <button onClick={() => void resource.reload()}>重新加载</button>}
  </>;
}
function ChatDetail({ chat, chats }: { chat: Chat; chats: Chat[] }) {
  const resource = useLoad(async () => { const [members, invites] = await Promise.all([request<{ items: Member[] }>(`/chats/members?chatId=${chat.id}`), request<{ items: Invite[] }>(`/chats/invites?chatId=${chat.id}`)]); return { members: members.items, invites: invites.items }; }, [chat.id]);
  return <><Section title={chat.title || String(chat.id)} description={`${chatType[chat.type] || chat.type} · ${chat.id}`} action={<button disabled={resource.loading} onClick={() => void resource.reload()}>刷新记录</button>}>
    <Notice error={resource.error} />{!chat.canInvite && <div className="notice">机器人缺少邀请权限，无法发放审批邀请或处理入群申请。</div>}
    <h3 className="table-label">已观察成员</h3>{resource.loading ? <Loading /> : resource.data && <MembersTable members={resource.data.members} chats={chats} onChange={resource.reload} />}
  </Section><Section title="邀请记录" description="邀请与具体用户绑定，仅供查看状态。">{resource.data?.invites.length ? <div className="table-scroll"><table><thead><tr><th>用户 ID</th><th>状态</th><th>有效期至</th></tr></thead><tbody>{resource.data.invites.map(invite => <tr key={invite.id}><td className="mono">{invite.userId}</td><td>{invite.status}</td><td>{formatDate(invite.expiresAt)}</td></tr>)}</tbody></table></div> : <Empty>暂无邀请记录。用户通过验证并选择加入时，会创建绑定该用户的邀请。</Empty>}</Section></>;
}
