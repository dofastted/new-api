---
mode: plan
cwd: /mnt/x/newapi
task: Dashboard models billing and usage statistics repair plan
complexity: complex
tool: sequential-thinking
total_thoughts: 10
created_at: 2026-07-02_14-14-53
---

# Plan: Dashboard models billing and usage statistics repair

## 任务概述

`dashboard/models` 的模型调用分析和用户统计目前读取 `quota_data` 物化表，而真实消费扣费先写入 `logs`。近期模型价格和计费逻辑调整后，`quota_data` 存在导出延迟和历史未回算问题，导致 `claude-fable-5` 等模型在看板中低估或漏计。

目标是把看板统计的数据源契约调整为以 `logs.type=consume` 为权威来源，同时保留 `quota_data` 作为缓存/历史物化表，并提供可回算、可验证、可回滚的生产修复路径。

## 执行计划

1. 固定数据源契约
   - 明确 `logs` 是实时计费与统计权威来源，`quota_data` 只作为物化缓存。
   - 梳理 `/api/data`、`/api/data/users`、`/api/data/self` 的返回字段，保持前端 `QuotaDataItem` 兼容。

2. 新增基于 `logs` 的聚合查询
   - 在 `model/usedata.go` 增加模型维度、用户维度、自身用户维度的日志聚合函数。
   - 聚合条件限定 `type = LogTypeConsume`，按时间窗口过滤，按小时 bucket 输出 `created_at`。
   - 聚合字段保持 `model_name` / `username` / `count` / `quota` / `token_used`，避免前端大改。

3. 兼容独立日志库与多数据库
   - 聚合读取走 `LOG_DB`，不依赖主库与日志库跨库 JOIN。
   - SQL 只使用 SQLite、MySQL、PostgreSQL 可兼容的表达式；如 ClickHouse 需要单独分支，沿用现有 `common.UsingLogDatabase(...)` 模式。
   - 保留 `quota_data` 原有写入流程，不把物化表删除或改成硬依赖。

4. 切换 dashboard 数据接口
   - `controller.GetAllQuotaDates` 改为调用日志聚合模型维度函数。
   - `controller.GetQuotaDatesByUser` 改为调用日志聚合用户维度函数。
   - `controller.GetUserQuotaDates` 改为调用日志聚合自身用户函数，并保留现有 1 个月窗口限制。
   - 前端 `dashboard/models`、`dashboard/users` 尽量不改数据处理逻辑，只在必要时调整请求参数或 loading 状态。

5. 增加历史回算能力
   - 实现 `RebuildQuotaDataFromLogs(start,end)` 或系统任务 handler，从 `logs` 聚合后在应用层分批写回 `quota_data`。
   - 写入策略必须幂等：同一维度同一小时先删除窗口内旧物化行，或使用明确唯一维度做 upsert，禁止重复累加。
   - 支持限定模型/时间窗口，先用于 `claude-fable-5` 与近期价格调整窗口。

6. 补齐回归测试
   - 新增后端测试：当 `quota_data` 缺失或旧值错误时，`/api/data` 返回 `logs` 聚合值。
   - 覆盖模型维度、用户维度、自身用户维度、时间窗口边界。
   - 覆盖历史回算幂等：重复执行不会重复累加。

7. 本地验证
   - 运行目标后端测试：`go test ./model ./controller`。
   - 如前端类型或接口参数变化，运行：`cd web/default && bun run build`。
   - 用构造数据确认 `claude-fable-5` 的 `count/quota/token_used` 与 `logs` 聚合一致。

8. 生产修复与验收
   - 部署前只读抽样：比对 `logs`、`quota_data`、接口返回。
   - 部署后执行限定窗口回算，优先覆盖近期价格调整窗口和 `claude-fable-5`。
   - 再次比对 `claude-fable-5`、`glm-5.2`、`claude-sonnet-5` 的模型统计和用户统计。

9. 回滚路径
   - 如果日志聚合接口出现性能问题，临时切回旧 `quota_data` 查询路径。
   - 历史回算不改账单事实，只重建物化表；可再次从 `logs` 重建修正。
   - 不做 schema 强迁移，降低回滚成本。

## 风险与注意事项

- 性能风险：`logs` 可能较大，聚合查询必须命中 `created_at`、`type`、`model_name`、`username` 等索引；必要时限制 dashboard 最大查询窗口。
- 数据库兼容风险：`LOG_DB` 可为独立库或 ClickHouse，不能使用跨库 JOIN 或主库方言专属 SQL。
- 历史数据风险：`quota_data` 回算必须幂等，避免重复累加造成二次污染。
- 当前小时延迟：即使保留物化表，当前小时仍会落后；接口切到 `logs` 后才解决实时看板问题。
- 配置同步噪音：生产日志出现 `unexpected end of JSON input`，需另行清理空 JSON 类 option，避免干扰排查。

## 参考

- `web/default/src/features/dashboard/components/models/log-stat-cards.tsx:92`：`dashboard/models` 拉取 quota 数据入口。
- `web/default/src/features/dashboard/api.ts:37-51`：管理员接口当前走 `/api/data`。
- `controller/usedata.go:31-35`：`/api/data` 当前调用 `GetAllQuotaDates`。
- `model/usedata.go:173-182`：`GetAllQuotaDates` 当前聚合 `quota_data`。
- `model/log.go:323-386`：消费日志先写 `logs`，再异步记录 `QuotaData` 缓存。
- `.trellis/workspace/ted/newapi-dashboard-models-24h.csv:2-5`：生产抽样显示 `logs` 与 `quota_data` 差异，含 `claude-fable-5`。

## 验证清单

- `go test ./model ./controller`
- `cd web/default && bun run build`（仅在前端接口或类型变动时必须执行）
- 生产只读 SQL：按模型比对 `logs` 与 `quota_data` 的 `count/quota/token_used`。
- 生产接口验证：`/api/data`、`/api/data/users`、`/api/data/self` 返回值与 `logs` 聚合一致。
