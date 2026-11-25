import { useEffect, useState, type FormEvent } from 'react';
import { post, request } from './api.js';
import { Badge, Empty, Field, Loading, Notice, Section, useAction, useLoad } from './components.js';
import { formatDate, userName } from './forms.js';
import { MembersTable } from './MembersTable.js';
import type { Chat, Page, Profile, User, UserDetail } from './types.js';

export function UsersPage({ onMessage }: { onMessage: (id: number) => void }) {
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<number>();
  const resource = useLoad(() => request<Page<User>>(`/users/list?q=${encodeURIComponent(search)}`), [search]);
  const supporting = useLoad(async () => { const [chats, profiles] = await Promise.all([request<{ items: Chat[] }>('/chats/list'), request<{ items: Profile[] }>('/profiles/list')]); return { chats: chats.items, profiles: profiles.items }; }, []);
  const action = useAction();
  function submit(event: FormEvent) { event.preventDefault(); if (query.trim() === search) void resource.reload(); else setSearch(query.trim()); }
  return <><Notice error={resource.error || supporting.error || action.error} />
    <Section title="用户档案" description="展示已与机器人交互或已观察到的用户。">
      <form onSubmit={submit} className="inline-form"><Field label="搜索用户"><input value={query} onChange={e => setQuery(e.target.value)} placeholder="Telegram ID 或用户名" /></Field><button>搜索</button></form>
      {resource.loading ? <Loading /> : resource.data?.items.length ? <div className="table-scroll"><table><thead><tr><th>用户</th><th>Telegram ID</th><th>私聊</th><th>最近更新</th><th>操作</th></tr></thead><tbody>{resource.data.items.map(user => <tr className={selected === user.id ? 'selected-row' : ''} key={user.id}><td><strong>{userName(user)}</strong><small>{user.username ? `@${user.username}` : '未设置用户名'}</small></td><td className="mono">{user.id}</td><td><Badge on={user.privateChat}>{user.privateChat ? '已建立会话' : '未建立会话'}</Badge></td><td className="muted nowrap">{formatDate(user.updatedAt)}</td><td><button onClick={() => setSelected(user.id)}>查看档案</button></td></tr>)}</tbody></table></div> : <Empty>{search ? '没有找到匹配的用户。' : '尚无用户。用户与机器人交互后会显示在此。'}</Empty>}
      {resource.data?.nextCursor && <button className="load-more" disabled={action.busy} onClick={() => void action.run(async () => { const page = await request<Page<User>>(`/users/list?q=${encodeURIComponent(search)}&cursor=${encodeURIComponent(resource.data!.nextCursor)}`); resource.setData(current => current && ({ items: [...current.items, ...page.items], nextCursor: page.nextCursor })); })}>加载更多用户</button>}
    </Section>
    {selected && supporting.data && <UserCard key={selected} id={selected} chats={supporting.data.chats} profiles={supporting.data.profiles} onMessage={onMessage} />}
    {(resource.error || supporting.error) && <button onClick={() => { void resource.reload(); void supporting.reload(); }}>重新加载</button>}
  </>;
}
function UserCard({ id, chats, profiles, onMessage }: { id: number; chats: Chat[]; profiles: Profile[]; onMessage: (id: number) => void }) {
  const resource = useLoad(() => request<UserDetail>(`/users/detail?id=${id}`), [id]);
  const action = useAction();
  const [note, setNote] = useState('');
  useEffect(() => { if (resource.data) setNote(resource.data.user.note); }, [resource.data]);
  return <Section title={resource.data ? userName(resource.data.user) : '用户档案'} description={`Telegram ID · ${id}`} action={resource.data?.user.privateChat && <button onClick={() => onMessage(id)}>发送私聊消息</button>}>
    <Notice error={resource.error || action.error} success={action.success} />{resource.loading ? <Loading /> : resource.data && <>
      <form className="inline-form" onSubmit={event => { event.preventDefault(); void action.run(async () => { await post('/users/note', { id, note }); await resource.reload(); }, '备注已保存。'); }}><Field label="管理员备注"><input value={note} onChange={e => setNote(e.target.value)} placeholder="添加备注，便于识别" /></Field><button disabled={action.busy}>保存备注</button></form>
      <h3 className="table-label">身份资格</h3>{resource.data.grants.length ? <div className="table-scroll"><table><thead><tr><th>核验配置</th><th>资格</th><th>外部身份</th><th>验证时间</th><th>操作</th></tr></thead><tbody>{resource.data.grants.map(grant => <tr key={grant.profileId}><td>{profiles.find(profile => profile.id === grant.profileId)?.name || grant.profileId}</td><td><Badge on={grant.active}>{grant.active ? '有效' : '已撤销'}</Badge></td><td>{grant.displayName || '—'}<small className="mono">{grant.externalId || ''}</small></td><td className="nowrap">{formatDate(grant.verifiedAt)}</td><td><button className="danger-quiet" disabled={action.busy || !grant.active} onClick={() => { const profileName = profiles.find(profile => profile.id === grant.profileId)?.name || grant.profileId; if (window.confirm(`撤销用户 ${userName(resource.data!.user)}（${id}）的“${profileName}”资格？\n此操作不会自动移出已加入的群组。`)) void action.run(async () => { await post('/users/revoke', { userId: id, profileId: grant.profileId }); await resource.reload(); }, '资格已撤销。'); }}>撤销资格</button></td></tr>)}</tbody></table></div> : <Empty>该用户尚未通过身份核验。</Empty>}
      <h3 className="table-label">成员状态</h3><MembersTable members={resource.data.members} chats={chats} onChange={resource.reload} />
    </>}
  </Section>;
}
