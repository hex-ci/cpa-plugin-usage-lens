# usage-lens（用量透镜）· 产品需求文档（PRD）

> 目标产品：`usage-lens`，一个 CLIProxyAPI（简称 CPA）插件，在不启动独立服务的前提下，持久化并可视化用量。
> 发布目标：上架 CPA 官方插件市场（见第 9 节发布与开源规范）。
> 对标对象：`cpa-usage-keeper`（简称 Keeper），用量分析功能面照抄，前端用 Vue 3 + 脚本语法重写。
> 本文档基于对 Keeper 源码（v1.15.0）的逐模块调研整理，是后续开发的唯一需求基线。
> 语言约定：全部文案简体中文；Token 为行业通量术语保留原词，其余概念用中文表达。

---

## 1. 产品定位

本插件聚焦「用量分析」**核心功能**：请求数、Token 量、成本、缓存命中、延迟、错误率，以及按模型 / API Key 的
维度分布。取舍原则：**凡与用量统计核心无关的一律不做**——Keeper 的凭证管理（auth files 列表、配额检查、
AI 提供商健康）不做（那是凭证健康管理，且依赖各省份上游 API 的配额探测重活）。

**核心差异化：零独立服务。** Keeper 是独立服务（自起端口 + 订阅 Redis 队列 + 自带账号体系 + 单独发版运维）；
本插件跑在 CPA 进程内，通过 `UsagePlugin` 能力直接接收用量事件，数据落本地 SQLite，面板走 CPA 资源路由。
装机 = 放一个动态库 + 配置里开一个开关，**随 CPA 启停、不用守护进程、不占端口、不碰 Redis、不要密钥**。

差异化优势（相对独立服务方案）：

| 维度 | 独立服务（Keeper） | usage-lens（本插件） |
|---|---|---|
| 部署 | 独立进程 + 端口 + 常驻守护 | 动态库落地 + 配置开关，随 CPA 启停 |
| 数据采集 | Redis 队列订阅 / 轮询（独占消费） | 事件推送（同源、非破坏性、零延迟） |
| 基础设施 | Redis + SQLite + 账号体系 | 仅 SQLite |
| 密钥 | 明文管理密钥环境变量 | 零密钥配置（面板挂靠 CPA 登录态） |
| 运维 | 单独监控 / 升级 / 守护 | 与 CPA 生命周期一致 |
| 升级换代 | 独立发版 + 重启服务 | 换 .so 即生效，随 CPA 重启 |
| 共存迁移 | 独占队列，多实例冲突 | 非破坏性采集，可与现有 Keeper 并存灰度 |

已裁剪（无需再议）：凭证页全部、社区排行、独立登录、多语言、版本检查、定时备份、配额刷新、主题切换。

插件名 / ID = `usage-lens`（用量透镜）；开源仓库 `github.com/hex-ci/cpa-plugin-usage-lens`（公开、MIT，用户 hex-ci 维护）。

---

## 2. 术语表

| 术语 | 中文含义 |
|---|---|
| 用量事件 | 每次请求完成后的一条记录（一行一条请求） |
| Token 明细 | 输入 / 输出 / 推理 / 缓存读 / 缓存写，及总量 |
| 缓存命中率 | 缓存读 Token ÷（输入 Token + 缓存读 Token） |
| 每分钟请求 / 每分钟 Token | 对应余 Keeper 的 RPM / TPM |
| 首 Token 延迟 | 首个 Token 的响应时间（Keeper 叫 TTFT）；总延迟为全响应耗时 |
| 模型定价 | 模型 → 每百万 Token 输入/输出单价，用于成本估算 |
| 活动窗口 | 热图/趋势的时间聚合粒度（天=小时，周/月=天） |
| 实时窗口 | 实时指标的对比窗口（15/30/60 分钟） |

---

## 3. 信息架构

CPA 主面板左侧菜单「用量统计」进入面板，顶栏标签导航：

```
概览（默认页）
分析
请求事件
设置
```

---

## 4. 功能需求

### 4.1 概览页

**4.1.1 工具条**
- 时间范围（对齐 Keeper，5 档，完整复刻不裁剪）：
  - 滚动小时：5–24 小时，滑块选择，默认 8 小时
  - 滚动天：1–30 天，滑块选择，默认 7 天
  - 今天 / 昨天
  - 自定义：单位（小时/天）+ 起止时间选择器
  - 范围状态持久化本地（localStorage，键 `cli-proxy-usage-time-range-v1`）。
- API Key 下拉筛选（对齐 Keeper，完整复刻不裁剪）：
  - 选项 =「全部」+ 各 API Key；显示标签优先用别名，无别名用原文
  - 选中后概览/分析/事件等所有统计查询按该 Key 过滤（聚合端点统一带 `api_key` 参数）
  - 别名在设置页可编辑（写本地库），下拉实时体现；筛选只在统计类标签显示
- 刷新按钮。

**4.1.2 统计卡（7 张）**
每张卡：强调色 + 顶部 3px 渐变细条 + 34×34 圆角图标徽章 + 大数字（主卡 32px / 次卡 28px）+ 副行说明 + 迷你趋势线。

