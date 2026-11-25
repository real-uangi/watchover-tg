# Watchover TG 工程规范

称呼用户为徐总。遵守用户全局 AGENTS.md。架构见 DESIGN.md，HTTP 契约见 docs/API.md。

- Go/allingo/Fx 分层单体，仅 auth、bot 两域；domain 不依赖框架，application 通过领域端口访问 IO，infra 实现数据库、加密和外部 HTTP。适配器只调用应用用例。
- 单实例、单管理员、单 Telegram Bot，仅 Webhook；不要引入多机器人、租户、插件或规则引擎。
- 业务事实与 Webhook 完成收据同事务提交；eventbus 仅承担提交后的可丢提示，不持有凭据，不充当持久队列。
- 凭据输入、原始 Update、完整核验请求响应不得落库或日志。Token 不回显。错误必须安全映射，不返回原始网络/数据库错误。
- 管理写操作检查 Origin，采用 Cookie 会话，API no-store；媒体下载必须鉴权代理，文档附件下载并 nosniff。
- 优先语义导航；无语义工具或非符号检索用限定范围 rg。最小改动、KISS，不添加假设性兜底。
- 禁止启动 dev server、真实数据库、中间件或真实 Telegram 联调。测试用模拟 RoundTripper、recorder、fake，不监听端口。Fx 仅 ValidateApp。
- 验证类型检查、单测、必要竞态、vet、构建，明确未验证的真实环境。提交、推送、发布和部署分别授权。
