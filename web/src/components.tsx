import { useEffect, useRef, useState, type DependencyList, type ReactNode } from 'react';
import { errorText } from './api.js';

export function Field({ label, help, children, wide = false }: { label: string; help?: string; children: ReactNode; wide?: boolean }) {
  return <label className={`field ${wide ? 'col-span-full' : ''}`}><span>{label}</span>{children}{help && <small>{help}</small>}</label>;
}
export function Notice({ error, success }: { error?: string; success?: string }) {
  return <>{error && <div className="notice error" role="alert">{error}</div>}{success && <div className="notice success" role="status">{success}</div>}</>;
}
export function Empty({ children }: { children: ReactNode }) { return <div className="empty"><span className="empty-line" />{children}</div>; }
export function Loading() { return <div className="skeleton" role="status" aria-label="正在加载"><span /><span /><span /></div>; }
export function Badge({ on, children }: { on?: boolean; children: ReactNode }) { return <span className={`badge ${on ? 'on' : ''}`}>{children}</span>; }
export function Section({ title, description, action, children }: { title: string; description?: string; action?: ReactNode; children: ReactNode }) {
  return <section className="section"><div className="section-heading"><div><h2>{title}</h2>{description && <p>{description}</p>}</div>{action}</div>{children}</section>;
}
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  async function run(work: () => Promise<unknown>, message = '') {
    setBusy(true); setError(''); setSuccess('');
    try { await work(); setSuccess(message); }
    catch (cause) { setError(errorText(cause)); }
    finally { setBusy(false); }
  }
  return { busy, error, success, run, clear: () => { setError(''); setSuccess(''); } };
}
export function useLoad<T>(load: () => Promise<T>, deps: DependencyList) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const serial = useRef(0);
  async function reload() {
    const current = ++serial.current;
    setLoading(true); setError('');
    try { const result = await load(); if (current === serial.current) setData(result); }
    catch (cause) { if (current === serial.current) setError(errorText(cause)); }
    finally { if (current === serial.current) setLoading(false); }
  }
  useEffect(() => { setData(undefined); void reload(); return () => { serial.current++; }; }, deps);
  return { data, error, loading, reload, setData };
}
