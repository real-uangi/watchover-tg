export class ApiError extends Error {
  constructor(message: string, public status: number) { super(message); this.name = 'ApiError'; }
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api/v1/admin${path}`, { ...init, credentials: 'same-origin', cache: 'no-store' });
  } catch {
    throw new ApiError('网络连接失败，请检查连接后重试。', 0);
  }
  let envelope: { code: number; message: string; data: T };
  try { envelope = await response.json(); }
  catch { throw new ApiError('服务响应格式异常。', response.status); }
  if (response.status === 401) window.dispatchEvent(new Event('watchover:unauthorized'));
  if (!response.ok || envelope.code !== response.status) {
    throw new ApiError(envelope.message || '请求失败。', response.status);
  }
  return envelope.data;
}

export function post<T>(path: string, body: unknown = {}): Promise<T> {
  return request<T>(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
}
export function fileURL(id: string) { return `/api/v1/admin/messages/file?id=${encodeURIComponent(id)}`; }
export function errorText(error: unknown) { return error instanceof Error ? error.message : '操作失败，请重试。'; }
