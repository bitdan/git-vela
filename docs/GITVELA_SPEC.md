# GitVela — 产品与工程规范

> 规范版本：2026-11-09 ·  
> 本文档是**目标与验收标准**，不是既有功能的“完成证明”。改动前先核对本地 `main` 的最新代码、Git 状态和测试结果。

## 1. 产品定位

GitVela 是轻量、独立的桌面 Git 客户端。目标是让熟悉 **IntelliJ IDEA** 的开发者，使用熟悉的布局、菜单和操作流程完成 Git 日常工作；不追求复刻完整 IDE，也不引入 IntelliJ 运行时。

**参考优先级：**

1. 用户确认的界面和交互要求，尤其是 `docs/reference/idea-git-layout.png`（默认布局基准）。
2. 目标 IntelliJ IDEA 版本的实际可观察行为。
3. `F:\project\rebased` 的相应版本源码（固定 Rebased 1.1.23 作为现阶段研究基线；开始研究前核对 commit/版本）。
4. Git 本身的行为和安全约束。

参考资料与 GitVela 已有实现不一致时，要记录差异并验证，**不得直接把现有实现、旧测试或文案当作正确规范**。未确认的行为标为「待验证」，不得推测后宣称完全一致。对参考源码仅研究行为与设计，遵守许可要求，不复制受限制的资源。

## 2. 固定的默认界面

- **顶部**：紧凑 Main Toolbar、仓库和当前分支入口、常用操作。
- **最左侧**：IDEA 风格的工具窗口活动栏。
- **左上**：Commit Tool Window，显示当前 Working Tree / Index 变更、提交信息与提交操作。
- **右上**：多标签 Editor / Diff / Conflict Workspace。
- **底部（横贯工作区）**：Git Tool Window；内部从左到右为 **Branches、Commit Log/Graph、Changed Files + Commit Details**。
- **最底部**：状态栏。

保持 `docs/reference/idea-git-layout.png` 的区域关系；允许面板拖拽、隐藏、最大化和尺寸恢复，**不因 Rebased 默认视图不同而擅自更换整体布局**。适配不同窗口尺寸，但不能靠隐藏核心功能伪装“对齐”。

### 关键交互

- 单击 Git Log 提交：更新底部历史文件与提交详情；不能无条件覆盖右上已打开的 Editor 标签或丢弃编辑内容。
- 双击历史文件：按参考行为打开或切换相应 Diff 标签；保留其他标签和滚动/编辑状态。
- Commit 的 Working Changes 与 Git Log 的历史 Changed Files **独立选中、独立数据来源**；不能共用导致串扰的状态。
- 分支单击与 Checkout 分开；右键菜单、焦点、键盘操作和禁用条件依据真实上下文。
- Ctrl/Shift 多选、右键菜单、键盘快捷键、状态刷新和异步结果均要有一致的交互规则。
- 外部 Git/Codex/IDE 修改仓库后及时刷新；切换仓库后旧请求结果不得覆盖新仓库界面。

## 3. Git 操作契约：先确定目标，再执行命令

每个动作必须显式定义：**入口 / 操作对象 / 仓库根 / 当前 HEAD / 目标 ref / upstream 或 remote / 前置条件 / 确认与取消 / Git 效果 / 成功、错误及冲突恢复**。

特别要求：

- **Update Branch**：分支上下文的更新入口，应作用于**被选中的本地分支**及其追踪关系；不等同于通用 Pull。非当前分支如无法安全更新，按参考交互禁用、解释或引导切换，**不得悄悄更新 HEAD**。
- **Pull**：独立的远程拉取并集成操作；核对来源 remote/branch、目标与策略，不能硬套 Update 的界面。
- **Fetch**：获取远程引用，不隐式修改工作区或当前分支。
- **Push**：明确目标及 upstream；危险推送必须显式确认和后端校验。
- **Merge / Rebase / Reset / Cherry-pick / Revert / Discard**：明确操作方向、目标、工作区状态、冲突和继续/中止；取消时不产生写操作。
- **Commit and Push**：本地提交与远程推送结果分别反馈；Push 失败不得报告为 Commit 失败。

所有 Git 写操作必须防重复、验证仓库与目标在执行前是否变化；禁止静默 `reset --hard`、force push 或丢失未保存内容。操作中断应保留可恢复的 Git 状态。

## 4. 技术栈与架构约束

