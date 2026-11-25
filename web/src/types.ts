export interface Settings { botId: number; username: string; hasToken: boolean; welcome: string; retentionDays: number }
export interface TelegramUser { id: number; username: string; first_name: string }
export interface WebhookInfo { url: string; pending_update_count: number; last_error_message: string; last_error_date: number }
export interface Rule { path: string; expected: string | number | boolean | null }
export interface RequestConfig { method: 'GET' | 'POST'; url: string; headers: Record<string, string>; query: Record<string, string>; body?: unknown; rules: Rule[]; identityPath?: string; displayNamePath?: string }
export interface Profile { id: string; name: string; enabled: boolean; prompt: string; request: RequestConfig; chatIds: number[] }
export interface VerificationResult { passed: boolean; externalId?: string; displayName?: string }
export interface Chat { id: number; type: string; title: string; username: string; enabled: boolean; botStatus: string; canInvite: boolean; canRestrict: boolean; canPost: boolean }
export interface User { id: number; username: string; firstName: string; lastName: string; note: string; privateChat: boolean; updatedAt: string }
export interface Grant { userId: number; profileId: string; active: boolean; externalId: string; displayName: string; verifiedAt: string }
export interface Member { chatId: number; userId: number; status: string; untilDate: number; updatedAt: string }
export interface Invite { id: string; userId: number; chatId: number; profileId: string; link: string; expiresAt: string; status: string }
export interface Message { id: string; chatId: number; telegramMessageId: number; senderId?: number; senderName: string; kind: string; text: string; fileId?: string; fileName?: string; mimeType?: string; fileSize?: number; direction: string; status: string; replyToMessageId?: number; date: string; editedAt?: string; sensitive: boolean }
export interface Page<T> { items: T[]; nextCursor: string }
export interface UserDetail { user: User; grants: Grant[]; members: Member[] }
