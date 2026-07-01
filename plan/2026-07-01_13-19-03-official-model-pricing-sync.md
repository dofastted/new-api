---
mode: plan
cwd: /mnt/x/newapi
task: 官方模型元数据与定价统一同步
complexity: complex
tool: sequential-thinking
total_thoughts: 10
created_at: 2026-07-01T13:19:03+08:00
issue_csv: issues/2026-07-01_13-19-03-official-model-pricing-sync.csv
---

# Plan: 官方模型元数据与定价统一同步

🎯 任务概述

当前系统同时存在模型元数据、模型倍率、固定价格、缓存倍率、表达式计费和上游差异同步。目标是建立一个官方来源优先的模型元数据与价格事实源：每天从 OpenAI、Anthropic Claude、Google Gemini、智谱 GLM 官方价格源检测一次，归一化为统一模型定价结构，并让运行时计费、定价页和模型元数据同步都只采纳官方价格。非官方价格条目不再进入官方定价目录；Codex 系列模型按官方 `$ / 1M tokens` 单位计费。

## Goal

- 建立统一的官方模型元数据和价格目录，覆盖输入、输出、缓存读取、缓存创建、固定/按次价格与表达式计费所需字段。
- 每 24 小时自动检测官方价格变更，支持手动触发、审计结果、失败保留上一版成功快照。
- 运行时计费只读取官方价格目录或由其生成的兼容缓存；缺少官方价格的模型不得静默落到非官方价格。
- Codex 模型使用官方每 1M token 价格，避免被当作固定按次价格或旧倍率补丁。

## Scope

- In:
  - 后端官方价格数据模型、跨 DB 迁移、同步服务、缓存、系统任务、管理 API。
  - OpenAI、Claude、Gemini、GLM 官方价格源 fetch/parse/normalize。
  - 现有 `ModelRatio` / `CompletionRatio` / `CacheRatio` / `CreateCacheRatio` / `ModelPrice` 的兼容生成或读路径迁移。
  - 模型元数据 `models` / `vendors` 与官方价格目录的关联。
  - 前端定价同步入口增加官方-only状态、每日同步结果展示、非官方来源禁用提示。
  - Go 单元/集成测试和前端目标构建验证。
- Out:
  - 不引入非官方价格源作为自动同步来源；`models.dev` / OpenRouter 只能保留为手动对比或调试，不参与官方定价落库。
  - 不删除用户渠道、密钥、provider group；“移除非官方 model”限定为官方定价目录和由其生成的价格配置，不直接删除业务渠道能力。
  - 不处理无法从官方用量返回中计量的缓存存储时长费用；若官方价格页给出 storage/hour，仅先保存在元数据中并在 UI 标注未计费。

## Assumptions / Dependencies

- 官方价格以官方页面/API为准：OpenAI `https://developers.openai.com/api/docs/pricing.md`、Claude `https://platform.claude.com/docs/en/about-claude/pricing.md`、Gemini `https://ai.google.dev/gemini-api/docs/pricing`、GLM `https://bigmodel.cn/pricing`。
- 现有内部倍率换算仍需兼容：`1 ratio = $0.002 / 1K tokens`，因此 `$X / 1M input tokens` 可换算为 `model_ratio = X / 2`；表达式计费则直接使用 `$ / 1M tokens`。
- 多实例部署已有 `system_tasks` 租约框架，可复用来保证每日任务单实例执行。
- 官方页面结构可能变化，解析器必须有 fixture 测试和失败降级，不能把空解析结果写入当前生效版本。

## Phases

1. **现状审计与口径冻结**
   - 梳理 `setting/ratio_setting`、`model/pricing.go`、`relay/helper/price.go`、`service/text_quota.go`、`controller/ratio_sync.go`、`controller/model_sync.go` 的当前读写路径。
   - 明确定价口径：官方目录存 `$ / 1M` 原始价格；运行时只通过统一服务读取；旧 option map 是兼容层而不是事实源。

