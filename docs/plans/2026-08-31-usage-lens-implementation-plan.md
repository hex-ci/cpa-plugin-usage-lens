# usage-lens 实施计划

> **目标**：把 `cpa-plugin-usage`（现名 usage-keeper）改造为可上架插件市场的 `usage-lens`，
> 并按 PRD 里程碑逐页完成功能。
> **架构**：Go 原生插件（UsagePlugin 事件采集 + modernc SQLite 落库）+ go:embed 的 Vue3+TS 面板。
> **技术栈**：Go 1.26（c-shared）、Vue 3.5 + TS 6.0.2 + Vite 8 + Tailwind 4 + pnpm 11。
> **验证方式**：后端 `go build` + `go vet`；前端 `pnpm build`；端到端沙箱（独立 CPA + mock 上游）+ playwright 渲染；
> 生产部署后资源路由 200 校验。视觉复刻以 playwright 渲染 + 实测 Keeper 为准，不做前端单测。
> **基线文档**：`docs/PRD.md`（需求）、`docs/research.md`（调研底账）。

---

## Phase 0 —— 改名 usage-keeper → usage-lens + PRD 对齐清理

改动面（前次 grep 全量清单）：

| 文件 | 改动 |
|---|---|
| `go.mod` | module → `github.com/hex-ci/cpa-plugin-usage-lens` |
| `main.go:67` | `providerName` → `usage-lens`（插件 ID，决定所有路由/配置键） |
| `main.go:203` | `GitHubRepository` → `hex-ci/cpa-plugin-usage-lens` |
| `main.go` / 各 .go | 日志前缀 `[usage-keeper]` → `[usage-lens]` |
| `Makefile:1` | `PLUGIN := usage-lens.so` |
| `panel/package.json` | name → `usage-lens-panel` |
| `panel/src/api/client.ts:7` | `API_BASE` → `/v0/management/plugins/usage-lens/api` |
| `panel/src/App.vue` 等 | 面板标题「用量统计」→「用量透镜」 |
| `README.md` | 全文重写（新名 + 新仓库地址 + 聚焦定位） |

PRD 对齐清理（凭证页已砍）：

| 文件 | 改动 |
|---|---|
| `sync.go` | 整体删除（auth-files 后台同步，凭证页已砍） |
| `config.go` | 删 `mgmt_key`/`gateway_url`/`poll_secs`/`sync_enable`，仅留 `db_path`（零密钥） |
| `store.go` | 删 `auth_files`、`poll_stats` 表 schema |
| `api.go`/`query.go` | 删 `/auth-files`、`/keys` 相关冗余端点，保留用量分析核心 |

验证：`go build` + `go vet` + `gofmt -l` 干净；沙箱端到端；生产备份旧 .so → 换新名 .so → 改 config 键 →
删旧 .so → `pm2 restart` → 新路由 200 / 旧路由 404。

---

## Phase 1 —— 里程碑 1 补完：时间范围 5 档 + API Key 下拉

**1.1 时间范围控件**（对齐 Keeper TimeRangeControl）
- 前端 `panel/src/pages/UsagePage.vue` + 新组件 `TimeRangeControl.vue`：
  滚动小时（5–24 滑块默认 8）/ 滚动天（1–30 滑块默认 7）/ 今天 / 昨天 / 自定义（单位+起止）
- localStorage 键 `cli-proxy-usage-time-range-v1`
- 后端无需改（已用通用 `start_ts`/`end_ts`，前端算出范围）

**1.2 API Key 下拉筛选**
- 后端 `query.go`：`stats`/`trend`/`realtime`/`models`/`heatmap` 加 `api_key` 可选过滤
- 后端新增 `GET /api-keys/options`（→ options[{id,label}]，label=alias||key）、`PATCH /api-keys/alias`
- 后端 `store.go` 加 `api_key_aliases` 表
- 前端：下拉组件 + 选中后所有查询带 api_key

验证：沙箱发多 key 数据 → 下拉列出 → 选中后统计只含该 key；别名生效。

---

## Phase 2 —— 里程碑 2：请求事件页

- 后端 `query.go`：`/events` 已有分页+筛选，补返回完整字段（latency/ttft/failed/status_code）
- 前端新 `EventsPage.vue`：表格 + 时间/模型/Key/结果筛选 + 详情抽屉
- 列配置持久化 localStorage

---

## Phase 3 —— 里程碑 3：分析页

- 后端新增 `GET /analysis`（维度分布 + 平均延迟 TTFT/latency）
- 前端新 `AnalysisPage.vue`：模型/Key 维度占比 + 延迟诊断 + 成本构成

---

## Phase 4 —— 里程碑 4：设置页

- 后端 `query.go`：`model_pricing` 加 `source` 字段；`GET /pricing/sync/preview`（models.dev 拉取比对）
- 前端新 `SettingsPage.vue`：定价表编辑 + 价格同步（预览→勾选→应用）+ API Key 别名

---

## Phase 5 —— 里程碑 6：发布

- README / LICENSE / logo 齐备
- 多平台构建产出 + checksums.txt
- GitHub release + registry 条目