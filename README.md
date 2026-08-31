# usage-lens

一个 CLIProxyAPI（CPA）原生插件——**不启动任何独立服务**，在 CPA 进程内持久化并可视化用量：
请求数 / Token / 成本 / 缓存命中 / 延迟 / 错误率，按模型、API Key 等维度分析。

核心技术路线：用量事件由 CPA 在请求完成后同步推送给插件（`UsagePlugin` 能力），插件缓冲落本地 SQLite，
面板走 CPA 资源路由 —— 全部跑在 CPA 进程内。

## 优势

| | 传统独立服务方案 | usage-lens |
|---|---|---|
| 部署 | 独立进程 + 端口 + 常驻守护 | 放一个动态库 + 配置开一个开关，随 CPA 启停 |
| 采集 | 消息队列订阅/轮询（独占消费） | 事件推送（同源、非破坏、零延迟） |
| 基础设施 | Redis + 独立数据库 + 账号体系 | 仅内置 SQLite |
| 密钥 | 管理密钥环境变量 | 零密钥配置（面板挂靠 CPA 登录态） |
| 运维 | 单独监控 / 升级 / 守护 | 与 CPA 生命周期一致 |
| 升级换代 | 独立发版 + 重启服务 | 换 .so 即生效，随 CPA 重启 |
| 共存迁移 | 独占队列，多实例冲突 | 非破坏性采集，可与任何现有采集方案并存灰度 |

## 架构

```
请求完成 → CPA 用量分发 → UsagePlugin(usage.handle) → 缓冲通道 → 后台落 SQLite
面板：Vite 产物 go:embed → 资源路由 /v0/resource/plugins/usage-lens/
```

- `usage.handle` 是同步回调，插件内 O(1) 入队返回（缓冲 + 溢出丢弃计数），不阻塞 CPA 分发。
- 事件为广播复制，不消费队列，与任何既存采集方共存互不干扰。
- SQLite 只写一条增长表 `usage_events`，配 WAL + busy_timeout，查询走 ts/model/api_key 索引。

## 功能

- **概览**：7 张统计卡（迷你趋势线）+ 时间范围 5 档（滚动小时/天 + 今天/昨天 + 自定义）+ API Key 筛选
  + Token 热图 + 请求健康时间线 + 实时指标 + Token 份额
- **分析**：按模型 / API Key 的分布（占比、请求、Token、成本、成功率、平均延迟、平均首 Token），表头排序
- **请求事件**：11 列事件表 + 模型 / Key / 来源 / 结果筛选 + 分页 + 详情抽屉 + 列配置持久化
- **设置**：模型定价表（手动编辑）+ models.dev 价格同步（预览→勾选→应用，手动价优先）+ API Key 别名

## 构建

产物 `usage-lens.so`（c-shared 动态库，内嵌面板）。前置：Go 1.26+、Node 22+、pnpm。

```
make build        # 构建前端面板 + 编译动态库
make test         # go test -race -count=1 ./...
make lint         # gofmt + go vet
make release      # 交叉编译 zip + checksums.txt 到 dist/
```

## 安装

1. 下载最新 `usage-lens_<version>_linux_amd64.zip`（Release 资产），把 zip 根目录的 `usage-lens.so`
   放到 CPA 插件目录 `plugins/linux/amd64/`。
2. 配置（`config.yaml`）：

```yaml
plugins:
  enabled: true
  configs:
    usage-lens:
      enabled: true
      # db_path 可选，默认 ~/.cli-proxy-api/usage-lens.db（CPA 数据家目录）
```

3. 重启 CPA。面板入口：`http://<cpa>/v0/resource/plugins/usage-lens/panel`（主面板左侧菜单「Usage Lens」）。
   管理 API 前缀：`/v0/management/plugins/usage-lens/api/`。

## 配置

| 键 | 默认 | 说明 |
|---|---|---|
| `db_path` | `~/.cli-proxy-api/usage-lens.db` | SQLite 文件路径（CPA 数据家目录，与 auth 文件同生命周期） |

零密钥：用量事件采集不需要任何密钥；面板数据查询走 CPA 鉴权（面板同源取主面板登录态，也可 `?key=` 传入）。

## 数据与升级

- 数据落在 `db_path` 指定的 SQLite（默认 CPA 家目录），升级插件**不丢数据**（换 .so 即生效）。
- 备份 = 复制该 db 文件（WAL 模式下连同 `-wal`/`-shm` 一起复制，或先停 CPA）。
- 面板为 `go:embed` 内嵌，无需单独部署前端。

## 安全

- 面板不渲染任何上游凭证明文（API Key 全程掩码显示）。
- 管理路由统一走 CPA 管理鉴权；敏感操作不在未鉴权资源路由上执行。
- 无外发流量：models.dev 价格同步为可选手动触发，12s 超时失败仅提示、不影响已有定价。

## 设计取舍

- 聚焦用量分析核心：请求数、Token、成本、缓存、延迟、错误率与维度分布。
- 不做凭证管理、社区/排行、独立登录、多语言、版本检查、定时备份、预聚合与归档表
  （单机量级实时查询足够，保持插件最小面）。

## 开发

```
make build        # 前端 pnpm build + go build
```

面板为 Vite + Vue 3 + TS（`panel/`），构建产物经 `go:embed all:panel/dist` 内嵌进插件。

## 许可

MIT