2. **设计官方价格目录模型**
   - 新增跨 SQLite/MySQL/PostgreSQL 的 GORM 表，例如 `official_model_prices` 和必要的 `official_model_price_snapshots`。
   - 字段至少包括：provider、model_name、display_name、input_usd_per_mtok、output_usd_per_mtok、cache_read_usd_per_mtok、cache_write_5m_usd_per_mtok、cache_write_1h_usd_per_mtok、fixed_usd_per_call、billing_mode、billing_expr、source_url、source_hash、source_updated_at、status、raw_json、created_at、updated_at。
   - 对 provider + model_name 建唯一索引；快照表记录每次同步的新增/变更/移除数量与错误。

3. **实现统一归一化层**
   - 新建官方价格 domain/service，输入供应商原始条目，输出统一 `OfficialModelPrice`。
   - 提供两种 materializer：
     - legacy ratio maps：生成现有 `ModelRatio`、`CompletionRatio`、`CacheRatio`、`CreateCacheRatio`、`ModelPrice` 兼容数据；
     - billing expression：对阶梯、长上下文、缓存 5m/1h 或 Codex 1M 计费生成 `tiered_expr`。
   - 去掉运行时散落换算，所有换算集中测试，避免 OpenAI/Claude/Gemini/GLM 各写一套隐式规则。

4. **官方源适配器拆分**
   - 将 `controller/ratio_sync.go` 里 OpenAI/Claude 解析逻辑迁出控制器，保留控制器只做 HTTP 输入输出。
   - 增加 `openai`、`anthropic`、`gemini`、`zhipu` 四个官方源适配器，每个适配器实现 `Fetch -> Parse -> Normalize -> Validate`。
   - OpenAI/Codex：解析标准 tier 的 token 表；Codex 系列模型（如 `gpt-*-codex*`）按官方 `$ / 1M tokens` 进入 token 计费，不进入固定按次价格。
   - Claude：保留 cache read、5m write、1h write；移除 `claudeCacheCreation1hMultiplier` 作为唯一来源，改用官方字段，缺字段时才走兼容默认。
   - Gemini：解析付费 tier 的 standard 价格；有长上下文分段的模型生成表达式而不是单一 ratio。
   - GLM：优先读取官方页面/API；若页面需 JS 渲染，先封装可替换 fetcher，并用固定 fixture 测试解析逻辑。

5. **同步任务与缓存闭环**
   - 新增 `SystemTaskTypeOfficialPricingSync` 和 handler，默认启用、间隔 24h，支持 env 调整和手动触发。
   - 任务执行流程：获取上一版快照 -> 并发拉取四个官方源 -> parse/validate -> DB transaction upsert -> 标记官方已移除模型 -> 生成兼容缓存 -> 失效 `model.GetPricing()` 与 ratio exposed cache。
   - 复用 system task lease，避免多 master 同步；失败时记录错误并保留上一版成功快照，不写入半成品。

6. **运行时计费切换**
   - 在 `relay/helper/price.go` 前置读取官方价格服务；缺官方价格时返回现有“价格未配置”错误，不回退到非官方 map。
   - `model.GetPricing()` 改为展示官方目录与渠道能力的交集；非官方价格条目不再显示为可计费模型。
   - 保留 provider group / user group ratio 作为官方基础价之后的业务倍率，不改变当前分组结算逻辑。

7. **管理端与 API 调整**
   - `/api/ratio_sync/channels` 中官方来源扩展为 OpenAI、Claude、Gemini、GLM；默认选择官方-only preset。
   - 新增后台接口查看最近同步结果、手动触发同步、查看移除/新增/变更模型列表。
   - 前端 `UpstreamRatioSync` 增加官方-only说明；禁用把 `models.dev` / OpenRouter 应用为官方价格的路径，保留“仅对比”标签。
   - i18n 补齐新增文案，避免英文 key 缺失。

