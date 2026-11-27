# 实施契约（模块联调）

API 返回 `{code:number,message:string,data:...}`；失败 data=null。认证 cookie；管理 POST 均需同源 Origin。GET 不带凭据；管理读取 no-store。

## HTTP

- POST /api/v1/admin/auth/login `{password}` -> `{authenticated,expiresAt}`；GET /auth/session；POST /auth/logout。
- GET /api/v1/admin/settings -> Settings；POST /settings/save SaveSettings -> Settings。Token 省略保留。
- POST /settings/check `{}` -> TelegramUser；GET /webhook/info -> WebhookInfo；POST /webhook/register、/webhook/remove `{}`。
- GET /profiles/list -> `{items:Profile[]}`；POST /profiles/save Profile -> Profile（新建id空）；POST /profiles/delete `{id}`；POST /profiles/test `{profile:Profile,credential:string,userId:number,username:string}` -> VerificationTestResult。
- GET /chats/list -> `{items:Chat[]}`；POST /chats/add `{id:number}` -> Chat（联网获取信息并检查bot权限）；POST /chats/save `{id,enabled}` -> Chat；POST /chats/check `{id}` -> Chat。
- GET /chats/members?chatId= -> `{items:Member[]}`；POST /members/action MemberAction -> Member。action=ban|unban|kick|mute|unmute|refresh，untilDate Unix秒，0永久。
- GET /chats/invites?chatId= -> `{items:Invite[]}`，后台不提供公开分享本应绑定用户的链接动作。
- GET /users/list?q=&cursor= -> UserPage（50条）；GET /users/detail?id= -> UserDetail；POST /users/note `{id,note}` -> User；POST /users/revoke `{userId,profileId}`。
- GET /messages/list?chatId=&cursor= -> MessagePage（50条，时间倒序）；POST /messages/send multipart/form-data，chatId、kind=text|photo|document、text、replyToMessageId（可选），file（媒体必需） -> Message。
- GET /messages/file?id=消息记录ID -> 认证媒体流。图片预览；其他文件attachment+nosniff。超20MB官方无法下载，返回安全错误。
- POST /api/v1/telegram/webhook Telegram Update，secret header；JSON、大小和secret校验通过且成功进入内存队列后返回200，不等待后台业务处理。入队失败503；队列满时最多等待5秒，后续处理失败不自动重试，退出时未完成的事件可能丢失。

## 数据类型

后端权威定义 internal/bot/domain/bot.go；JSON标签为前后端契约，ID数字 Telegram IDs 用number，数据库int64。Profile.request.body为JSON值（可省略），Rule.expected为JSON标量。GET lists无结果返回items:[]。

VerificationTestResult嵌入VerificationResult（passed、可选externalId/displayName），另含message:string、durationMs:number、request:{method:string,url:string,headers:Record<string,string[]>,body:string}、response:{statusCode:number,headers:Record<string,string[]>,body:string,truncated:boolean}|null。请求为变量替换后的最终内容，未收到响应时response=null。单次外部请求限10秒、不跟随重定向、不自动重试；响应正文最多1MiB，超限保留截断内容并判为未通过。

管理员显式测试使用当前提交的配置，不授予资格；完整请求响应可含测试凭据与外部接口密钥，仅在本次请求与当前页面内存中流转，不落库或日志。上游非2xx、网络失败/超时、读取失败、非JSON、规则不符和身份字段解析失败均为HTTP 200测试结果；配置无效、认证和Origin错误仍走原错误状态。普通Webhook核验继续安全映射错误。

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
TestProfile(ctx,Profile,VerificationInput)(VerificationTestResult,error)
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
ReceiveUpdate(ctx,secret string,Update)error
HandleUpdate(ctx,Update)error
DeliverReply(ctx,Reply)error
Cleanup(ctx)error

## infra 构造约定

NewStore(manager *db.Manager,cipher *Cipher)*Store 实现 domain.Store；NewCipher(cfg *config.Config)(*Cipher,error)；NewTelegram() domain.Telegram；NewVerifier() domain.Verifier。
domain.Verifier提供Validate(RequestConfig)error、Verify(ctx,RequestConfig,VerificationInput)(VerificationResult,error)和Test(ctx,RequestConfig,VerificationInput)(VerificationTestResult,error)；TestProfile校验输入后调用Test，仅Test收集管理员诊断，普通Verify保持原核验及安全错误契约。
Bootstrap 由infra实现并声明Fx lifecycle：迁移ORM+初始化Settings（欢迎文案、30天、自动生成secret）；DB关闭职责参照posthaste Bootstrap。
config.Config: Origin string; SecureCookies bool; AdminPassword string; EncryptionKey []byte。
Publisher由根负责装配，提供PublishUpdate(Update)error和Publish(Reply)error。PublishUpdate向Fx命名updates入站总线发布domain.Update（UpdateTopic=watchover.bot.update）；该总线1个dispatcher、1个executor，两级队列各64。ReceiveUpdate读取设置并校验secret，不获取业务处理锁；入站固定订阅者使用独立Background、60秒context调用HandleUpdate，保留业务锁及业务事实/完成收据同事务提交。

Publish继续向默认出站总线发布单条domain.Reply（ReplyTopic=watchover.bot.reply），两个executor、单条10秒context；回复不保证顺序。两总线分别关闭，正常顺序为HTTP、入站、出站、DB，保持Fx默认15秒停机期限。入站载荷不带HTTP context、secret或Bot Token，可在内存包含用户凭据；出站载荷不含凭据。原始Update和凭据不得落库或日志，无额外领域事件框架或持久队列。
