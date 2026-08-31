# cpa-usage-keeper 源码调研附录（功能点底账）

> 本文档是 PRD 的支撑材料，记录对标项目 Keeper（v1.15.0）的原始功能点清单，
> 供后续开发对照「哪些已复刻、哪些已裁剪、哪些待做」。
> 源码：`/tmp/keeper-src`（depth-1 clone，2026-08-31）。

## 1. Keeper 完整 API 端点清单（internal/api）

按文件分组：

| 文件 | 端点 |
|---|---|
| auth | POST /login、POST /logout、POST /api-key-login、GET /session |
| auth_sessions | GET/PATCH/DELETE /auth/sessions[/:id] |
| auth_files | DELETE /auth-files、PATCH /auth-files/status |
| cpa_api_keys | GET /usage/api-keys、/usage/api-keys/options、/usage/api-keys/settings、PATCH /usage/api-keys/:id |
| pricing / pricing_rules | GET/PUT/DELETE /pricing、PUT /pricing/:model、PUT /pricing/batch、GET/PUT /pricing/rules、GET /pricing/sync/preview |
| quota | GET/POST /quota/inspection、POST /quota/refresh、GET /quota/refresh/:auth_index、POST /quota/reset、GET /quota/reset-credits/:auth_index、POST /quota/cache、GET /quota/history/:auth_index、GET/PUT /quota/auto-refresh/settings |
| usage_overview | GET /usage/overview、GET /usage/overview/realtime |
| usage_activity | GET /usage/activity、GET /key-activity |
| usage_analysis | GET /usage/analysis、/usage/analysis/latency、/key-analysis、/key-analysis/latency |
| usage_events | GET /usage/events、/usage/events/export、/usage/events/filters/models、/usage/events/filters/sources、/usage/events/:id/request-log、/request-log/download-file、POST /request-log/download-token |
| usage_identities | GET /usage/identities、/usage/identities/page、/usage/identities/:id/errors、PATCH /usage/identities/:id |
| 其他 | GET /healthz、/ping、/status、/version、/update/check、/models/used、/client-ip、/panic-review |

**本插件映射**：认证/更新/排行相关端点 → 裁剪（挂靠 CPA 鉴权）。已在 M2-M6 需要对齐的：
`/usage/analysis`、`/key-analysis`、`/usage/events`（含 export/request-log）、`/usage/identities`、
`/quota`（只读）、`/pricing`（已做）。

## 2. Keeper 数据表清单（SQLite，internal/repository/migration）

- `usage_events` — 事件主表（含 ts/provider/model/alias/api_key/auth/source/latency/ttft/failed/tokens 明细）
- `usage_events_archive` — 90 天冷归档表（每日 04:30 迁移，正常查询不扫）
- `usage_identities` / `usage_identities_normalized` — 凭证身份聚合（按 auth_index 归并的用量维度）
- `usage_overview_hourly_stats` / `usage_overview_daily_stats` — 预聚合表（支撑大量 spot）
- `usage_overview_aggregation_checkpoints` — 聚合进度检查点
- `model_price_settings` — 模型定价
- `auth_files` / `auth_sessions` / `cpa_api_keys` / `provider_metadata` — 凭证与元数据快照
- `redis_usage_inboxes` — Redis 采集收件箱
- `snapshot_runs` / `schema_migrations` — 运维表

**本插件映射**：首期只用 `usage_events + model_pricing + auth_files + poll_stats`。
预聚合/归档/身份聚合是 Keeper 撑大流量的关键，视数据量增长二期补齐。

## 3. Keeper 前端结构（web/src，311 文件）

页面（pages/）：
- `LoginPage` — 登录（裁剪：挂靠 CPA 鉴权）
- `UsagePage` — 概览主页面（2300 行，含 6 个 tab 的容器 + 工具条 + 统计卡 + 活动热图 + 实时指标 + 份额）
- `KeyOverviewPage` — 凭证概览（auth files 列表 + 配额）
- `KeyAnalysisPage` — 按 key 分析
- `KeyRankingPage` — 社区排行（裁剪）

usage 组件（components/usage/）：
- `StatCards` — 7 张统计卡（含 Daily Average、sparkline、accent、icon badge）
- `ActivityHeatmapGrid` — Token 活动热图（GitHub 贡献图）
- `OverviewRealtimePanel` — 实时指标面板（window 切换 + 5 指标）
- `RecentActivityPanel` — 近期活动容器
- `RequestEventsDetailsCard` — 请求事件表（列配置 + 详情 + 导出）
- `DailyAverageCard` / `ServiceHealthCard` / `CustomRangePanel` / `TimeRangeControl`
- `ApiKeySettingsCard` / `SessionSettingsCard` / `PriceSettingsCard`
- `credentials/` — 凭证区（AuthFileCredentialsSection、quota inspection、Codex quota history 等）
- `analysis/`、`pricing/`、`hooks/`

样式体系（styles/）：`themes.scss`（三主题 white/dark/light，生产用 white）、`variables.scss`、
`components.scss`（卡片/徽章/按钮等）、`layout.scss`。

**关键视觉规格**（themes.scss + components.scss 实测值）：
- white 主题：bg `#ffffff`、卡片 `#ffffff`、hover `#f6f6f6`、border `#e5e5e5`
- 主色 `#8b8680`（灰棕），成功 `#10b981`，错误 `#c65746`
- 卡片圆角 24px、padding 20px、卡标题 18px/700、正文 12px
- 统计卡：padding 18px、min-height 176px、顶部 3px accent 渐变细条、34×34 圆角 icon badge、
  大数字 28px（主卡 32px）weight 800、hover 上浮 -2px + 加深阴影

## 4. Keeper 采集/同步机制（internal/poller）

- `redis_pull_source` — RESP LPOP `usage` key（连 CPA 自带 8317 RESP 端点，AUTH management key）
- `redis_subscribe_source` — RESP SUBSCRIBE 模式（多 collector 共存时用，非破坏性）
- `http_pull_source` — 走 CPA 管理端点 `GET /v0/management/usage-queue?count=N`（LPOP 同款，需管理密钥）
- 同步：poll CPA `/v0/management/auth-files`（+ status PATCH）、`/v1/models`、api-keys 等；排名走社区服务器。

**本插件差异**：不用上述任何轮询做用量采集，改 `UsagePlugin` 事件推送（同源、非破坏性、更快）。
凭证同步（auth-files）整体不做——凭证页已裁剪，插件零密钥配置。

## 5. 裁剪映射总表

| Keeper 功能 | 决策 | 备注 |
|---|---|---|
| 独立登录/账号/session | 裁剪 | 挂靠 CPA 管理密钥 |
| 社区排行 Ranking | 裁剪 | 依赖 Keeper 社区服务器 |
| i18n 多语言 | 裁剪 | 文案 zh-CN 写死 |
| update check | 裁剪 | 无独立发版流 |
| SQLite 定时备份/日志运维 | 裁剪 | 数据在 CPA 进程内 |
| 凭证页（auth files 列表 / AI 提供商 / 配额） | 裁剪 | 聚焦用量分析，不做凭证健康管理（用户确认） |
| 配额主动刷新/重置 | 裁剪 | 各省份上游 API 重活 |
| 主题切换（3 态） | 裁剪（浅色固定） | 用户确认先做浅色 |
| 预聚合表/90 天归档 | 二期 | 视数据量评估 |
| API Key 下拉筛选 | 完整对齐 | 见 PRD 4.1.1（不裁剪） |
| request-log 详情/导出 | 二期 | M2 事件表先做主体 |
| P50/P95 延迟诊断 | 二期 | M3 先做均值 |