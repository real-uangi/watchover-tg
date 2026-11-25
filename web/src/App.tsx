import { useEffect, useState, type FormEvent } from 'react';
import { ApiError, errorText, post, request } from './api.js';
import { Loading, Notice } from './components.js';
import { SettingsPage } from './SettingsPage.js';
import { ProfilesPage } from './ProfilesPage.js';
import { ChatsPage } from './ChatsPage.js';
import { UsersPage } from './UsersPage.js';
import { MessagesPage } from './MessagesPage.js';

type Tab = 'settings' | 'profiles' | 'chats' | 'users' | 'messages';
const tabs: { id: Tab; title: string; subtitle: string }[] = [
  { id: 'settings', title: '机器人设置', subtitle: '机器人与 Webhook 接入' },
  { id: 'profiles', title: '核验配置', subtitle: '验证资格，开放对应群组' },
  { id: 'chats', title: '群组与频道', subtitle: '管理接入群组和成员' },
  { id: 'users', title: '用户', subtitle: '身份资格与成员状态' },
  { id: 'messages', title: '消息', subtitle: '查看记录，收发与回复' },
];
export default function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [initialError, setInitialError] = useState('');
  const [tab, setTab] = useState<Tab>('settings');
  const [privateChat, setPrivateChat] = useState<number>();
  async function session() {
    setInitialError(''); setAuthenticated(null);
    try { await request('/auth/session'); setAuthenticated(true); }
    catch (cause) { if (!(cause instanceof ApiError && cause.status === 401)) setInitialError(errorText(cause)); setAuthenticated(false); }
  }
  useEffect(() => {
    void session();
    const expired = () => setAuthenticated(false);
    window.addEventListener('watchover:unauthorized', expired);
    return () => window.removeEventListener('watchover:unauthorized', expired);
  }, []);
  const active = tabs.find(item => item.id === tab)!;
  if (authenticated === null) return <div className="login-shell"><Loading /></div>;
  if (!authenticated) return <Login onLogin={() => { setAuthenticated(true); setInitialError(''); }} initialError={initialError} onRetry={() => void session()} />;
  return <div className="app-shell">
    <aside className="sidebar"><a className="brand" href="/" aria-label="Watchover 首页"><span className="brand-mark">W</span><span>watchover<span className="brand-sub">TELEGRAM WORKSPACE</span></span></a>
      <span className="nav-label">工作台</span><nav aria-label="主导航">{tabs.map((item, index) => <button type="button" key={item.id} className={tab === item.id ? 'active' : ''} onClick={() => setTab(item.id)}><span className="nav-number">0{index + 1}</span>{item.title}</button>)}</nav>
      <div className="sidebar-bottom"><span className="connection-dot" />单机器人 · Webhook<button className="text-button" type="button" onClick={async () => { try { await post('/auth/logout'); setAuthenticated(false); setPrivateChat(undefined); } catch (cause) { window.alert(errorText(cause)); } }}>退出登录</button></div>
    </aside>
    <main className="main"><header className="page-header"><div><p className="eyebrow">WATCHOVER / ADMIN</p><h1>{active.title}</h1><p>{active.subtitle}</p></div><span className="admin-label">管理员</span></header>
      {tab === 'settings' && <SettingsPage />}
      {tab === 'profiles' && <ProfilesPage />}
      {tab === 'chats' && <ChatsPage />}
      {tab === 'users' && <UsersPage onMessage={id => { setPrivateChat(id); setTab('messages'); }} />}
      {tab === 'messages' && <MessagesPage initialChat={privateChat} />}
      <footer className="page-footer"><span>Watchover TG</span><span>时间显示为北京时间</span></footer>
    </main>
  </div>;
}

function Login({ onLogin, initialError, onRetry }: { onLogin: () => void; initialError: string; onRetry: () => void }) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError('');
    try { await post('/auth/login', { password }); setPassword(''); onLogin(); }
    catch (cause) { setError(errorText(cause)); }
    finally { setBusy(false); }
  }
  return <div className="login-shell"><div className="login-intro"><span className="brand-mark">W</span><p className="eyebrow">WATCHOVER</p><h1>群组、用户与消息，<br />在同一处管理。</h1><p>Telegram 机器人管理平台</p></div><form className="login-form" onSubmit={submit}><p className="eyebrow">ADMIN ACCESS</p><h2>登录工作台</h2><p>使用管理员口令继续。</p><label className="field"><span>管理员口令</span><input type="password" autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)} autoFocus /></label><Notice error={error || initialError} /><button className="primary" disabled={busy}>{busy ? '登录中…' : '登录'}</button>{initialError && <button type="button" className="text-button" onClick={onRetry}>重新检查连接</button>}</form></div>;
}
