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

/start展示核验和可加入群；选择配置进入15分钟等待状态，/cancel取消。凭据只在当前请求内存中核验。2xx+合法JSON+全部条件满足才保存grant，业务拒绝与服务故障分开反馈。资格不自动到期。选群同步创建或复用30分钟审批邀请，保存与指定用户、群、配置的映射后才发布提示。chat_join_request重新检查当前资格和映射再approve，错人使用本平台链接则decline，其他管理员邀请不自动处理。

## Webhook与副作用

仅POST /api/v1/telegram/webhook，验证secret header。注册max_connections=1，显式订阅message/edited_message/channel_post/edited_channel_post/callback_query/my_chat_member/chat_member/chat_join_request；注册注销均drop_pending_updates=false，退出不注销。

单实例mutex串行执行查收据、同步用例、业务事实与收据同事务提交；同一把锁保护后台配置、资格和用户资料写入，避免撤销/删除与正在核验、审批相互覆盖。已处理update直接200；DB错误5xx；核验网络失败等安全结果作为一次业务处理提交并200，由用户主动再试。禁止仅用last_update_id高水位去重。

eventbus使用固定订阅者，只发送已提交后的安全提示与群列表/已存邀请。事件不带凭据；提示丢失可用/start或再次点击恢复。发布失败不回滚已提交授权。外部API与DB无法原子提交，不承诺exactly-once；后台发送和成员操作同步执行，不自动重试，超时/连接中断明确为结果未知。

## HTTP、附件与生命周期

管理员使用环境口令、12小时内存opaque Cookie会话，HttpOnly、SameSite=Lax、生产Secure；退出/重启失效。管理POST（含login/logout）验证Origin；API no-store，实际HTTP状态与code相同，未知API不回落SPA。

文本/图片/文档支持发送及回复；私聊仅针对已建立会话的用户。媒体只保存Telegram文件标识/元数据，经已认证的同源接口流式代理，不给浏览器Bot URL；文档强制attachment+nosniff。下载受官方20MB限制，上传图片10MB/文档50MB；不引入本地Bot API。保留期限仅影响本平台记录，不删除Telegram原件。

环境变量仅DB_DSN、WATCHOVER_PUBLIC_URL、WATCHOVER_ADMIN_PASSWORD、WATCHOVER_ENCRYPTION_KEY及allingo运行参数。默认30天消息保留，后台可改正整数。allingo task配本地KV，每天Asia/Shanghai 03:00读取最新配置并删除过期消息，完成收据固定7天清理。无Redis依赖。

Fx依赖顺序保证数据库初始化后启用业务与HTTP；退出先停止HTTP，再停止任务并取消/等待在途清理、排空eventbus、关闭DB。allingo task无公开Stop API，晚到callback通过门禁拒绝业务执行。eventbus Close超时不取消handler，handler自行使用有界context。

## 验证与交付边界

只运行类型、单元/必要竞态、vet和编译。模拟RoundTripper/recorder及fake，不运行dev server、真实DB、监听型mock或真实Telegram。fx.ValidateApp验证装配，不Start生产模块。真实Postgres约束/迁移、反向代理、浏览器现场和Telegram联调需另行验收。源码、文档、Dockerfile和只读CI为本地交付，commit/push/release/deploy分别授权。
