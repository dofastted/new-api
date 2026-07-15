---
mode: plan
task: Provider groups 交互复杂度优化
created_at: "2026-07-15T14:44:44+08:00"
complexity: complex
---

# Plan: Provider groups 交互复杂度优化

## Goal
- 将 Provider groups 页面的主流程收敛为“选择分组 → 管理 Provider 与顺序 → 修改必要设置 → 统一保存”。
- 默认界面只展示高频路由配置；计费倍率、描述、自定义优先级层级和删除进入次级区域。
- 消除立即保存、详情保存、成员保存和 Auto 独立保存并存造成的认知负担。
- 保持现有 provider group、成员优先级、Auto 路由和计费语义不变。

## Scope
- In:
  - 左侧分组区域改为带搜索的紧凑导航列表。
  - Provider 成员管理提升为右侧主区域，分组设置降级为折叠区域。
  - 将 Provider 搜索、选择、添加合并为一个可搜索 Combobox。
  - 默认用列表顺序表达 Provider 回退顺序，自定义 priority/weight 进入高级模式。
  - 建立统一草稿、未保存保护、Discard 和单一 Save changes 操作模型。
  - 为统一保存增加事务型后端更新入口，避免详情和成员部分成功。
  - Auto 分组在右侧直接编辑 route type 候选规则，并与独立 Auto 页面复用组件。
  - 补齐桌面、移动端、键盘操作、i18n 和路由结果验证。
- Out:
  - 不改变 provider group 路由算法、计费算法、API key 绑定语义和 Auto fallback 语义。
  - 不删除后端现有 priority、weight、sort_order 能力。
  - 不重构 Provider channel 管理页面或全站导航。
  - 不修改受保护的项目与作者标识，不修改 `routeTree.gen.ts`。

## Assumptions / Dependencies
- 页面核心用户任务是为分组选择 Provider 并配置回退顺序；Display name、Billing multiplier 和 Description 属于次级设置。
- 分组 `sort_order` 在当前路径主要控制分组列表顺序，不代表 Provider 路由优先级。
- 相同 priority 可能承载同层分流语义，因此只从默认界面隐藏，不能删除后端能力。
- Auto 是特殊 provider group，其候选项按 `completions`、`responses`、`messages`、`other` 四种 route type 独立排序。
- 统一保存必须具备事务边界；前端串行调用现有详情和成员接口不能视为原子保存。
- 当前前端一次请求最多加载 10000 个 channel；数据量继续增长时，Provider Combobox 需要服务端搜索支持。

## Current Flow and Problems
1. 页面加载 group 列表和最多 10000 个 channel，并自动选择第一个 group；选择后再加载成员关系。
2. 左侧每行提供上移、下移和选择；排序立即发送两个更新请求，而详情和成员需要分别保存。
3. 创建弹窗默认展示稳定名称、显示名称和 Usage ratio 三个字段，创建后才进入成员配置。
4. 详情区域先展示 Display name、Usage ratio、Online 和 Description，Provider 成员管理排在其后。
5. 添加 Provider 需要“搜索框 → 原生下拉框 → Add 按钮”三个控件。
6. 成员顺序同时受拖拽顺序和数值 priority 控制，用户难以判断两者关系。
7. metadata 没有 dirty 判断，membership 有 dirty 判断；切换 group 时没有未保存提示，草稿会随组件卸载而丢失。
8. Auto group 出现在普通 group 列表中，但选中后必须跳到另一页面才能配置实际规则。
9. 删除长期暴露在详情头部；关闭 routing、移除全部 Provider 和删除 group 的业务影响没有形成一致的高风险确认流程。

## Target Information Architecture
- 页面头部：标题、帮助入口、New provider group；移除常驻说明横幅。
- 左侧：Search groups、Auto 固定入口、普通 group 列表；每行只显示 Display name、稳定标识和状态。
- 右侧头部：分组身份、Routing enabled、未保存状态、Discard、Save changes、更多菜单。
- 右侧主区域：Providers in this group；说明“Providers are tried from top to bottom”。
- 右侧次级区域：折叠的 Group settings，包含 Display name、Routing enabled、Billing multiplier、Description。
- 更多菜单：Delete provider group 及其他低频操作。
- Auto group：用同一右侧区域直接展示四种 route type 的候选 group 顺序，不显示普通成员编辑器。

