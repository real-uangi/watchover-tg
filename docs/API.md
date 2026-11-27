# Watchover TG HTTP API

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
- POST /api/v1/telegram/webhook Telegram Update，secret header；JSON、大小和secret校验通过且成功进入内存队列后返回200，不等待后台业务处理。

## 数据类型

后端权威定义 internal/bot/domain/bot.go；JSON标签为前后端契约，ID数字 Telegram IDs 用number，数据库int64。Profile.request.body为JSON值（可省略），Rule.expected为JSON标量。GET lists无结果返回items:[]。

VerificationTestResult包含`passed:boolean`、可选`externalId:string`与`displayName:string`，以及：

- `message:string`：本次核验结果或失败原因的安全说明；`durationMs:number`：耗时毫秒。
- `request:{method:string,url:string,headers:Record<string,string[]>,body:string}`：变量替换后的本次请求，正文为文本，头字段保留多值。
- `response:{statusCode:number,headers:Record<string,string[]>,body:string,truncated:boolean}|null`：已收到的响应；无响应时为null。正文最多1MiB，超限保留截断内容并标记truncated，核验不通过。

测试使用当前提交的配置（可未保存），不授予资格。仅管理员显式测试完整返回请求响应，含测试凭据和外部接口密钥；详情只保留在本次请求与当前页面内存，不写数据库或日志。每次测试仅调用一次外部接口，10秒超时、不跟随重定向、不自动重试。

## 状态与安全

- 200：应用操作成功；Webhook仅表示Update已进入内存队列，不表示业务已处理或持久化。核验测试是否通过由data.passed决定；上游非2xx、网络失败/超时、正文读取失败、非JSON、规则不符及身份字段解析失败均返回200，具体原因见data.message及请求响应详情。
- 400：字段或配置错误；401：管理员会话无效；403：来源、Webhook secret或操作权限不满足；404：资源不存在；409：配置/身份冲突。
- 413：正文或上传超限；415：JSON媒体类型不符；502：外部操作结果未知；503：外部服务或存储不可用；500：未分类内部故障。502可能附带消息记录，调用方不得自动重发。
- 响应包含code、message、data、time。HTTP状态与code一致。错误不含原始网络URL、Token或数据库消息。
- 管理写请求（包括登录）必须带与WATCHOVER_PUBLIC_URL匹配的Origin。所有认证状态和密钥仅在当前页面内存中，不使用localStorage。
- Cookie watchover_session，Path=/api/v1/admin，HttpOnly，SameSite=Lax，HTTPS Secure，12小时有效。登录后获取的核验请求配置仅用于管理员编辑；Bot Token和Webhook secret不出现在读取响应。
- webhook需X-Telegram-Bot-Api-Secret-Token，更新上限1MiB。管理JSON正文上限64KiB；上传总正文上限51MiB。
- Webhook入队失败返回503；队列满时最多等待5秒。入队后后台处理失败不自动重试，退出时未完成的事件可能丢失；凭据与原始Update只在内存流转，不写入日志或数据库。
- File附件按本地消息记录ID鉴权、检查保留期限后代理，不支持客户端直接传file_id或任意URL。文档强制附件下载；图片预览仅返回安全图片媒体类型。

## 最小调用示例

```sh
curl --cookie-jar cookies.txt \
  -H 'Origin: https://watchover.example.com' \
  -H 'Content-Type: application/json' \
  --data '{"password":"REPLACE_ME"}' \
  https://watchover.example.com/api/v1/admin/auth/login
curl --cookie cookies.txt https://watchover.example.com/api/v1/admin/settings
```

群组和用户操作使用数字Telegram ID；本地消息ID/配置ID为UUID。消息页游标是服务端生成的不透明字符串，原样传回；用户列表也使用返回的nextCursor分页。空列表统一items:[]。