| 卡 | 主值 | 副行 | 迷你趋势序列 |
|---|---|---|---|
| 每日均值 | 日均请求 | 日均 Token / 日均成本 | 请求 |
| 总请求 | 请求数 | 成功率 | 请求 |
| 总 Token | 总量 | 缓存读 / 推理 | Token |
| 每分钟请求 | 每分钟请求数（2 位小数） | 总请求数 | 请求 |
| 每分钟 Token | 每分钟 Token 数 | 总 Token | Token |
| 缓存命中率 | 百分比 | 缓存读 / 输入 | 命中率 |
| 总成本 | 元（$） | 总 Token | 成本 |

**4.1.3 近期活动**
- 活动窗口切换：天 / 周 / 月（决定聚合粒度：小时 / 天）。
- Token 用量热图：贡献图风格，每格一个时间桶，颜色深浅按 Token 量，悬停显示明细。
- 请求健康时间线：每桶绿（成功）红（失败）堆叠条。

**4.1.4 实时指标**
- 窗口切换：15 / 30 / 60 分钟。
- 5 项指标，每项显示「现在 / 均值（今日）/ 趋势%」：Token 速率、首 Token 延迟、总延迟、请求速率、缓存水平。

**4.1.5 Token 份额**
- 维度切换：模型 / API Key。
- 每行：名称 + 占比进度条 + 百分比 + Token 数（悬停显示请求数 / 成本）。

### 4.2 分析页

- 维度（模型 / API Key）分布：占比 + 请求数 + Token + 成本，支持排序。
- 延迟诊断：按维度输出平均首 Token 延迟 / 平均总延迟。
- 成本构成：按模型成本占比。
- 本期范围：维度分布 + 延迟诊断（均值）；P50/P95 分位数列为二期。

### 4.3 请求事件

- 事件表：分页 + 可配置列（时间/模型/别名/API Key/来源/输入输出 Token/缓存/延迟/首 Token 延迟/状态）；
  列顺序与显隐持久化（localStorage）。
- 筛选：时间范围、模型、API Key、来源、结果（成功/失败）。
- 请求级详情抽屉：单条事件完整字段（上游请求/响应日志列二期）。
- 导出（CSV/JSON）列二期。

### 4.4 设置页

- **模型定价表**：列出已出现模型 + 每百万 Token 输入/输出单价，可编辑保存（写本地库）。
  成本 = Σ(输入单价×输入/1e6 + 输出单价×输出/1e6)。
- **价格同步（models.dev）**：拉取公开目录 `https://models.dev/api.json`（免费、无需密钥），
  流程为「预览 → 勾选 → 应用」：
  - 预览比对「已用模型」与目录，返回匹配到的模型 + 目录缺失的模型
  - 用户勾选后批量写入本地库（非自动覆盖，手动价优先）
  - 失败降级：12 秒超时，同步失败仅提示、不影响已有定价与成本计算
- **API Key 别名**：给每个 Key 设中文别名，供概览下拉显示。

---

## 5. 数据模型（SQLite）

```
usage_events（用量事件主表，唯一增长表）
  id INTEGER 主键自增
  ts INTEGER 非空            -- UnixMilli，所有范围查询的轴
  provider / model / alias TEXT
  api_key / auth_id / auth_index / auth_type / source TEXT
  latency_ms / ttft_ms INTEGER
  failed INTEGER, status_code INTEGER
  input_tokens / output_tokens / reasoning_tokens / cached_tokens / total_tokens INTEGER
  索引：ts, model, api_key

model_pricing（模型定价，用户可编辑，含同步来源）
  model TEXT 主键, input_price REAL, output_price REAL, source TEXT, updated_at INTEGER

api_key_aliases（API Key 别名，用户可编辑）
  api_key TEXT 主键, alias TEXT, updated_at INTEGER
```

与 Keeper 的差异：Keeper 另有身份聚合、预聚合表、90 天归档表、凭证快照表。
本插件首期不做预聚合与归档（单机量级实时查询足够）；凭证相关表整体不做。

---

## 6. 后端 API（插件管理路由）

前缀 `/v0/management/plugins/usage-keeper/api/`，均需 CPA 管理密钥（面板自动携带）。

| 方法 | 路径 | 说明 | 关键参数 |
|---|---|---|---|
| GET | /stats | 聚合总览 | start_ts,end_ts[,api_key] → 请求/Token 明细/成本/成功率/命中率/速率/延迟 |
| GET | /trend | 时间序列（迷你趋势+热图+健康线） | start_ts,end_ts,bucket → 每桶 请求/Token/缓存/失败/输入/成本 |
| GET | /realtime | 实时指标 | window(15/30/60) → 5 指标{n}×现在/均值/趋势 |
| GET | /models | 按模型聚合 | → 名称/请求/Token/输入/输出/成本 |
| GET | /keys | 按 API Key 聚合 | 同上 |
| GET | /api-keys/options | 下拉选项 | → 选项[{id,label}]，label=别名或原文 |
| PATCH | /api-keys/alias | 更新别名 | body {api_key, alias} |
| GET | /events | 事件分页 | start,end,limit,offset,model,api_key,failed → 总数+列表 |
| GET | /health | 健康 | 事件数/最近事件时间/丢弃计数 |
| GET/PUT | /pricing | 定价表读写 | PUT 批量 upsert |
| GET | /pricing/sync/preview | 价格同步预览 | 拉 models.dev 公开目录 → 匹配/未匹配 |

