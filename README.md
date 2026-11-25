# Watchover TG

自托管 Telegram 机器人管理平台。一个机器人管理多个群组、频道和用户，提供可配置的 HTTP 身份核验、审批入群、成员管理及消息后台。

## 工程

Go 1.26、allingo/Fx、Gin、GORM/PostgreSQL、eventbus；React 19/TypeScript/Vite/Tailwind 4。Web 构建资源嵌入 Go，生产部署只需一个程序和外部 PostgreSQL。单实例、单管理员，仅 Webhook。

- [系统设计](DESIGN.md)
- [工程规范](AGENTS.md)
- [HTTP API](docs/API.md)
- [模块契约](docs/CONTRACT.md)

## 构建与检查

需要 Go 1.26+、Node.js 22、pnpm 10.6.3。

```sh
pnpm --dir web install --frozen-lockfile
make check
make build
```

`make check` 运行前端单测/构建、Go 竞态单测、vet 和编译；测试不启动服务器、数据库或真实 Telegram 连接。输出程序为 `bin/watchover-tg`。没有伪造的嵌入资源：Go 根包构建前必须先生成 `web/dist`。

## 部署与首次配置

准备外部 PostgreSQL 数据库和公开 HTTPS 反向代理。复制 `.env.example`，填写数据库连接、公开地址、管理员口令及一个随机 32 字节密钥的标准 Base64 值。加密密钥必须保持稳定并随数据库备份；密钥丢失后无法解密已有配置。生产 `RUN_MODE=release`，避免 allingo 在开发模式额外启用 pprof。

可运行构建后的 `bin/watchover-tg`，或从 Dockerfile 构建镜像。容器只接收必要环境变量，使用非 root 运行。HTTPS 可由反向代理终止，程序内部 HTTP 默认 8080；公网 Webhook 为 `WATCHOVER_PUBLIC_URL/api/v1/telegram/webhook`。数据库初始化在启动阶段执行。

后台首次设置顺序：

1. 使用环境中配置的管理员口令登录。
2. 在“机器人设置”填入 BotFather 创建机器人的 Token，保存后注册 Webhook。后台会保存生成的 Webhook 密钥，重启不需要重新注册。
3. 把机器人加入群组或频道并设为管理员，授予邀请权限；成员管理需限制成员权限，频道发送需发帖权限。后台可发现成员变更事件中的聊天，也可输入数字 Chat ID 接入及检查权限。
4. 新建“核验配置”，填写请求地址、方法、请求头、参数、JSON 响应条件，选择关联群组，先用测试凭据检查配置。
5. 用户私聊机器人 `/start`，选择配置，输入一个凭据，通过后选择群组并申请加入；机器人核对 Telegram ID 后审批。

已有数据绑定一个 Bot ID。同一机器人可以更换 Token；不能用另一个机器人的 Token 复用当前数据。正常停机不注销 Webhook，Telegram 仅在其保留窗口内缓存更新。管理员点击移除 Webhook 也不会丢弃积压。

## 核验配置示例

请求为固定地址，GET/POST，支持静态请求头、Query 和 JSON 正文。三个字符串变量：`{{credential}}`、`{{telegram_user_id}}`、`{{telegram_username}}`。正文先作为 JSON 解析，再替换字符串值；不执行脚本或表达式。

```json
{
  "method": "POST",
  "url": "https://account.example.com/api/verify",
  "headers": {"Authorization": "Bearer REPLACE_ME"},
  "query": {},
  "body": {"key": "{{credential}}", "telegramId": "{{telegram_user_id}}"},
  "rules": [
    {"path": "/code", "expected": 0},
    {"path": "/data/valid", "expected": true}
  ],
  "identityPath": "/data/user/id",
  "displayNamePath": "/data/user/name"
}
```

路径采用 JSON Pointer，`/data/valid` 对应嵌套字段，数组元素可用 `/items/0/id`。字段必须存在且 JSON 类型相同；数字 `0` 与字符串 `"0"` 不相等。所有条件同时满足才通过。HTTP/JSON 故障与业务拒绝分别反馈。静态请求配置加密保存，已登录管理员可读取编辑；不要把管理员账户提供给不可信人员。

## 行为边界

- 资格不过期、不限制凭据复用；撤销、禁用配置和修改群关联影响后续入群，不会自动清退。
- 审批邀请绑定用户，30 分钟过期；等待凭据状态 15 分钟过期。`/cancel` 取消输入，`/start` 可重新获取资格与群列表。
- 只有本平台生成的审批邀请自动处理；其他管理员和邀请入口独立工作，因此后台不保证全部群成员都经过本平台验证。
- 消息和成员仅展示接入后实际观察到的数据。不能通过 Bot API 拉取完整历史或成员名册；普通群升级为超级群后需在后台检查新的 Chat ID 并调整核验关联。
- 支持文本、图片、文档及回复；其他媒体显示类型摘要。发送成功表示 Telegram 接口成功，不代表已读。超时显示结果未知，不自动重发，避免重复操作。
- 图片上传限制 10 MiB、文档 50 MiB，官方文件下载限制 20 MiB；较大文件只展示元数据。文件经后台鉴权代理，浏览器不会获取 Bot Token。
- 消息默认保存 30 天，可在后台修改；每天北京时间 03:00 使用 allingo task/LocalKV 清理。保留期限删除本平台消息与附件入口，不删除 Telegram 原件。完成收据独立保存 7 天。
- 核验凭据只存在于本次请求内存，消息记录保留脱敏占位；不保存原始 Update、完整核验请求/响应。事件总线仅发送提交后的提示，提示丢失可重新操作，资格不受影响。
- Cookie 会话 12 小时有效，退出或重启失效。API no-store，管理写操作检查同源 Origin。

官方接口：[Webhook](https://core.telegram.org/bots/api#setwebhook)、[审批邀请](https://core.telegram.org/bots/api#createchatinvitelink)、[媒体下载](https://core.telegram.org/bots/api#getfile)。

## 验收状态

本仓库的默认检查属于本地代码/协议验证。真实 PostgreSQL 迁移和约束、反向代理、浏览器现场及 Telegram 权限/收发联调需要在目标环境单独验收。Dockerfile 和 CI 配置不代表镜像已构建、CI 已运行或已部署。