8. **清理非官方价格与兼容迁移**
   - 写一次性迁移/任务：从旧 option maps 中剔除不在官方目录的 token 模型价格；保留明确的非 LLM 按次任务价格（如 MJ/Suno/图片/视频）并标记为 `non_token_task_price`。
   - 对本地 `models` 元数据：官方目录不存在的模型不直接删除，改为 `official_price_status=missing/removed` 或在定价页隐藏，避免误删用户渠道配置。
   - 删除旧的硬编码 TODO 和散落价格注释，统一由官方同步生成。

9. **测试与验证**
   - 解析器 fixture：OpenAI、Claude、Gemini、GLM 各覆盖正常、空表、负值/缺字段、缓存读写、长上下文阶梯、Codex 模型。
   - DB/service：SQLite 内存库跑 upsert、remove、snapshot rollback、cache miss 创建、缓存失效。
   - 计费：覆盖 token 模型、固定价格模型、cache read、cache create 5m/1h、Gemini 长上下文表达式、Codex 1M token 价格、缺官方价格拒绝。
   - 系统任务：覆盖 24h 调度、手动触发、并发租约、失败不覆盖上一版。
   - 前端：目标组件测试或 `bun run build`，确认新增文案与官方-only UI 不破坏设置页。

10. **上线、回滚与观测**
    - 先上线 dry-run：每日检测只写 snapshot，不切换 runtime；对比当前价格差异并人工确认。
    - 再切换 read path：官方目录成为事实源，旧 option map 只作为生成缓存。
    - 回滚方式：关闭 env 开关，恢复旧 ratio map 读取；DB 保留上一版成功 snapshot 可重新 materialize。
    - 观测指标：同步成功/失败、每供应商解析条目数、变更模型数、移除模型数、运行时缺价拒绝数。

## Tests & Verification

- `go test ./controller -run 'Test.*Official.*Pricing|Test.*RatioSync'` -> 官方源 parser 与旧 ratio_sync 回归。
- `go test ./model -run 'Test.*Official.*Price|Test.*Pricing'` -> DB 模型、快照、`GetPricing` 交集行为。
- `go test ./service -run 'Test.*Official.*Pricing|Test.*TextQuota|Test.*SystemTask'` -> 计费公式、系统任务、缓存行为。
- `go test ./relay/helper -run 'Test.*Price'` -> runtime price helper 缺价/有价路径。
- `cd web/default && bun run build` -> 前端管理页和 i18n 编译验证。
- 手动 smoke：触发官方价格同步，确认 `/api/pricing` 返回包含官方 `pricing_version/source_hash`，缺官方价格模型不能扣费。

## Issue CSV

- Path: `issues/2026-07-01_13-19-03-official-model-pricing-sync.csv`
- Must share the same timestamp/slug as this plan.
- 计划评审通过后再生成 CSV，建议拆成 6 个 issue：数据模型、官方源适配器、同步任务缓存、运行时计费切换、前端管理、测试与迁移。

## Tools / MCP

- `sequential-thinking/sequentialthinking`：复杂任务拆解与计划结构化。
- `Read` / `Grep` / `Glob`：定位现有定价、模型元数据、系统任务、前端同步路径。
- `web_search` / `Read URL`：验证官方价格源地址。
- `Go test` / `Bun build`：实施阶段目标验证。

## Acceptance Checklist

- [ ] 官方目录只包含 OpenAI、Claude、Gemini、GLM 官方源解析出的 token 模型价格。
- [ ] 每日同步默认 24h 执行，失败不覆盖上一版成功官方价格。
- [ ] 非官方来源不会被自动写入运行时价格事实源。
- [ ] Codex 模型按 `$ / 1M tokens` 计费，覆盖输入/输出与缓存命中。
- [ ] cache read、cache write 5m、cache write 1h 价格可被官方源表达并用于结算。
- [ ] `/api/pricing` 和管理端展示同一份官方价格版本。
- [ ] 缺少官方价格的模型无法静默计费，错误信息可定位。
- [ ] SQLite、MySQL、PostgreSQL 迁移和测试路径明确。

## Risks / Blockers

