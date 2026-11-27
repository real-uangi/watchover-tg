import { useState, type FormEvent } from 'react';
import { post, request } from './api.js';
import { Badge, Empty, Field, Loading, Notice, Section, useAction, useLoad } from './components.js';
import { emptyProfile, scalar, stringMap, telegramID } from './forms.js';
import type { Chat, Profile, VerificationTestResult } from './types.js';

export function ProfilesPage() {
  const resource = useLoad(async () => { const [profiles, chats] = await Promise.all([request<{ items: Profile[] }>('/profiles/list'), request<{ items: Chat[] }>('/chats/list')]); return { profiles: profiles.items, chats: chats.items }; }, []);
  const [selected, setSelected] = useState<Profile>();
  const [editorKey, setEditorKey] = useState(0);
  const [notice, setNotice] = useState('');
  function edit(profile: Profile) { setSelected(profile); setEditorKey(key => key + 1); }
  return <><Notice error={resource.error} success={notice} />
    <Section title="核验方案" description="每套方案通过验证后，开放其关联的群组和频道。" action={<button className="primary" onClick={() => edit(emptyProfile())}>新建核验配置</button>}>
      {resource.loading ? <Loading /> : resource.data?.profiles.length ? <div className="item-list">{resource.data.profiles.map(profile => <button className={`list-item ${selected?.id === profile.id ? 'selected' : ''}`} key={profile.id} onClick={() => edit(profile)}><div><strong>{profile.name}</strong><p>{profile.request.method} <span className="mono">{profile.request.url.replace(/\?.*$/, '')}</span></p></div><div className="list-meta"><span>{profile.chatIds.length} 个群组</span><Badge on={profile.enabled}>{profile.enabled ? '启用' : '停用'}</Badge><span>编辑</span></div></button>)}</div> : <Empty>尚无核验配置。新建配置并关联群组，用户即可在机器人中选择验证。</Empty>}
    </Section>
    {selected && resource.data && <ProfileEditor key={editorKey} initial={selected} chats={resource.data.chats} onClose={() => setSelected(undefined)} onSaved={async (profile) => { await resource.reload(); edit(profile); setNotice('核验配置已保存。'); }} onDeleted={async () => { setSelected(undefined); await resource.reload(); }} />}
    {resource.error && <button onClick={() => void resource.reload()}>重新加载</button>}
  </>;
}

