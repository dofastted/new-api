---
mode: plan
task: claude provider circuit breaker rebuild
created_at: "2026-07-05T21:30:00+08:00"
complexity: complex
---

# Plan: claude provider circuit breaker rebuild

## Goal
- `claude-sub` 出现 HTTP `500/502/503` 时，统一归类为 `high_load_temporarily_unavailable`。
- 该类错误触发 `claude-sub` 熔断 5 分钟。
- 熔断期间路由跳过 `claude-sub`，优先 fallback 到 `claude-p-max`。
- 保留现有安全边界：客户端取消不重试、不熔断；`skip_retry` / 官方 CLI 限制错误不熔断；429 限流仍走现有限流等待/冷却逻辑。

## Scope
- In:
  - 重构 channel circuit breaker 为可配置策略。
  - 支持按 channel / provider group / status code / error code / message keyword 匹配错误类别。
  - 为 `claude-sub` 配置 `500/502/503 -> high_load_temporarily_unavailable`，open duration `300s`，failure threshold `1`。
  - 路由层在重试时跳过 open circuit 的 `claude-sub`，让同组 `claude-p-max` 接管。
  - 增加分类、熔断、半开恢复、fallback 测试。
- Out:
  - 不重写 provider group 管理 UI 的全部结构。
  - 不改变 billing、quota、pre-consume 语义。
  - 不把所有 5xx 都永久视为高负载；仅按配置匹配。

## Assumptions / Dependencies
- 这里的“provider”按当前 newapi 结构落到 channel：`claude-sub` = channel `#34`，`claude-p-max` = channel `#24`。
- 两个 channel 当前都在 `claude-max` group 中，fallback 依赖现有同组重试选择。
- 当前熔断器不足点：
  - `model/channel_circuit.go` 只支持全局 env：`CHANNEL_CIRCUIT_FAILURE_THRESHOLD`、`CHANNEL_CIRCUIT_OPEN_SECONDS`。
  - `service/channel_circuit.go` 只有粗粒度 `ShouldRecordChannelCircuitFailure(err)`。
  - `controller/relay.go` 记录失败时只传 `err.GetErrorCode()`，没有可配置错误分类。
  - `model/channel_cache.go` 已有 open circuit 过滤，可复用。

## Phases
1. Config model and classifier
   - 新增 `ChannelCircuitPolicy` 配置结构，优先放入现有 channel settings JSON，避免新增迁移。
   - 字段：`enabled`、`failure_threshold`、`open_seconds`、`half_open_success_threshold`、`rules[]`。
   - `rules[]` 字段：`name`、`class`、`status_codes`、`error_codes`、`message_contains`。
   - 新增分类函数：input 为 channel settings 与 `*types.NewAPIError`；output 为 `{record bool, class string, policy}`。
   - 默认保持旧行为，未配置 channel 仍走当前全局 env。
2. Circuit state rebuild
   - 扩展 `ChannelCircuitStatus`：保留 `LastCategory`，增加 `PolicyName`、`FailureThreshold`、`OpenSeconds`、`HalfOpenSuccessCount`。
   - `RecordChannelCircuitFailure` 改为接收 policy/class。
   - open 后固定 `NextAttemptUnix = now + open_seconds`，不因后续失败刷新，避免失败风暴延长窗口。
   - half-open 只允许恢复探测；成功达到阈值后 close，失败立即 reopen。
3. Relay integration and fallback
   - `controller/relay.go:processChannelError` 调用新的 classifier。
   - 匹配 `high_load_temporarily_unavailable` 时记录熔断。
   - 当前 retry 选择会进入 `model.GetRandomSatisfiedChannel`；`model/channel_cache.go:filterOpenCircuitChannelIDs` 已能跳过 open channel，因此 `#34` open 后会选择同组其他可用 channel，目标为 `#24`。
   - 对 `skip_retry` / `access_denied` / 官方 CLI 限制错误继续短路，不进入熔断。
4. Production config
   - 给 `#34 claude-sub` 写入 circuit policy：`enabled=true`、`failure_threshold=1`、`open_seconds=300`、`half_open_success_threshold=1`、`rules=[{name:"claude_high_load", status_codes:[500,502,503], class:"high_load_temporarily_unavailable"}]`。
   - `#24 claude-p-max` 不配置该 high-load 熔断规则，避免 fallback 目标同时被同类规则打开。
   - 保留两个 channel 的 Claude CLI `param_override`。
   - 回滚只需把 `circuit_breaker.enabled` 置为 `false`，或删除 `circuit_breaker`，再清理 channel `#34` 的 circuit cache。