- 官方页面结构变化会导致 parser 失效；必须用 fixture + parse count 阈值 + 失败保留旧快照防止空写入。
- Gemini/GLM 页面可能依赖 JS 或有多计费 tier；需要先确定官方可机器读取源，否则实现可替换 fetcher。
- 旧 `ModelRatio` 允许管理员手写任意模型；切到 official-only 会影响历史自定义模型，需要发布前输出影响列表。
- 当前 Claude 1h cache create 由代码硬编码推导；切官方字段会改变边界模型价格，需要专门回归。
- 非 token 的按次任务价格不能误删；清理逻辑必须区分 token model 与任务模型。

## Rollback / Recovery

- 保留旧 option maps 一段版本窗口，只关闭 official read path，不删除历史配置。
- 同步任务写入新 snapshot 后才更新 active version；失败直接保留旧 active version。
- 提供 admin 手动 re-materialize：从指定 snapshot 重新生成兼容 ratio maps。
- 若官方 parser 全部失败，系统任务标记 failed 并告警，不影响现有计费。

## Checkpoints

- Commit after: 官方价格表与 parser fixture 完成。
- Commit after: 同步任务、缓存和 API 完成。
- Commit after: runtime 切换、前端、清理迁移和全量目标测试完成。

## References

- `model/pricing.go:18-38`：当前 `/api/pricing` 输出模型价格字段。
- `model/pricing.go:66-87`：当前 pricing 1 分钟进程缓存与失效入口。
- `model/pricing.go:110-358`：当前按渠道能力、模型元数据和 ratio settings 构造价格列表。
- `setting/ratio_setting/model_ratio.go:26-300`：当前硬编码默认模型倍率与固定价格。
- `setting/ratio_setting/model_ratio.go:331-405`：当前 ratio/price map 初始化和查询。
- `setting/ratio_setting/cache_ratio.go:7-178`：当前 cache read/create ratio 配置。
- `relay/helper/price.go:62-158`：请求预扣费读取 `PriceData` 的主路径。
- `service/text_quota.go:228-300`：文本结算中 token/cache/image/audio 的现有公式。
- `model/model_meta.go:24-41`：当前模型元数据与 `SyncOfficial` 字段。
- `controller/model_sync.go:23-50`：当前上游模型元数据源配置。
- `controller/model_sync.go:265-470`：当前模型元数据同步逻辑。
- `controller/ratio_sync.go:32-55`：当前 ratio sync 内置 preset，仅有 OpenAI/Claude 官方源。
- `controller/ratio_sync.go:151-566`：当前上游价格拉取与差异构建控制器逻辑。
- `controller/ratio_sync.go:773-826`：当前官方 token price 到 ratio map 的换算函数。
- `controller/ratio_sync.go:861-1015`：当前 OpenAI/Claude 官方价格 parser。
- `controller/ratio_sync.go:1287-1342`：当前同步源列表接口。
- `service/system_task.go:30-47`：系统任务 handler/scheduled handler 接口。
- `service/system_task.go:259-303`：系统任务调度器按 interval 创建任务。
- `controller/system_task_handlers.go:72-112`：现有 scheduled model update handler 可作参考。
- `web/default/src/features/system-settings/models/constants.ts:34-55`：前端当前官方 preset 只有 OpenAI/Claude。
- `web/default/src/features/system-settings/models/upstream-ratio-sync.tsx:386-443`：前端当前把选中上游值写回 option maps。
- `pkg/billingexpr/expr.md:15-20`：表达式计费用 `$ / 1M tokens` 与版本化规则。
- `https://developers.openai.com/api/docs/pricing.md`：OpenAI 官方价格源，已验证可读。
- `https://platform.claude.com/docs/en/about-claude/pricing.md`：Claude 官方价格源，已验证可读。
- `https://ai.google.dev/gemini-api/docs/pricing`：Gemini 官方价格源，已验证可读。
- `https://bigmodel.cn/pricing`：GLM/智谱官方价格页，已验证存在但需要 JS 解析策略。