## Interaction Rules
- Provider 添加：单个可搜索 Combobox，选择后立即加入草稿并允许连续添加；已选项不再出现在结果中。
- Provider 排序：默认只显示顺序；桌面拖拽、键盘和触屏上移/下移操作写入同一顺序模型。
- 高级路由层级：仅在 Advanced routing tiers 展开后显示 priority 和 weight，并解释相同 priority 的行为。
- 分组排序：从常驻列表行移除；如业务仍需要，进入独立 Reorder groups 模式并一次保存。
- 创建 group：默认只展示 Group ID 和可选 Display name；Billing multiplier、Description 和初始状态放入 Advanced settings。
- 创建成功：自动选中新 group，并聚焦 Add provider Combobox。
- 保存：metadata、成员和顺序进入统一草稿；只有一个 Save changes，并提供 Discard。
- 未保存保护：切换 group、打开 Auto、离开页面或删除前提供 Save and continue、Discard changes、Stay here。
- 高风险确认：禁用 routing、移除全部 Provider、删除 group 时明确说明对路由和现有 API key 的影响。
- 文案：`Online` 调整为 `Routing enabled`，`Usage ratio` 调整为 `Billing multiplier`，`Provider membership` 调整为 `Providers in this group`，默认顺序使用 `Fallback order` 表达。

## Phases
1. Frontend information architecture
   - 左侧数据表改为可搜索导航列表，移除常驻排序按钮和 Usage ratio 列。
   - Provider 管理移到 Group settings 之前；Group settings 改为折叠区域。
   - 搜索、选择和 Add 合并为现有组件体系中的可搜索 Combobox。
   - 删除移入更多菜单；补齐 metadata dirty 检测和 group 切换保护。
   - 保持现有两个保存接口和保存边界，避免在没有事务支持时伪装为原子保存。
2. Unified transactional save
   - 定义 metadata + members 的统一更新 DTO，只提交 dirty 部分。
   - 在 Controller → Service → Model 路径增加事务型保存逻辑。
   - 事务成功后统一重建 abilities、同步 channel group 并刷新 channel cache。
   - 前端合并草稿状态，增加 Discard 和唯一 Save changes。
   - 对禁用 routing、清空成员和删除加入影响确认。
3. Priority and Auto simplification
   - 默认移除数值 priority 列，以列表顺序表示回退顺序。
   - 增加 Advanced routing tiers，保留相同 priority 与 weight 能力。
   - 为排序增加键盘和触屏操作，不把 HTML5 drag 作为唯一入口。
   - 抽取可复用 Auto rules editor；Auto group 右侧内联使用，`/providers/auto` 路由继续复用。
   - 将 Auto 四张卡片压缩为四行 route type 配置，统一添加和排序交互。
4. Scale and polish
   - Provider 规模较大时增加服务端搜索，避免默认加载 10000 个 channel。
   - 同步 en、zh、fr、ja、ru、vi 六语言文案。
   - 完成响应式、无障碍、错误反馈和保存状态校验。

## Tests & Verification
- Common add flow -> 从选中 group 到添加 Provider 并保存，不超过“搜索选择 Provider + Save”两个编辑动作。
- Save model -> 页面同一时刻最多显示一个主保存按钮，metadata 和 membership 的 dirty 状态统一。
- Unsaved guard -> 修改后切换 group、进入 Auto 或离开页面均不会静默丢失草稿。
- Transaction -> metadata 或 membership 任一校验失败时数据库不产生部分更新，abilities 和 cache 不刷新为半完成状态。
- Routing order -> 保存后的实际 Provider 选择顺序与界面从上到下的顺序一致。
- Advanced tiers -> 相同 priority 和 weight 的现有路由语义保持不变。
- Auto rules -> 四种 route type 的候选 group 添加、删除、排序、保存结果与现有 API 契约一致。
- Risk confirmation -> 关闭 routing、清空成员和删除 group 都显示准确影响说明。
- Accessibility -> 搜索、添加、排序、删除和保存可以只用键盘完成；移动端不依赖拖拽。
- Responsive -> 375px、768px、1440px 无横向溢出，左右区域保持正确滚动。
- Frontend checks -> `bun run typecheck`、`bun run lint`、`bun run format:check`、`bun run i18n:sync`、`bun run build`；以实际 `package.json` 脚本名为准。
- Backend checks -> 针对统一保存事务、路由重建和 Auto 规则运行目标 Go tests，并启动应用执行一次真实 UI smoke test。

## Issue CSV
- Path: `issues/2026-07-15_14-43-19-provider-groups-interaction-optimization.csv`
- Must share the same timestamp/slug as this plan.