二期（对齐 Keeper）：`/analysis`（延迟诊断）、`/events/:id/request-log`、`/events/export`。

---

## 7. 插件架构

```
请求完成 → CPA 用量分发
         └─ 本插件 UsagePlugin(usage.handle) → 缓冲通道 → 后台工作线程落 SQLite
面板：Vite 产物 go:embed（all: 前缀）→ 资源路由 /v0/resource/plugins/usage-keeper/
```

配置仅一项：`db_path`（本地库路径）。**零密钥**——用量事件采集不需要任何密钥，
面板数据查询走 CPA 鉴权（面板同源取登录态），无后台同步任务。

关键实现约束（均已实测验证）：
- `usage.handle` 是同步回调，必须立即返回（异步落库），否则阻塞 CPA 用量分发线程。
- `go:embed` 必须用 `all:panel/dist`（否则 Vite 的共享 helper 文件被排除 → 页面白屏）。
- 面板资源路由逐条注册（embed 枚举）；入口资源带 `Menu` 字段才出现在 CPA 左侧菜单。
- 后台工作线程必须逐段 recover（panic 会崩宿主进程）。

---

## 8. 非功能需求

- 性能：单写 SQLite（WAL + busy_timeout）；`usage.handle` O(1) 返回（缓冲 + 丢弃计数）；
  聚合查询走 ts/model/api_key 索引。
- 安全：面板不渲染任何上游凭证明文；管理路由统一走 CPA 鉴权；敏感操作不在未鉴权资源路由上执行。
- 文案：全简体中文（术语 Token 保留）。
- 样式：精确对齐 Keeper 白色主题（背景 #ffffff、主色 #8b8680、卡片圆角 24px、顶部强调色渐变细条）。
- 兼容：插件与用户现有独立 Keeper 可并存（事件广播非破坏性消费，互不干扰）。

---

## 9. 发布与开源规范

- 命名：插件名/ID = `usage-lens`（用量透镜）；GitHub 仓库 `github.com/hex-ci/cpa-plugin-usage-lens`
  （公开、MIT、独立仓库，用户 hex-ci 维护，对标 model-playground 先例）。
- 上架流程：往 CPA 官方插件市场 registry（`CLIProxyAPI-Plugins-Store/registry.json`）加条目
  `{id, name, description, author, repository, version, logo, homepage, license, tags}`。
- 开源仓库基线：README（安装使用说明）、LICENSE（MIT）、logo、checksums.txt、多平台产物
  （至少 linux/amd64，macOS/Windows 按精力补）。
- Release 资产：`usage-lens_<version>_<goos>_<goarch>.zip`（zip 根目录直放动态库，不放子目录）
  + `checksums.txt`（sha256）。版本号跟随 GitHub latest release tag（可带 v 前缀）。
- 注册元数据：metadata 填全 Name / Version / Author / GitHubRepository / Logo / ConfigFields。
- 质量门槛（对齐用户偏好）：上架前元数据完整、README/logo/license 齐、checksums 校验、
  release 与 registry 版本一致、配置与面板无明文密钥、不含个人敏感数据。

---

## 10. 里程碑

| 阶段 | 内容 | 状态 |
|---|---|---|
| 里程碑 0 骨架 | 事件采集 + SQLite + 面板资源路由 + 端到端 | 完成（已上生产） |
| 里程碑 1 概览页 | 7 卡迷你趋势 + 热图 + 健康线 + 实时指标 + Token 份额 + 时间范围（5 档）+ API Key 下拉 | 已上演示版；时间范围控件 + API Key 下拉待对齐 |
| 里程碑 2 请求事件 | 事件表 + 筛选 + 详情 | 待开发 |
| 里程碑 3 分析页 | 维度分布 + 延迟诊断 | 待开发 |
| 里程碑 4 设置页 | 模型定价表（成本闭环）+ 价格同步 models.dev + API Key 别名 | 待开发 |
| 里程碑 5 二期 | 请求日志、导出、P50/P95、request-log | 待评估 |
| 里程碑 6 发布 | 上架插件市场：仓库/registry 条目/release 资产/checksums/logo/README | 待评估 |

---

## 11. 验收基线（每阶段完成的标准）

1. 沙箱（独立 CPA 实例 + mock 上游）真实请求验证：事件落库、API 返回正确、面板渲染无资源/JS 错误。
2. 生产部署后：资源路由全 200（含 helper 文件）、面板渲染预期内容、管理 API 非 401。
3. 文案全中文、样式对齐 Keeper 实测值。