export function ProfileEditor({ initial, chats, onClose, onSaved, onDeleted }: { initial: Profile; chats: Chat[]; onClose: () => void; onSaved: (profile: Profile) => Promise<void>; onDeleted: () => Promise<void> }) {
  const [profile, setProfile] = useState(initial);
  const [configOpen, setConfigOpen] = useState(!initial.id);
  const [headers, setHeaders] = useState(JSON.stringify(initial.request.headers ?? {}, null, 2));
  const [query, setQuery] = useState(JSON.stringify(initial.request.query ?? {}, null, 2));
  const [body, setBody] = useState(initial.request.body === undefined ? '' : JSON.stringify(initial.request.body, null, 2));
  const [rules, setRules] = useState(initial.request.rules.map(rule => ({ path: rule.path, expected: JSON.stringify(rule.expected) })));
  const [credential, setCredential] = useState('');
  const [testUserID, setTestUserID] = useState('');
  const [testUsername, setTestUsername] = useState('');
  const [result, setResult] = useState<VerificationTestResult>();
  const action = useAction();
  const testAction = useAction();
  const busy = action.busy || testAction.busy;
  function value(): Profile {
    return { ...profile, request: { ...profile.request, headers: stringMap(headers, '请求头'), query: stringMap(query, 'Query 参数'), ...(body.trim() ? { body: JSON.parse(body) as unknown } : { body: undefined }), rules: rules.map(rule => ({ path: rule.path, expected: scalar(rule.expected) })) } };
  }
  function submit(event: FormEvent) { event.preventDefault(); void action.run(async () => onSaved(await post<Profile>('/profiles/save', value())), '核验配置已保存。'); }
  return <Section title={initial.id ? `编辑 · ${initial.name}` : '新建核验配置'} action={<button type="button" onClick={onClose}>关闭面板</button>}>
    <details className="profile-config" open={configOpen} onToggle={event => setConfigOpen(event.currentTarget.open)}>
    <summary>配置表单<span>{configOpen ? '收起配置' : '展开配置'}</span></summary>
    <Notice error={action.error} success={action.success} />
    <form onSubmit={submit} className="form-grid">
      <Field label="名称"><input value={profile.name} onChange={e => setProfile({ ...profile, name: e.target.value })} required /></Field>
      <Field label="状态"><select value={profile.enabled ? 'on' : 'off'} onChange={e => setProfile({ ...profile, enabled: e.target.value === 'on' })}><option value="on">启用</option><option value="off">停用</option></select></Field>
      <Field label="机器人输入提示" wide><textarea rows={2} value={profile.prompt} onChange={e => setProfile({ ...profile, prompt: e.target.value })} required /></Field>
      <div className="subheading col-span-full"><h3>HTTP 请求</h3><p>可用变量：<code>{'{{credential}}'}</code>、<code>{'{{telegram_user_id}}'}</code>、<code>{'{{telegram_username}}'}</code></p></div>
      <Field label="请求方法"><select value={profile.request.method} onChange={e => setProfile({ ...profile, request: { ...profile.request, method: e.target.value as 'GET' | 'POST' } })}><option>GET</option><option>POST</option></select></Field>
      <Field label="请求地址" help="使用固定 HTTP(S) 主机，变量可放在路径或查询参数中。"><input type="url" value={profile.request.url} onChange={e => setProfile({ ...profile, request: { ...profile.request, url: e.target.value } })} placeholder="https://your-service.example/verify" required /></Field>
      <Field label="请求头 · JSON 对象" help="例如 Authorization。配置含密钥，仅管理员可读取。"><textarea className="code-input" rows={5} value={headers} onChange={e => setHeaders(e.target.value)} spellCheck={false} /></Field>
      <Field label="Query 参数 · JSON 对象"><textarea className="code-input" rows={5} value={query} onChange={e => setQuery(e.target.value)} spellCheck={false} /></Field>
      <Field label="JSON 请求正文" help="POST 可填写；变量写在 JSON 字符串值中，留空表示不发送正文。" wide><textarea className="code-input" rows={5} value={body} onChange={e => setBody(e.target.value)} placeholder={'{\n  "credential": "{{credential}}"\n}'} spellCheck={false} /></Field>
      <div className="subheading col-span-full"><h3>通过条件</h3><p>所有条件均需满足。字段路径采用 JSON Pointer，例如 <code>/data/valid</code>；期望值填写 JSON 标量。</p></div>
      <div className="col-span-full rule-list">{rules.map((rule, index) => <div className="rule-row" key={index}><Field label={`字段路径 ${index + 1}`}><input value={rule.path} onChange={e => setRules(rules.map((item, i) => i === index ? { ...item, path: e.target.value } : item))} placeholder="/data/valid" /></Field><Field label="期望值"><input className="mono" value={rule.expected} onChange={e => setRules(rules.map((item, i) => i === index ? { ...item, expected: e.target.value } : item))} placeholder='true / 0 / "ok"' required /></Field><button type="button" disabled={rules.length === 1} onClick={() => setRules(rules.filter((_, i) => i !== index))}>删除</button></div>)}<button type="button" onClick={() => setRules([...rules, { path: '', expected: 'true' }])}>增加条件</button></div>
      <Field label="外部身份 ID 字段" help="可选，仅用于后台展示。"><input value={profile.request.identityPath || ''} onChange={e => setProfile({ ...profile, request: { ...profile.request, identityPath: e.target.value } })} placeholder="/data/id" /></Field>
      <Field label="显示名称字段" help="可选，不限制业务身份复用。"><input value={profile.request.displayNamePath || ''} onChange={e => setProfile({ ...profile, request: { ...profile.request, displayNamePath: e.target.value } })} placeholder="/data/name" /></Field>
      <fieldset className="col-span-full"><legend>通过后可加入的群组／频道</legend>{chats.length ? <div className="check-grid">{chats.map(chat => <label className="check-item" key={chat.id}><input type="checkbox" checked={profile.chatIds.includes(chat.id)} onChange={e => setProfile({ ...profile, chatIds: e.target.checked ? [...profile.chatIds, chat.id] : profile.chatIds.filter(id => id !== chat.id) })} /><span>{chat.title || chat.id}<small className="mono">{chat.id}{chat.enabled ? '' : ' · 已停用'}</small></span></label>)}</div> : <p className="hint">先到“群组与频道”添加群组。</p>}</fieldset>
      <div className="form-actions col-span-full"><button className="primary" disabled={busy}>保存配置</button>{initial.id && <button type="button" className="danger-quiet" disabled={busy} onClick={() => { if (window.confirm(`删除核验配置“${initial.name}”？关联资格和邀请将失去准入效力。`)) void action.run(async () => { await post('/profiles/delete', { id: initial.id }); await onDeleted(); }); }}>删除配置</button>}</div>
    </form>
    </details>
    <div className="test-area"><h3>测试当前配置</h3><p className="hint">使用未保存的当前配置调用接口；测试凭据不保存，不授予入群资格。</p><div className="form-grid"><Field label="测试凭据"><input type="password" autoComplete="off" value={credential} onChange={e => setCredential(e.target.value)} /></Field><Field label="Telegram 用户 ID"><input inputMode="numeric" value={testUserID} onChange={e => setTestUserID(e.target.value)} placeholder="测试身份的数字 ID" /></Field><Field label="Telegram 用户名 · 可选"><input value={testUsername} onChange={e => setTestUsername(e.target.value)} /></Field><div className="form-actions"><button disabled={busy || !credential} onClick={() => void testAction.run(async () => { setResult(undefined); try { setResult(await post<VerificationTestResult>('/profiles/test', { profile: value(), credential, userId: telegramID(testUserID), username: testUsername })); } finally { setCredential(''); } })}>{testAction.busy ? '处理中…' : '执行测试'}</button></div></div>
      <Notice error={testAction.error} />
      {result && <VerificationTestDetails result={result} />}
    </div>
  </Section>;
}