## Tools / MCP
- `read` / `grep` / `lsp`: 定位 Provider groups、Auto rules、路由和缓存契约。
- `edit` / `write`: 修改现有前后端文件和新增必要测试。
- `browser` / `chrome-devtools`: 验证桌面、移动端、键盘流程和保存反馈。
- `bash`: 运行 Bun、Go 和计划校验命令。
- `i18n-translate`: 同步六语言用户可见文案。
- `shadcn-ui`: 复用项目现有 Combobox、Dialog、Dropdown 和表单模式。
- `vercel-react-best-practices`: 审查 React 状态边界、查询缓存和渲染行为。

## Acceptance Checklist
- [ ] 左侧 group 区域可以搜索，默认不显示排序箭头和 Usage ratio 列。
- [ ] 普通 group 的 Provider 编辑器是右侧第一主区域，Group settings 默认折叠或视觉降级。
- [ ] 添加 Provider 只使用一个可搜索 Combobox，不再需要独立搜索框、NativeSelect 和 Add 按钮。
- [ ] 默认界面只使用列表顺序表达 Provider 回退顺序。
- [ ] 相同 priority 和 weight 能力保留在高级模式，现有路由语义不变。
- [ ] metadata 与 membership 使用统一 dirty 状态、Discard 和唯一 Save changes。
- [ ] group 切换、页面离开和 Auto 切换不会静默丢失未保存修改。
- [ ] 统一保存具备事务性，不会出现详情成功而成员失败的半保存状态。
- [ ] Auto group 直接显示 Auto rules editor，不显示无效的普通成员区域。
- [ ] 删除、禁用 routing 和清空成员均有准确的影响确认。
- [ ] Provider 排序支持桌面、键盘和移动端操作。
- [ ] 六语言文案、类型检查、lint、format、build、目标 Go tests 和浏览器 smoke test通过。

## Risks / Blockers
- 相同 priority 可能代表真实的同级分流，不能仅根据当前 UI 直接删除该字段。
- 新的统一更新接口必须兼容 SQLite、MySQL 和 PostgreSQL，并保证路由重建失败时状态可恢复。
- Auto group 是否为不可删除的系统实体需要以当前 model 和初始化逻辑确认；确认前不能只在前端隐藏删除。
- 现有 API key 绑定 group 后不会随删除自动迁移，删除确认必须保留这一事实。
- 服务端搜索会改变 channel 查询契约，应避免与主交互重构同时引入无关分页行为。
- 响应式下不能通过重新暴露 numeric priority 来解决触屏拖拽问题，应提供同一顺序模型的可访问操作。

## Rollback / Recovery
- 阶段一仅调整前端信息架构，可按组件恢复原有列表、详情和成员区域，不涉及数据迁移。
- 统一更新接口上线时保留现有详情和成员接口作为内部回滚路径，前端不同时暴露两套操作模型。
- 若事务型保存出现问题，前端回退到两个明确保存区域，不能继续显示虚假的统一成功状态。
- Advanced routing tiers 只改变字段展示，不改数据库结构；可直接恢复默认 priority 列。
- Auto editor 复用失败时恢复 `/providers/auto` 独立页面入口，保持现有 API 数据不变。

## Checkpoints
- Commit after: 左侧导航、Provider 主区域、Combobox、settings 层级和 dirty guard。
- Commit after: 事务型统一保存接口、前端统一草稿和目标测试。
- Commit after: Advanced routing tiers、可访问排序和 Auto editor 复用。
- Commit after: i18n、响应式、全量前端检查和浏览器 smoke test。

## References
- `web/default/src/features/channels/provider-groups.tsx:167-432` 页面加载、选择、分组排序和左右布局。
- `web/default/src/features/channels/provider-groups.tsx:434-540` 创建 group 流程。
- `web/default/src/features/channels/provider-groups.tsx:545-817` metadata、membership 草稿、排序和 Provider 过滤。
- `web/default/src/features/channels/provider-groups.tsx:820-1093` 详情、成员列表、保存和删除交互。
- `web/default/src/features/channels/provider-group-auto-rules.tsx:79-329` Auto rules 草稿、保存和四种 route type 卡片。
- `web/default/src/features/channels/provider-group-api.ts:107-170` 前端 group、members 和 Auto rules API。
- `controller/provider_group.go:20-267` group、members、Auto rules 更新及路由缓存刷新。
- `.trellis/spec/frontend/components.md` 组件复用和组合规范。
- `.trellis/spec/frontend/css-layout.md` 固定高度、滚动和响应式布局规范。
- `.trellis/spec/frontend/i18n.md` 六语言文案规范。
- `.trellis/spec/frontend/quality.md` 前端质量检查规范。