5. Observability
   - error log `other.channel_chain` 增加 `circuit_class`、`circuit_state`、`circuit_open_until`、`fallback_candidate`。
   - 字段仅记录分类、状态和同组 fallback 意图，不记录请求体、请求头、密钥或上游响应正文。

## Tests & Verification
- Error classifier -> `#34 + 500/502/503` 归类为 `high_load_temporarily_unavailable`。
- Error classifier -> `#34 + 403 official Claude CLI` 不熔断。
- Error classifier -> `#34 + client canceled` 不熔断。
- Error classifier -> `#34 + skip_retry` 不熔断。
- Error classifier -> `#34 + 429` 不进入高负载熔断。
- Circuit state -> threshold `1` 后立即 open，open duration 为 `300s`。
- Circuit state -> open 过期后进入 half-open，half-open 成功 close，half-open 失败 reopen。
- Routing -> group `claude-max` 内 `#34` open 时，`GetRandomSatisfiedChannel` 不返回 `#34`。
- Routing -> `#24` enabled 且支持模型时被选中。
- Relay -> 首次 `#34` 返回 500/502/503 后，本次请求重试走 `#24`。
- Relay -> 若 `#24` 返回官方 CLI 403，则不继续 retry，不熔断。
- Target command -> `go test ./model ./service ./controller -run 'ChannelCircuit|RandomSatisfiedChannel|RelayRetry'`。

## Issue CSV
- Path: `issues/2026-07-05_21-30-00-claude-provider-circuit-breaker.csv`
- Must share the same timestamp/slug as this plan.

## Tools / MCP
- `ssh-skill`: 生产配置变更、健康检查。
- `read/grep`: 当前 newapi 熔断与路由代码定位。
- `edit/write`: 计划实施时修改源码与配置。
- `bash`: 运行 Go 目标测试与 plan/issue 脚本。

## Acceptance Checklist
- [x] `claude-sub` 的 500/502/503 被统一归类。
- [x] `claude-sub` 熔断 5 分钟。
- [x] 熔断期间 `claude-max` group 不再选中 `#34`。
- [x] 同组 fallback 能选中 `#24 claude-p-max`。
- [x] 官方 CLI 限制错误不触发熔断。
- [x] 客户端取消不触发 retry 或熔断。
- [x] 429 不触发本次高负载熔断。
- [x] 日志可看见 circuit class/state，但不泄露请求体。

## Risks / Blockers
- `502 Upstream access forbidden` 可能是账号权限问题，不一定是高负载；本计划按需求把 `500/502/503` 统一视为高负载临时不可用。
- half-open 若允许并发探测，可能在恢复瞬间放大请求；建议加单飞 probe guard。
- 如果 `#24` 也不可用，最终仍会返回 503；这不是 routing bug。

## Rollback / Recovery
- 删除或禁用 `#34` 的 channel circuit policy。
- 调用 reset 或清理 `new-api:channel_circuit:v1:channel:34` 缓存键。
- 保留当前 Claude CLI `param_override` 不回滚，除非明确要求恢复旧行为。
- 若新策略误伤，可临时设置 `enabled=false` 或 `failure_threshold=0`。

## Checkpoints
- Commit after: policy struct + classifier + tests。
- Commit after: circuit state rebuild + tests。
- Commit after: relay integration + fallback tests。
- Commit after: production config notes / minimal docs。

## References
- `model/channel_circuit.go:44` global threshold/open config。
- `model/channel_circuit.go:107` failure recording。
- `model/channel_cache.go:119` open circuit filtering。
- `service/channel_circuit.go:28` coarse classifier。
- `controller/relay.go:662` failure recording hook。
- `/mnt/x/project/claude-code-hub/src/lib/endpoint-circuit-breaker.ts:19` endpoint breaker default open duration `300000ms`。
- `/mnt/x/project/claude-code-hub/src/lib/circuit-breaker.ts:447` open/half-open decision。
- `/mnt/x/project/claude-code-hub/src/lib/circuit-breaker.ts:484` failure recording。
- `/mnt/x/project/claude-code-hub/src/app/v1/_lib/proxy/forwarder.ts:1813` provider error handling and fallback。
- `/mnt/x/project/claude-code-hub/src/app/v1/_lib/proxy/provider-selector.ts:1069` circuit/rate-limit filtering。
