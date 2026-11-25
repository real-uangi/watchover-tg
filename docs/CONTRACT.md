# 实施契约（模块联调）

API 返回 `{code:number,message:string,data:...}`；失败 data=null。认证 cookie；管理 POST 均需同源 Origin。GET 不带凭据；管理读取 no-store。

## HTTP

- POST /api/v1/admin/auth/login `{password}` -> `{authenticated,expiresAt}`；GET /auth/session；POST /auth/logout。
- GET /api/v1/admin/settings -> Settings；POST /settings/save SaveSettings -> Settings。Token 省略保留。
- POST /settings/check `{}` -> TelegramUser；GET /webhook/info -> WebhookInfo；POST /webhook/register、/webhook/remove `{}`。
- GET /profiles/list -> `{items:Profile[]}`；POST /profiles/save Profile -> Profile（新建id空）；POST /profiles/delete `{id}`；POST /profiles/test `{profile:Profile,credential:string,userId:number,username:string}` -> VerificationResult。
- GET /chats/list -> `{items:Chat[]}`；POST /chats/add `{id:number}` -> Chat（联网获取信息并检查bot权限）；POST /chats/save `{id,enabled}` -> Chat；POST /chats/check `{id}` -> Chat。
- GET /chats/members?chatId= -> `{items:Member[]}`；POST /members/action MemberAction -> Member。action=ban|unban|kick|mute|unmute|refresh，untilDate Unix秒，0永久。
- GET /chats/invites?chatId= -> `{items:Invite[]}`，后台不提供公开分享本应绑定用户的链接动作。
- GET /users/list?q=&cursor= -> UserPage（50条）；GET /users/detail?id= -> UserDetail；POST /users/note `{id,note}` -> User；POST /users/revoke `{userId,profileId}`。
- GET /messages/list?chatId=&cursor= -> MessagePage（50条，时间倒序）；POST /messages/send multipart/form-data，chatId、kind=text|photo|document、text、replyToMessageId（可选），file（媒体必需） -> Message。
- GET /messages/file?id=消息记录ID -> 认证媒体流。图片预览；其他文件attachment+nosniff。超20MB官方无法下载，返回安全错误。
- POST /api/v1/telegram/webhook Telegram Update，secret header，成功返回200。

## 数据类型

后端权威定义 internal/bot/domain/bot.go；JSON标签为前后端契约，ID数字 Telegram IDs 用number，数据库int64。Profile.request.body为JSON值（可省略），Rule.expected为JSON标量。GET lists无结果返回items:[]。

## 应用Service方法（上下文参数均在首位）

NewService(store domain.Store,tg domain.Telegram,verifier domain.Verifier,publisher domain.Publisher,origin string)*Service
Settings(ctx)(Settings,error)
SaveSettings(ctx,SaveSettings)(Settings,error)
CheckBot(ctx)(TelegramUser,error)
Webhook(ctx,register bool)error
WebhookInfo(ctx)(WebhookInfo,error)
Profiles(ctx)([]Profile,error)
SaveProfile(ctx,Profile)(Profile,error)
DeleteProfile(ctx,id string)error
TestProfile(ctx,Profile,VerificationInput)(VerificationResult,error)
Chats(ctx)([]Chat,error)
AddChat(ctx,id int64)(Chat,error)
SaveChat(ctx,id int64,enabled bool)(Chat,error)
CheckChat(ctx,id int64)(Chat,error)
Users(ctx,q,cursor string)(UserPage,error)
UserDetail(ctx,id int64)(UserDetail,error)
SaveNote(ctx,id int64,note string)(User,error)
Revoke(ctx,userID int64,profileID string)error
Members(ctx,chatID int64)([]Member,error)
Invites(ctx,chatID int64)([]Invite,error)
Act(ctx,MemberAction)(Member,error)
Messages(ctx,chatID int64,cursor string)(MessagePage,error)
Send(ctx,SendMessage)(Message,error)
Download(ctx,id string)(File,error)
HandleUpdate(ctx,secret string,Update)error
DeliverReply(ctx,Reply)error
Cleanup(ctx)error

## infra 构造约定

NewStore(manager *db.Manager,cipher *Cipher)*Store 实现 domain.Store；NewCipher(cfg *config.Config)(*Cipher,error)；NewTelegram() domain.Telegram；NewVerifier() domain.Verifier。
Bootstrap 由infra实现并声明Fx lifecycle：迁移ORM+初始化Settings（欢迎文案、30天、自动生成secret）；DB关闭职责参照posthaste Bootstrap。
config.Config: Origin string; SecureCookies bool; AdminPassword string; EncryptionKey []byte。
Publisher由根负责装配，通过eventbus发布domain.Reply，无额外领域事件框架。
