import { useEffect, useState, type FormEvent } from 'react';
import { post, request } from './api.js';
import { Badge, Field, Loading, Notice, Section, useAction, useLoad } from './components.js';
import { formatDate } from './forms.js';
import type { Settings, TelegramUser, WebhookInfo } from './types.js';

export function SettingsPage() {
  const resource = useLoad(() => request<Settings>('/settings'), []);
  const action = useAction();
  const [token, setToken] = useState('');
  const [welcome, setWelcome] = useState('');
  const [days, setDays] = useState(30);
  const [webhook, setWebhook] = useState<WebhookInfo>();
  useEffect(() => { if (resource.data) { setWelcome(resource.data.welcome); setDays(resource.data.retentionDays); } }, [resource.data]);
  function save(event: FormEvent) {
    event.preventDefault();
    void action.run(async () => {
      const settings = await post<Settings>('/settings/save', { welcome, retentionDays: days, ...(token ? { token } : {}) });
      resource.setData(settings); setToken('');
    }, '机器人设置已保存。');
  }
  return <><Notice error={resource.error || action.error} success={action.success} />
    {resource.loading && !resource.data ? <Loading /> : resource.data && <>
      <Section title="机器人身份" description="配置一个机器人，管理多个群组和频道。" action={<Badge on={resource.data.hasToken}>{resource.data.hasToken ? '已配置' : '待配置'}</Badge>}>
        <div className="identity-strip"><div><span>BOT USERNAME</span><strong>{resource.data.username ? `@${resource.data.username}` : '尚未连接'}</strong></div><div><span>BOT ID</span><strong className="mono">{resource.data.botId || '—'}</strong></div></div>
        <form onSubmit={save} className="form-grid"><Field label="Bot Token" help={resource.data.hasToken ? '留空保留现有 Token。已保存的 Token 不会回显。' : '从 BotFather 获取 Token。'} wide><input type="password" value={token} onChange={e => setToken(e.target.value)} autoComplete="new-password" placeholder={resource.data.hasToken ? '已保存 · 留空保留' : '输入 Bot Token'} /></Field>
          <Field label="欢迎文案" wide><textarea rows={4} value={welcome} onChange={e => setWelcome(e.target.value)} required /></Field>
          <Field label="消息保留天数" help="每天清理过期记录；不会删除 Telegram 中的原始消息。"><input type="number" min={1} step={1} value={days} onChange={e => setDays(Number(e.target.value))} required /></Field>
          <div className="form-actions col-span-full"><button className="primary" disabled={action.busy}>保存设置</button><button type="button" disabled={action.busy || !resource.data.hasToken} onClick={() => void action.run(async () => { const user = await post<TelegramUser>('/settings/check'); resource.setData(current => current && ({ ...current, botId: user.id, username: user.username })); }, '机器人连接正常。')}>检查连接</button></div>
        </form>
      </Section>
      <Section title="Webhook 接入" description="服务地址需可被 Telegram 通过 HTTPS 访问。修改设置后，可在此更新注册。">
        <div className="button-row"><button disabled={action.busy || !resource.data.hasToken} onClick={() => void action.run(async () => { await post('/webhook/register'); setWebhook(await request<WebhookInfo>('/webhook/info')); }, 'Webhook 已注册。')}>注册／更新 Webhook</button><button disabled={action.busy || !resource.data.hasToken} onClick={() => void action.run(async () => setWebhook(await request<WebhookInfo>('/webhook/info')))}>读取接入状态</button><button className="danger-quiet" disabled={action.busy || !resource.data.hasToken} onClick={() => { if (window.confirm('移除 Webhook 后，平台将停止接收机器人更新。是否继续？')) void action.run(async () => { await post('/webhook/remove'); setWebhook(await request<WebhookInfo>('/webhook/info')); }, 'Webhook 已移除，积压更新保留。'); }}>移除 Webhook</button></div>
        {webhook ? <dl className="detail-grid"><div><dt>接入地址</dt><dd className="break-all mono">{webhook.url || '未注册'}</dd></div><div><dt>等待更新</dt><dd>{webhook.pending_update_count}</dd></div><div><dt>最近错误</dt><dd>{webhook.last_error_message || '无'}</dd></div><div><dt>错误时间</dt><dd>{formatDate(webhook.last_error_date)}</dd></div></dl> : <p className="hint">点击“读取接入状态”查看 Telegram 返回的当前状态。</p>}
      </Section>
    </>}
    {resource.error && <button onClick={() => void resource.reload()}>重新加载</button>}
  </>;
}
