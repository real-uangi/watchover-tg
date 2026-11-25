import { useEffect, useRef, useState, type FormEvent } from 'react';
import { errorText, fileURL, request } from './api.js';
import { Empty, Loading, Notice, useAction, useLoad } from './components.js';
import { chatType, formatDate, messageStatus, userName } from './forms.js';
import type { Chat, Message, Page, User } from './types.js';

const kindLabels: Record<string, string> = { video: '视频', voice: '语音', audio: '音频', animation: '动画', sticker: '贴纸', video_note: '视频消息', other: '其他消息' };

type Destination = { id: number; title: string; type: string };
export function mergeMessages(current: Message[], incoming: Message[]) {
  const index = new Map(current.map(item => [item.id, item]));
  for (const item of incoming) index.set(item.id, item);
  return [...index.values()].sort((a, b) => Date.parse(b.date) - Date.parse(a.date) || b.id.localeCompare(a.id));
}
export function MessagesPage({ initialChat }: { initialChat?: number }) {
  const resource = useLoad(async () => { const [chats, users] = await Promise.all([request<{ items: Chat[] }>('/chats/list'), request<Page<User>>('/users/list')]); return { chats: chats.items, users }; }, []);
  const [selected, setSelected] = useState<number | undefined>(initialChat);
  const [search, setSearch] = useState('');
  const [extraUser, setExtraUser] = useState<User>();
  const action = useAction();
  useEffect(() => { if (initialChat && !resource.loading && !resource.data?.users.items.some(user => user.id === initialChat)) { let active = true; void request<{ user: User }>(`/users/detail?id=${initialChat}`).then(result => { if (active) setExtraUser(result.user); }).catch(cause => { if (active) void action.run(async () => { throw cause; }); }); return () => { active = false; }; } }, [initialChat, resource.loading]);
  const destinations: Destination[] = [...(resource.data?.chats.map(chat => ({ id: chat.id, title: chat.title || chat.username || String(chat.id), type: chat.type })) || []), ...(resource.data?.users.items.filter(user => user.privateChat).map(user => ({ id: user.id, title: userName(user), type: 'private' })) || [])];
  if (extraUser?.privateChat && !destinations.some(item => item.id === extraUser.id)) destinations.push({ id: extraUser.id, title: userName(extraUser), type: 'private' });
  const destination = destinations.find(item => item.id === selected);
  return <><Notice error={resource.error || action.error} /><div className="message-workspace">
    <section className="chat-picker"><div className="picker-heading"><h2>聊天</h2><input aria-label="筛选聊天" placeholder="名称或 ID" value={search} onChange={e => setSearch(e.target.value)} /></div>{resource.loading ? <Loading /> : destinations.length ? <div className="chat-list">{destinations.filter(item => `${item.title} ${item.id}`.toLowerCase().includes(search.toLowerCase())).map(item => <button key={item.id} className={selected === item.id ? 'active' : ''} onClick={() => setSelected(item.id)}><span className="chat-avatar">{item.type === 'private' ? '私' : item.type === 'channel' ? '频' : '群'}</span><span><strong>{item.title}</strong><small>{chatType[item.type] || item.type} · <span className="mono">{item.id}</span></small></span></button>)}</div> : <Empty>暂无聊天。接入群组或等待用户私聊机器人。</Empty>}
      {resource.data?.users.nextCursor && <button className="load-more" disabled={action.busy} onClick={() => void action.run(async () => { const page = await request<Page<User>>(`/users/list?cursor=${encodeURIComponent(resource.data!.users.nextCursor)}`); resource.setData(current => current && ({ ...current, users: { items: [...current.users.items, ...page.items], nextCursor: page.nextCursor } })); })}>加载更多用户</button>}
    </section>
    {destination ? <Timeline key={destination.id} destination={destination} /> : <section className="conversation-placeholder"><p className="eyebrow">MESSAGES</p><h2>选择一个聊天</h2><p>查看近期消息，或代表机器人发送内容。</p></section>}
  </div></>;
}
function Timeline({ destination }: { destination: Destination }) {
  const resource = useLoad(() => request<Page<Message>>(`/messages/list?chatId=${destination.id}`), [destination.id]);
  const action = useAction();
  const [pollError, setPollError] = useState('');
  const [text, setText] = useState('');
  const [kind, setKind] = useState('text');
  const [file, setFile] = useState<File>();
  const [reply, setReply] = useState<Message>();
  const fileInput = useRef<HTMLInputElement>(null);
  const polling = useRef(false);
  useEffect(() => {
    let active = true;
    const poll = async () => {
      if (document.visibilityState !== 'visible' || polling.current) return;
      polling.current = true;
      try { const page = await request<Page<Message>>(`/messages/list?chatId=${destination.id}`); if (active) { resource.setData(current => current ? ({ items: mergeMessages(current.items, page.items), nextCursor: current.nextCursor }) : page); setPollError(''); } }
      catch (cause) { if (active) setPollError(errorText(cause)); }
      finally { polling.current = false; }
    };
    const interval = window.setInterval(() => void poll(), 5000);
    const visible = () => { if (document.visibilityState === 'visible') void poll(); };
    document.addEventListener('visibilitychange', visible);
    return () => { active = false; window.clearInterval(interval); document.removeEventListener('visibilitychange', visible); };
  }, [destination.id]);
  function send(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      const data = new FormData(); data.set('chatId', String(destination.id)); data.set('kind', kind); data.set('text', text);
      if (reply) data.set('replyToMessageId', String(reply.telegramMessageId));
      if (file && kind !== 'text') data.set('file', file);
      const sent = await request<Message>('/messages/send', { method: 'POST', body: data });
      resource.setData(current => ({ items: mergeMessages(current?.items || [], [sent]), nextCursor: current?.nextCursor || '' }));
      setText(''); setFile(undefined); setReply(undefined); if (fileInput.current) fileInput.current.value = '';
    }, 'Telegram 已接受消息。');
  }
  return <section className="conversation"><header className="conversation-heading"><div><h2>{destination.title}</h2><p>{chatType[destination.type]} · <span className="mono">{destination.id}</span></p></div><button disabled={resource.loading} onClick={() => void resource.reload()}>刷新</button></header>
    <div className="conversation-body"><Notice error={resource.error || pollError} />{resource.loading ? <Loading /> : resource.data?.items.length ? <>
      {resource.data.nextCursor && <button className="load-more" disabled={action.busy} onClick={() => void action.run(async () => { const page = await request<Page<Message>>(`/messages/list?chatId=${destination.id}&cursor=${encodeURIComponent(resource.data!.nextCursor)}`); resource.setData(current => current && ({ items: mergeMessages(current.items, page.items), nextCursor: page.nextCursor })); })}>加载更早消息</button>}
      <div className="message-list">{[...resource.data.items].reverse().map(message => <MessageItem key={message.id} message={message} onReply={() => setReply(message)} />)}</div>
    </> : <Empty>暂无消息记录。在下方发送第一条消息。</Empty>}</div>
    <form className="composer" onSubmit={send}><Notice error={action.error} success={action.success} />{reply && <div className="reply-preview"><span>回复 #{reply.telegramMessageId} · {reply.sensitive ? '验证凭据已隐藏' : reply.text || reply.fileName || reply.kind}</span><button type="button" className="text-button" onClick={() => setReply(undefined)}>取消回复</button></div>}<div className="composer-tools"><label><span className="sr-only">消息类型</span><select value={kind} onChange={e => { setKind(e.target.value); setFile(undefined); if (fileInput.current) fileInput.current.value = ''; }}><option value="text">文本</option><option value="photo">图片</option><option value="document">文件</option></select></label>{kind !== 'text' && <input ref={fileInput} aria-label={kind === 'photo' ? '选择图片' : '选择文件'} type="file" accept={kind === 'photo' ? 'image/jpeg,image/png,image/webp' : undefined} onChange={e => setFile(e.target.files?.[0])} required />}</div><label><span className="sr-only">消息内容</span><textarea rows={3} value={text} onChange={e => setText(e.target.value)} maxLength={kind === 'text' ? 4096 : 1024} required={kind === 'text'} placeholder={kind === 'text' ? '输入消息…' : '添加说明（可选）'} /></label><div className="composer-footer"><small>以机器人身份发送</small><button className="primary" disabled={action.busy || (kind === 'text' ? !text.trim() : !file)}>{action.busy ? '发送中…' : '发送消息'}</button></div></form>
  </section>;
}
function MessageItem({ message, onReply }: { message: Message; onReply: () => void }) {
  const [imageFailed, setImageFailed] = useState(false);
  const outgoing = message.direction === 'out';
  const downloadAvailable = !!message.fileId && (!message.fileSize || message.fileSize <= 20 * 1024 * 1024);
  return <article className={`message-item ${outgoing ? 'outgoing' : ''}`}><div className="message-meta"><strong>{outgoing ? '机器人' : message.senderName || String(message.senderId || '频道')}</strong><time>{formatDate(message.date)}</time></div><div className="message-bubble">
    {message.replyToMessageId && <span className="quoted-message">回复 #{message.replyToMessageId}</span>}
    {message.sensitive ? <p className="muted">验证凭据已隐藏</p> : <>
      {message.kind === 'photo' && downloadAvailable && !imageFailed && <a href={fileURL(message.id)} target="_blank" rel="noreferrer"><img className="message-photo" src={fileURL(message.id)} alt="收到的图片" loading="lazy" onError={() => setImageFailed(true)} /></a>}
      {message.fileId && <div className="message-file"><span>{message.fileName || (message.kind === 'photo' ? '图片' : '附件')}{message.fileSize ? ` · ${(message.fileSize / 1024 / 1024).toFixed(1)} MB` : ''}</span>{downloadAvailable ? <a href={fileURL(message.id)} target="_blank" rel="noreferrer">{message.kind === 'photo' ? '查看原图' : '下载文件'}</a> : <small>超出 Telegram 官方下载上限，无法在后台下载。</small>}{imageFailed && <small>图片暂时无法预览，可点击“查看原图”。</small>}</div>}
      {message.text && <p className="message-text">{message.text}</p>}{!message.text && !message.fileId && <p className="muted">[{kindLabels[message.kind] || message.kind}]</p>}
    </>}
    <div className="message-bottom"><span>{messageStatus[message.status] || message.status}{message.editedAt ? ' · 已编辑' : ''}</span>{message.telegramMessageId > 0 && !message.sensitive && <button className="text-button" type="button" onClick={onReply}>回复</button>}</div>
  </div></article>;
}
