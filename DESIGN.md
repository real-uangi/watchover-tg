# Watchover TG 系统设计

## 定位与技术栈

单实例、单管理员、单 Telegram Bot 管理平台。采用 Go 1.26、allingo v0.1.14、Fx v1.24、Gin、GORM/PostgreSQL，eventbus固定v0.0.0-20261111004406-7eee93697032。后台使用React 19/TypeScript/Vite/Tailwind 4/pnpm，同源嵌入Go。

入口通过 app.Current().Option(...).Run() 装配公共、数据库、KV/task、shared、auth、bot及适配器。实际约定参见 AGENTS.md，接口见 docs/API.md；并行实现的精确类型与端口见 docs/CONTRACT.md。

## 分层与数据

仅auth与bot两域，域内application/domain/infra。HTTP/Webhook/event/schedule适配器调用应用用例；domain定义业务模型及Store/Telegram/Verifier/Publisher端口；infra实现ORM、密文、外部HTTP。ORM与领域模型分离，不建立通用repository/插件/规则引擎。shared只负责进程配置、事件总线。

PostgreSQL持久化单例设置、核验配置与群关联、Telegram用户/群/成员、用户资格、等待输入会话、邀请、消息、完成收据。Token/Webhook secret和核验请求配置AES-GCM加密，AAD区分资源。Bot Token不回显；请求配置仅已认证管理员可读取编辑。用户凭据及原始Update永不持久化或记录；凭据消息仅保存固定敏感占位。

用户按Telegram数字ID识别，不以username鉴权；用户+核验配置唯一资格，不限制凭据复用。当前启用的核验配置、当前群关联和群启用状态决定后续准入；配置变化不自动清退现有成员。管理员可撤销资格并另行移出用户。BotID首次确定后不允许用不同机器人复用当前数据，同Bot轮换Token可用。

消息(chat_id,telegram_message_id)按非零Telegram消息ID唯一；unknown发送无Telegram ID，各有独立本地ID。敏感标记不可逆，编辑不能回填明文或重新引入已清理消息。按事件时间与update ID避免旧编辑/成员状态覆盖新状态；出站结果与频道回显合并。只展示接入后已获得的成员/消息，不声称完整名册和历史同步。

## 核验与邀请

每套核验配置含提示、GET/POST URL、headers/query/JSON body、多个JSON Pointer严格标量等值规则（AND）、可选外部ID/展示名路径、关联chatIDs。仅允许http(s)固定scheme/host，用户变量只作用于path/query/header/JSON值；JSON先解析再替换并marshal。变量credential、telegram_user_id、telegram_username；对外HTTP禁redirect、自动重试，错误不可泄露原始URL/请求/响应。

管理员显式测试由TestProfile调用Verifier.Test，与普通Verify共用请求构造和核验逻辑，仅测试路径收集诊断。测试完整返回变量替换后的请求、外部响应、耗时和安全结果说明，可包含测试凭据和外部接口密钥；仅在本次请求及当前页面内存流转，不落库或日志、不授予资格。上游非2xx、网络失败/超时、读取失败、非JSON、规则不符或身份字段解析失败均以HTTP 200返回测试结果，配置无效、管理员认证或Origin错误沿用原状态。单次调用限10秒，响应正文最多1MiB，超限展示截断内容并判为未通过；普通Webhook核验继续使用安全错误映射。后台已有配置默认折叠，新建配置默认展开；折叠保留未保存内容，测试区独立常显并使用当前表单值。

/start分别发送配置的欢迎文案和核验/可加入群菜单，两条回复不保证先后顺序；欢迎为空时只发菜单，核验成功后不再发送欢迎。选择配置进入15分钟等待状态，/cancel取消。凭据只在入站事件和后台处理内存中核验。2xx+合法JSON+全部条件满足才保存grant，业务拒绝与服务故障分开反馈。资格不自动到期。选群在后台处理中同步创建或复用30分钟审批邀请，保存与指定用户、群、配置的映射后才发布提示。chat_join_request重新检查当前资格和映射再approve，错人使用本平台链接则decline，其他管理员邀请不自动处理。

新生成的菜单隐藏有效资格对应的验证项，资格撤销后恢复显示。仅对已授权且当前可邀请的群实时调用getChatMember，每次菜单同群最多查询一次；member、administrator、creator及is_member=true的restricted用户不再显示加入项。全部成员查询共用10秒子context，查询失败或预算耗尽时保留加入项，不向用户提示、不自动重试；失败仅记录安全日志，不因查询失败回滚验证资格。数据库操作继续使用原context。查询结果仅用于本次菜单，不写成员表、不定时刷新、不修改历史菜单。无按钮时显示“当前没有待完成的验证或可加入的群组。”。

## Webhook与副作用

仅POST /api/v1/telegram/webhook，验证secret header。注册max_connections=1，显式订阅message/edited_message/channel_post/edited_channel_post/callback_query/my_chat_member/chat_member/chat_join_request；注册注销均drop_pending_updates=false，退出不注销。