function headerLines(headers: Record<string, string[]>) {
  return Object.entries(headers).flatMap(([name, values]) => values.map(value => `${name}: ${value}`)).join('\n');
}

export function VerificationTestDetails({ result }: { result: VerificationTestResult }) {
  return <div className="test-result">
    <div className={`notice ${result.passed ? 'success' : 'error'}`} role="status">
      <strong>{result.passed ? '验证通过' : '验证未通过'}</strong><span> · {result.durationMs} ms</span>
      <p>{result.message}</p>
      {result.externalId && <span>身份 ID：{result.externalId}</span>}{result.displayName && <span> · 名称：{result.displayName}</span>}
    </div>
    <div className="test-exchange">
      <div><h3>请求信息</h3><pre className="test-payload" tabIndex={0} aria-label="请求信息">{`${result.request.method} ${result.request.url}\n\n${headerLines(result.request.headers)}\n\n${result.request.body}`}</pre></div>
      <div><h3>响应信息{result.response && <span> · HTTP {result.response.statusCode}</span>}</h3>
        {result.response ? <><pre className="test-payload" tabIndex={0} aria-label="响应信息">{`HTTP ${result.response.statusCode}\n\n${headerLines(result.response.headers)}\n\n${result.response.body}`}</pre>{result.response.truncated && <p className="hint">响应正文超过 1 MiB，已截断。</p>}</> : <p className="hint">未收到响应。</p>}
      </div>
    </div>
  </div>;
}