- 桌面：**Wails 3 + Go**；前端：**Vue 3 + TypeScript + Pinia + CodeMirror 6**。
- 样式目标：**Tailwind CSS 4**（当前代码未接入时先完成官方 Vite 集成）。继续使用现有 CSS Variables 作为语义设计变量，复杂 Splitter、Graph、Diff、CodeMirror 样式可保留专用 CSS；渐进迁移，避免全局 reset 引发回归。
- `internal/git`：Git 命令、模型、安全校验和状态处理。`internal/app`：尽量薄的 Wails 入口和事件绑定。
- 前端按领域渐进拆分：`branches`、`sync`、`history`、`working-changes`、`diff`、`editor`、`stash`、`worktrees`；共享基础组件与类型集中维护。
- 逐步减轻 `App.vue` 的业务职责：主界面负责布局和组合，领域模块管理自身交互，统一 Git Action/Command 入口协调上下文、可用性、执行和刷新。
- Pinia 只保存需要跨组件共享的业务状态；编辑中草稿、弹窗局部状态保留在相应模块；异步读请求必须能丢弃过期结果。
- 不为了“架构好看”一次性重写所有模块、迁移整个 CSS 或破坏生成的 Wails bindings。涉及 Go 绑定变更时必须重新生成并验证 TypeScript 类型。
- Node 24 使用项目独立环境，不升级用户全局 Node；以仓库实际锁文件、`go.mod` 和构建配置为准。

## 5. 对等审计与验收

**一个功能“存在”不等于“与 IDEA 对齐”。** 每个需要复刻的动作至少保留以下证据：

| 项目 | 必需证据 |
| --- | --- |
| 参考行为 | IDEA 实测记录或 Rebased 文件/符号/调用链，附版本 |
| 目标实现 | GitVela 组件、状态和 Go 入口 |
| 状态矩阵 | 正常、禁用、无 upstream、dirty、ahead/behind/diverged、冲突等适用状态 |
| 测试 | Go Git fixture + Vue 交互测试 + 必要的 Windows WebView2 桌面 smoke |
| 结果 | `PASS`、`PARTIAL`、`FAIL`、`UNVERIFIED`，不可用笼统“已交付”代替 |

测试优先使用**临时 bare remote、多 clone、临时工作树**，不要在真实开发仓库上测试破坏性操作。桌面交互使用现有 `tests/desktop-smoke.mjs` 和 WebView2 CDP 基础；视觉对照使用相同仓库数据和窗口尺寸截图。无法执行 GUI 验证时明确标为 `UNVERIFIED`。

每轮最低检查：

```powershell
npm --prefix frontend run typecheck
npm --prefix frontend test
go test ./...
```

涉及桌面布局、菜单、焦点或 Git 写操作的改动，还须按 README 的说明运行相关桌面 smoke；不可用时如实记录。Git 重负载测试在 Windows 上顺序执行。

## 6. 实施顺序与完成标准

1. **P0 安全与语义**：Branch Update / Pull / Push 操作对象、工作区保护、错误与冲突恢复；修正把错误交互固定下来的旧测试。
2. **P1 高使用频率交互**：Branch/Commit 右键菜单、Git Log、Working Changes、Diff、Editor 标签和状态联动。
3. **P1 架构与样式**：引入 Tailwind 4，提炼 Design Tokens，拆分 App.vue 和 Git Action，保持功能和截图不退化。
4. **P2 体验与性能**：大型提交图、虚拟列表、复杂文件、会话恢复、快捷键、主题和窗口细节。

每批必须完成**参考证据 → 差异 → 修改 → 自动测试 → 桌面复核**的闭环。保持小批量、可回滚的提交，不要求一轮重构所有代码。未经证据验证不得宣称“完整复刻”。

## 7. 不做与文档维护

- 不做完整 IDE、语言服务或无关 VCS 的功能克隆；高级能力按实际使用价值和风险决定。
- `README.md`：项目说明、安装、运行和测试方法；`AGENTS.md`：Codex 工作规则；**本文档**：唯一持续有效的产品、交互和架构规范。
- 功能进度、临时差异、批次日志不回填为冗长的永久规范；需要时记在 Issue/PR 或临时审计文件中，修复后关闭。
- 参考图 `docs/reference/idea-git-layout.png` 保留；重要的已确认设计变更修改本文档并说明原因。