Webhook调用ReceiveUpdate，完成JSON、1MiB大小限制和secret校验后发布解析后的Update；该入口读取设置但不获取业务处理锁。成功入队即返回200，不等待业务事务、核验或Telegram操作。eventbus队列满时最多等待5秒，入队失败返回503。200仅表示内存接收，后续失败不自动重试，退出时未完成的事件可能丢失。

入站使用Fx命名updates总线，1个dispatcher、1个executor，两级队列各64；固定订阅者使用独立于HTTP请求的60秒context调用HandleUpdate。单实例mutex串行执行查收据、业务用例、业务事实与收据同事务提交；同一把锁保护后台配置、资格和用户资料写入，避免撤销/删除与正在核验、审批相互覆盖。已处理update跳过；后台数据库/处理错误不写完成收据，只记录Update ID和安全错误，不再影响已返回的HTTP响应。核验网络失败等安全业务结果仍作为一次处理提交，由用户主动再试。禁止仅用last_update_id高水位去重。

事务提交后逐条发布Reply到独立的出站eventbus，由两个executor并发处理，单条回复使用10秒context。分开两条总线，避免入站处理发布回复时向同一满队列等待，以及关闭入站时拒绝派生回复。入站事件只带domain.Update，不带HTTP context、secret或Bot Token，可在内存包含用户凭据；出站事件只含安全提示与群列表/已存邀请，不带凭据。原始Update及凭据均不得落库或日志。提示丢失可用/start或再次点击恢复，发布失败不回滚已提交授权。外部API与DB无法原子提交，不承诺exactly-once；后台发送和成员操作同步执行，不自动重试，超时/连接中断明确为结果未知。

## HTTP、附件与生命周期

管理员使用环境口令、12小时内存opaque Cookie会话，HttpOnly、SameSite=Lax、生产Secure；退出/重启失效。管理POST（含login/logout）验证Origin；API no-store，实际HTTP状态与code相同，未知API不回落SPA。

文本/图片/文档支持发送及回复；私聊仅针对已建立会话的用户。媒体只保存Telegram文件标识/元数据，经已认证的同源接口流式代理，不给浏览器Bot URL；文档强制attachment+nosniff。下载受官方20MB限制，上传图片10MB/文档50MB；不引入本地Bot API。保留期限仅影响本平台记录，不删除Telegram原件。

环境变量仅DB_DSN、WATCHOVER_PUBLIC_URL、WATCHOVER_ADMIN_PASSWORD、WATCHOVER_ENCRYPTION_KEY及allingo运行参数。默认30天消息保留，后台可改正整数。allingo task配本地KV，每天Asia/Shanghai 03:00读取最新配置并删除过期消息，完成收据固定7天清理。无Redis依赖。

Fx依赖顺序保证数据库初始化后启用业务与HTTP；退出先停止HTTP，再停止任务并取消/等待在途清理，随后依次关闭并排空入站、出站eventbus，最后关闭DB。allingo task无公开Stop API，晚到callback通过门禁拒绝业务执行。保留Fx默认15秒停机期限，不承诺积压事件全部处理完毕；eventbus Close超时不取消handler，handler自行使用有界context。

## 验证与交付边界

只运行类型、单元/必要竞态、vet和编译。模拟RoundTripper/recorder及fake，不运行dev server、真实DB、监听型mock或真实Telegram。fx.ValidateApp验证装配，不Start生产模块。真实Postgres约束/迁移、反向代理、浏览器现场和Telegram联调需另行验收。源码、文档、Dockerfile和CI配置均须按实际执行结果报告，commit/push/release/deploy分别授权。

## 构建与发布

Checks 面向 main/PR，且通过 workflow_call 供 Release 复用，仅有只读权限。正式 Release 由稳定标签 vX.Y.Z 的 push 触发；门禁严格校验标签格式，并验证解引用后的 commit 属于 origin/main 历史。发布不要求标签必须指向 main 顶端，检查和产物都应基于该标签的同一 commit。

GoReleaser OSS 固定 v2.15.2，先构建嵌入的Web，再以CGO_ENABLED=0编译Linux amd64/arm64静态程序。两个架构tar.gz名称保留完整v标签，归档仅含程序和.env.example，另提供SHA256校验清单。运行时密钥和.env不进入发布文件。

原有Dockerfile继续支持从源码构建；Dockerfile.release专供dockers_v2/Buildx包装GoReleaser预编译产物，保持gcr.io/distroless/static-debian12:nonroot运行时，不重复编译、不使用QEMU运行程序探测版本。GHCR目标为ghcr.io/real-uangi/watchover-tg:<tag>与latest，发布Linux双架构manifest。包按public分发，首次发布后单独确认可见性及匿名拉取。latest语义为最近成功镜像发布，不保证最高版本或main最新代码。

Release gate及Checks保持只读；仅publish job使用GITHUB_TOKEN的contents/packages write权限。发布不依赖运行时数据库、管理员口令、Bot Token或主加密密钥。正式Release、归档校验、GHCR公开及匿名拉取的完成状态需由实际CI和产物证据确认，不能从配置文件推定。
