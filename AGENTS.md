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

## 发布约定

- main/PR使用只读Checks，Checks通过workflow_call供Release复用。正式发布仅由严格稳定标签vX.Y.Z触发，先验证标签解引用commit属于origin/main历史，再运行同一提交的Checks和publish；不要求必须是main顶端。
- 固定GoReleaser OSS v2.15.2。先构建Web，再CGO_ENABLED=0交叉编译Linux amd64/arm64；GitHub Release提供完整v标签命名的双架构归档与SHA256清单，归档只含程序和.env.example。
- 保留原Dockerfile作为源码构建入口；Dockerfile.release仅供dockers_v2/Buildx包装预编译程序，保持Debian 12 distroless nonroot运行时，不引入QEMU或启动程序探测版本。
- GHCR使用ghcr.io/real-uangi/watchover-tg:<tag>和latest，按public分发；latest只代表最近成功镜像发布。首次发布后核实包public及匿名拉取，不能把源码公开等同镜像公开。
- 只有publish job获得GITHUB_TOKEN的contents/packages write权限；检查和发布不需要运行时密钥，镜像/归档不得包含.env、数据库连接、管理员口令、Bot Token或主加密密钥。
- 发布配置不自动授权创建标签、提交、推送或部署；按当前会话已明确授予的范围执行，并分别报告CI、发布产物、匿名拉取与目标环境验收。
