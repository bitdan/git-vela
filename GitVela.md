# GitVela — 轻量级 Git 桌面客户端开发方案

> **GitVela** · A lightweight Git client with an IDE-inspired workflow.  
> **技术栈**：Wails 3 + Go + Vue 3 + TypeScript + Vite  
> **文档版本**：v1.0 · 2026-11-06  
> **首发平台**：Windows 10/11（AMD64），预留 macOS / Linux 支持

## 0. 实现状态核对（2026-11-06）

> 本节记录当前仓库与 v1.0 方案的差异；路线图勾选状态见 §10。文档其余章节保留原始设计意图。

**已完成**：M0、M1、M2 全部；M3 的 File History / Blame、Cherry-pick / Revert / Reset、分支与提交对比、交互式 Rebase 计划、冲突检测与 Base/Ours/Theirs 解决面板、Hunk / 行级 Stage；M4 的命令面板、可配置快捷键、会话恢复、全屏与紧凑行高、10,000 Commit 基准。

**本轮新增（IDEA 对齐批次 1 / 批次 2，2026-11-08）**：

- **批次 1**（`8edd0ea`）：Add to .gitignore 右键动作、状态栏/编辑器编码与换行符显示及 CRLF/LF 切换、提交消息历史下拉、Issue Navigation 链接（Commit Details 级）、Diff 标签会话持久化。
- **批次 2**：Tag 管理（lightweight/annotated 创建、删除确认、无 force 推送、detached 检出，`internal/git/tags.go`）；Pull 策略（Fast-forward only / Merge / Rebase + Autostash，`PullOptions`）；Push 选项（annotated tags `--follow-tags`、显式 re-point upstream、force-with-lease 受保护流程——确认对话框 + 后端强制本地 remote-tracking ref）；分支面板 ↑ahead/↓behind 同步徽标（`BranchSyncCounts`）。
- **测试**：`internal/git/tags_test.go`（tag 生命周期与 push 保护、pull 三策略与 autostash、push upstream/force-with-lease 场景、同步计数）、Vitest 51 项、`tests/batch2-smoke.mjs` 与 desktop-smoke（基础+M2+Worktree+MVP+M4，M2 段已适配 Pull 按钮并加固 amend 点击竞态）。

**与方案的偏差（事实记录）**：

- §2：未引入 Tailwind CSS 4 与 Reka UI；样式为手写 CSS + CSS Variables，菜单使用原生 `<details>` 与自研控件。
- §2 / §9：设置与最近仓库保存在 WebView `localStorage`，未建立 `internal/settings` 的 JSON 文件层。
- §1.3 / §14：后续迭代已实现工作区文本编辑器、GitHub 发布和交互式 Rebase 计划，超出「首版不做」清单；「不要提前实现 Push / Rebase」的阶段性约束已被后续需求取代。
- §10 M4：多仓库同时管理、Windows 安装包 / 签名 / 自动更新、macOS / Linux 发布，以及冷启动、内存、60 FPS、10 万提交实测仍未完成；`build/config.yml` 仍是 Wails 模板占位信息。

**验证（Windows 10 / Go 1.27.3 / Wails 3.1.0 / Node 22 冒烟）**：`go test ./...`、`go vet ./internal/... .`、`npm --prefix frontend run typecheck`、`npm --prefix frontend test`（51 项）、`wails3 generate bindings -ts -i`、`node tests/batch2-smoke.mjs` 与 `GITVELA_SMOKE_MVP=1 GITVELA_SMOKE_M4=1 GITVELA_SMOKE_M2_REMOTE=<未使用路径> node tests/desktop-smoke.mjs <一次性克隆>` 均通过。注意：`go test ./...` 不要与前端测试并行运行；`GITVELA_SMOKE_M2_REMOTE` 与 `GITVELA_SMOKE_WORKTREE` 必须传**未使用**的绝对路径（脚本自建 bare/worktree，预创建会导致 peer 克隆 HEAD 悬空与 `-b` 分支冲突）。

## 1. 项目概述

### 1.1 产品定位

GitVela 是一款面向开发者的**轻量级、独立 Git 图形客户端**。目标是保留 **IntelliJ IDEA / Rebased** 最有价值的 Git 管理体验，特别是：

**Git Log → 选择 Commit → 查看 Changed Files → 逐文件 Diff → 执行 Git 操作**。

GitVela 不构建工程索引，不提供语言服务器，不尝试替代 IDE，也不需要自带 Git 引擎。它通过本机 Git 与 Codex、Claude Code、IDE、终端共享仓库状态，并强调 Worktree 协作。

### 1.2 产品原则

1. **IDEA 式体验**：提交图谱、变更文件列表、左右 Diff、操作快捷键与上下文菜单。
2. **轻量优先**：有限加载、虚拟列表、按需读取、可取消后台任务、最小依赖。
3. **Git 行为可靠**：以系统 Git CLI 为准，尊重用户配置、凭证和 Hooks。
4. **不会破坏仓库**：只读操作优先；危险写操作明确确认；支持撤销时不夸大保证。
5. **与 AI 工具共存**：外部进程更新 Commit、Index、Worktree 后能够检测并刷新。
6. **Windows 优先，架构跨平台**：系统差异集中在平台适配层。

### 1.3 首版不做

- 代码编辑器、语言服务、项目编译、插件商店。
- GitHub/GitLab 账号体系、PR 管理、云同步。
- AI 聊天和自动生成提交信息（保留未来扩展点）。
- 完整三方 Merge 编辑器与交互式 Rebase（后续版本）。
- 数据库；初期设置使用 JSON 文件即可。

## 2. 技术选型

| 层级 | 选型 | 用途 |
| --- | --- | --- |
| 桌面应用 | Wails 3（稳定版本，初期固定版本） | 原生窗口、系统集成、Go ↔ JS 绑定 |
| 后端 | Go 1.26+ | 仓库服务、Git 进程调度、缓存与监听 |
| 前端 | Vue 3 + TypeScript + Vite | 桌面 UI 与交互 |
| 状态管理 | Pinia | 仓库选择、工作区、提交选择与布局状态 |
| 样式 | Tailwind CSS 4 + CSS Variables | 紧凑、深色/浅色、主题令牌 |
| 基础 UI | Reka UI + 少量自研控件 | 右键菜单、下拉、弹窗、命令面板 |
| 长列表 | @tanstack/vue-virtual | Commit Log、文件变更列表 |
| Git 图谱 | Canvas 2D + Go Lane 布局算法 | 提交节点、分支线、合并线 |
| Diff | CodeMirror 6 + @codemirror/merge | 双栏/单栏文本比较 |
| Git 内核 | 本机 Git CLI | 读取、比较、提交、分支和 Worktree |
| 文件监听 | fsnotify + 事件防抖 + 定期校验 | 外部 Git / Codex 操作感知 |
| 配置 | JSON | 最近仓库、主题、面板尺寸、用户偏好 |
| 测试 | Go testing + Vitest + 端到端测试 | Git 核心、界面交互、真实仓库回归 |

**技术决策**：首版不引入 `go-git`、`libgit2` 或数据库。先把 Git CLI 解析、调度和错误处理做好；在性能数据证明必要后，才替换热点读取路径。

**落地偏差（2026-11-06）**：Tailwind CSS 4 与 Reka UI 未引入，实际使用手写 CSS + CSS Variables、原生 `<details>` 与少量自研控件；配置未走 JSON 文件，而是 WebView `localStorage`。其余选型（Wails 3.1.0、Go、Pinia、TanStack Virtual、CodeMirror 6 Merge、fsnotify、Canvas 图谱、系统 Git CLI）与方案一致。

## 3. 界面与交互

### 3.1 主窗口布局

```text
┌────────────────────────────────────────────────────────────────────┐
│ GitVela  [仓库 ▼] [分支 ▼]           Fetch  Pull  Push  Settings │
├───────────────────┬────────────────────────────────────────────────┤
│ Repositories      │ Git Log                         Search  Filter  │
│ ├ main repo       │ ┌────────────────────────────────────────────┐ │
│ └ worktrees       │ │ graph │ commit message │ author │ date     │ │
│                   │ │   ●   │ fix: ...       │ user   │ ...      │ │
│ Branches          │ │  ●│   │ feat: ...      │ user   │ ...      │ │
│ ├ main            │ │   ●   │ merge ...      │ user   │ ...      │ │
│ ├ feature/xxx     │ └────────────────────────────────────────────┘ │
│ └ origin/main     ├─────────────────────┬──────────────────────────┤
│                   │ Changed Files       │ Diff Viewer              │
│                   │ M src/history.go    │ BEFORE  │ AFTER          │
│                   │ A src/graph.go      │ - old   │ + new          │
│                   │ R old → new         │         │                │
├───────────────────┴─────────────────────┴──────────────────────────┤
│ Status: main · clean                            Git 2.x · Ready     │
└────────────────────────────────────────────────────────────────────┘
```

### 3.2 主视图

| 视图 | 主要内容 | 第一版 |
| --- | --- | --- |
| Git Log | 所有分支提交图、Hash、作者、时间、标签、搜索 | 必做 |
| Commit Details | 提交元信息、父提交、Changed Files | 必做 |
| Diff Viewer | 双栏/统一 Diff、行号、语法高亮、变更导航 | 必做 |
| Local Changes | 工作区状态、Stage/Unstage、提交 | 第二阶段 |
| Branches | 本地/远程分支、切换、创建、比较 | 第二阶段 |
| Worktrees | 列表、创建、打开、删除 | 第二阶段 |
| File History / Blame | 单文件历史与逐行归属 | 第三阶段 |
| Advanced Git | Cherry-pick、Rebase、Merge 冲突 | 第三阶段 |

### 3.3 关键交互约束

- 单击 Commit：加载 Changed Files；默认保留或选中第一个文件（可在设置中切换）。
- 单击文件：加载文件差异；双击可最大化 Diff 或打开专用 Diff Tab。
- 上下方向键：切换提交；`Alt+Up/Down` 可跳到前后变更（快捷键可配置）。
- Changed Files：目录树与平铺模式切换，新增/修改/删除/重命名状态标识。
- 支持窗口分隔条拖动、面板折叠、全屏、深浅主题、紧凑行高。
- Commit 上下文菜单：复制 Hash、复制消息、创建分支、比较提交；写操作分阶段启用。
- Git 操作必须展示执行中、完成、失败、冲突等可见状态，不允许静默丢失错误。

## 4. 系统架构

```text
Vue 3 UI
  ├─ Repository / Branches / Worktrees
  ├─ Virtualized Git Log + Canvas Graph
  ├─ Changed Files + Diff Viewer
  └─ Pinia / View Models / Request State
             │
             ▼
Wails 3 Generated Bindings + Events
             │
             ▼
Go Application Services
  ├─ RepositoryService
  ├─ HistoryService ── GraphLayout
  ├─ DiffService
  ├─ StatusService
  ├─ BranchService
  ├─ WorktreeService
  └─ OperationService
             │
             ▼
Git Core
  ├─ GitRunner (exec.CommandContext)
  ├─ Argument / Repository Validation
  ├─ Per-repository Write Queue
  ├─ Output Parsers
  ├─ LRU Cache / In-flight Request Control
  └─ fsnotify Watcher / Invalidation
             │
             ▼
Installed Git CLI → Local .git / Worktrees
```

### 4.1 分层职责

- **前端**：仅展示视图状态和发送语义化调用，不拼 Git 命令字符串。
- **Application Services**：验证请求，调度 Git 核心，输出稳定 DTO。
- **Git Core**：封装系统 Git 的命令执行、解析、错误映射和仓库状态变更。
- **基础设施**：配置、缓存、仓库监听、日志、平台打开目录等。

### 4.2 GitRunner 原则

- 使用 `exec.CommandContext` + 参数数组，绝不把输入拼接到 shell 中执行。
- 每个请求携带 `context.Context`，支持取消、超时；给 stdout/stderr 设定限制或流式处理。
- 使用明确的 Git 工作目录或 `git -C <repo>`；校验目录、可信仓库状态、权限。
- 路径放在 `--` 之后，按需使用 `-z` 二进制安全解析，不能靠按行拆分路径。
- **同一仓库写操作串行化**，但不假设可以阻止其他 Git 客户端并发写入。
- 对 `index.lock`、认证错误、未完成 Merge/Rebase、网络错误等进行错误分类。
- 使用 Git 配置/Credential Manager/SSH/签名与 Hooks；交互式认证应专门设计 UI 与终端回退。
- 危险操作（Reset Hard、强制 Push、删除未清理 Worktree）必须要求用户明确确认。

## 5. Git Log 与图谱实现

### 5.1 读取方案

初版按需加载每批约 200 个 Commit。Git 元数据可采用 NUL 分隔格式输出，例如：

```bash
git log --all --topo-order -n 200 -z \
  --format='%H%x00%P%x00%an%x00%at%x00%s'
```

- 解析提交 Hash、父提交、作者、时间、消息。
- 分支和标签引用单独查询，关联到 Commit Hash。
- 提供搜索：文本、作者、日期、分支；搜索最好由 Git 原生过滤能力实现。
- `--topo-order` 确保拓扑顺序，但不负责绘制轨道。
- 后续分页需要稳定快照与连续 Lane 状态，避免新增提交或切页导致连线跳变。

### 5.2 Lane 布局算法

1. 读取每个 Commit 的父节点数组。
2. 维护一组活跃 Lane（轨道），为当前提交分配 x 轴位置。
3. 对每个父节点建立连接，合并交叉轨道并回收无效 Lane。
4. 返回每行节点坐标与连线片段，跨页保留延续状态。
5. Canvas 仅绘制视口及预加载缓冲区；列表文字由虚拟列表绘制。

**正确性测试**：直线历史、分叉、Merge、Octopus Merge、跨页连接、孤立根提交、多个 Ref 指向同一 Commit、浅克隆。

## 6. Changed Files 与 Diff

### 6.1 变更文件清单

普通 Commit：指定父提交与当前 Commit 比较：

```bash
git diff --name-status -z -M <parent-commit> <commit>
```

- 根提交：与空树比较。
- Merge 提交：默认第一父提交，并允许切换父提交查看不同视角。
- 使用 `--numstat -z` 或等效查询获取增删行数（按需加载）。
- 支持重命名/复制、类型变更、子模块、特殊路径；不要用 `\n` 作为文件名分隔符。

### 6.2 Diff 读取与展示

- 读取旧版 Blob 和新版 Blob；文本文件交给 CodeMirror 6 MergeView。
- Git Patch/Hunk 用于操作与差异权威判断；CodeMirror 可独立负责文本可视化。
- 必须区分未暂存、已暂存、指定两个 Commit 的三个比较来源。
- 对二进制、图片、LFS 指针、过大文件做专门预览或降级。
- 对含大量行的大文件限制渲染量，允许用户显式继续加载。
- Diff 视图关闭后释放不再使用的 EditorView，防止内存累计。

## 7. 外部变更与 Worktree 协作

```bash
git worktree list --porcelain -z
git status --porcelain=v1 -z
```

- 监听工作树、Git metadata（注意 Worktree 的 `.git` 可能是指向 gitdir 的文件）。
- 监听事件需要防抖；文件监听仅作“可能已变更”通知，最终以 Git 查询结果为准。
- 外部 Codex/IDE/命令行提交之后，更新分支、Log、Status 与 Worktree。
- 使用请求版本号或 `requestId` 忽略旧响应，防止快速切换提交出现旧 Diff 覆盖新 Diff。
- `git worktree remove`、`prune` 等操作显示风险并检查工作区状态。

## 8. 数据模型（示例）

```ts
interface Commit {
  hash: string;
  parents: string[];
  subject: string;
  author: string;
  authorTime: number;
  refs: string[];
}

interface CommitPage {
  commits: Commit[];
  nextCursor?: string;
  hasMore: boolean;
  graphVersion: string;
}

interface ChangedFile {
  path: string;
  oldPath?: string;
  status: 'A' | 'M' | 'D' | 'R' | 'C' | 'T';
  additions?: number;
  deletions?: number;
}

interface DiffRequest {
  repositoryId: string;
  oldRevision: string;
  newRevision: string;
  oldPath?: string;
  newPath: string;
}

interface DiffResult {
  binary: boolean;
  truncated: boolean;
  oldContent?: string;
  newContent?: string;
  errorCode?: string;
}
```

真实 Go DTO 通过 Wails 3 Bindings 生成类型，前端避免维护第二套重复定义。

## 9. 代码目录建议

```text
gitvela/
├── main.go
├── go.mod
├── build/                     # Wails 生成构建配置
├── internal/
│   ├── app/                   # Wails Services（IPC 适配）
│   ├── git/
│   │   ├── runner.go
│   │   ├── repository.go
│   │   ├── history.go
│   │   ├── graph.go
│   │   ├── diff.go
│   │   ├── status.go
│   │   ├── branch.go
│   │   ├── worktree.go
│   │   └── operations.go
│   ├── model/
│   ├── cache/
│   ├── watcher/
│   ├── task/
│   └── settings/
├── frontend/
│   ├── bindings/              # Wails 自动生成，禁止手改
│   ├── src/
│   │   ├── components/
│   │   │   ├── git-log/
│   │   │   ├── changed-files/
│   │   │   ├── diff/
│   │   │   ├── repository/
│   │   │   └── layout/
│   │   ├── stores/
│   │   ├── composables/
│   │   ├── views/
│   │   ├── styles/
│   │   ├── App.vue
│   │   └── main.ts
│   └── package.json
├── tests/
│   ├── fixtures/
│   └── integration/
└── docs/
    ├── architecture.md
    └── roadmap.md
```

## 10. 路线图

### M0 — 工程底座

- [x] 使用 Wails 3 Vue + TypeScript 模板初始化。
- [x] 配置 Pinia、主题、日志、错误边界与设置管理（设置改为 `localStorage`）。
- [x] 实现 GitRunner，校验 `git --version`，提供仓库打开对话框。
- [x] 定义 Wails Service 与稳定 DTO，生成前端绑定。
- [x] 添加 Go 单元测试、前端类型检查与构建检查。

### M1 — IDEA 风格的只读 Git MVP（第一交付目标）

- [x] 选择本地仓库，展示仓库和分支信息。
- [x] Git Log 列表虚拟化、增量加载、搜索。
- [x] 提交 DAG Lane 算法与 Canvas 绘制。
- [x] 选择提交查看 Changed Files（包含 Merge/根提交）。
- [x] CodeMirror 双栏 Diff + 文件变更导航（`Alt+Up/Down`、hunk 导航）。
- [x] 窗口分隔条、主题、键盘导航、布局持久化。
- [x] 外部操作更新后刷新状态，过期请求不会覆盖当前视图。

### M2 — 日常操作

- [x] Local Changes、Stage / Unstage（先整文件）。
- [x] Commit、Amend、Stash。
- [x] Create / Checkout / Rename / Delete Branch。
- [x] Fetch、Pull、Push（包含认证、错误反馈）。
- [x] Worktree 列表、创建、打开、删除。

### M3 — 高级 Git

- [x] 文件历史和 Blame。
- [x] Cherry-pick、Revert、Reset。
- [x] 分支和提交对比；多提交选择尚未实现。
- [x] 交互式 Rebase 计划、Merge 冲突检测与 Base/Ours/Theirs 解决面板（非完整三方 Merge 编辑器）。
- [x] Hunk/行级 Stage、子模块跳转与 LFS 指针展示（未实现真实 LFS smudge）。

### M4 — 产品化

- [ ] 大仓库性能基准（已有 10,000 Commit 基准）、内存与冷启动分析（缺冷启动、内存、60 FPS、10 万提交实测）。
- [x] 会话恢复、命令面板与可配置快捷键；多仓库同时管理尚未实现。
- [x] 紧凑行高、窗口全屏（§3.3 补充项）。
- [ ] Windows 安装/便携包、代码签名、自动更新评估。
- [ ] macOS/Linux 兼容测试与正式发布。

## 11. 性能目标（待实测）

| 场景 | 初期验收目标 |
| --- | --- |
| 冷启动到可交互 | ≤ 1.5 s（指定测试机器） |
| Git Log | 首批约 200 条，后台继续加载 |
| 日志滚动 | 目标 60 FPS，10 万提交仓库回归测试 |
| 已缓存提交切换 | 可见反馈 ≤ 100 ms |
| 普通文件 Diff | 尽量在 300 ms 内完成首屏加载 |
| 总内存 | 争取 ≤ 200 MB，统计 Go + WebView2 全进程树 |
| Git CLI 调用 | 有取消、超时与错误反馈，不阻塞 UI |

> 所有数值均为**设计目标，不是性能承诺**；应记录 CPU、磁盘、仓库数据量、Git 版本、冷热缓存状态。若指标与功能质量冲突，以正确性和数据安全为先。

## 12. 测试策略与验收

### Git 核心样本

- 根提交、线性历史、合并提交、八爪鱼合并。
- 重命名/复制、空文件、二进制、中文与特殊字符路径。
- CRLF、无末尾换行、超大 Diff、Git LFS、子模块。
- Detached HEAD、浅克隆、工作区和 Index 同时修改。
- 多 Worktree、正在 Merge/Rebase、`index.lock` 冲突。
- 外部 Git 进程在读取时修改分支和 HEAD。

### M1 完成标准

1. 打开真实仓库，展示正确的提交图、分支和标签。
2. 选中某次提交，能查看准确的 Changed Files。
3. 双击变更文件查看之前/之后版本的 Diff。
4. Merge、根提交、重命名和特殊文件名都能正确展示。
5. 频繁切换提交不会造成旧结果覆盖新结果、UI 长时间冻结或编辑器泄漏。
6. 能在 Windows 上进行开发构建和可发布构建。
7. Go 测试、TypeScript 类型检查和前端构建通过。

## 13. 项目初始化（Windows PowerShell）

使用 **Wails 3.1.0 稳定版**作为项目起点，避免意外使用 nightly。版本固定后由项目统一控制升级。

```powershell
go version
node --version
git --version

# 安装固定版本的 Wails 3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
wails3 doctor

# 创建 Vue + TypeScript 项目
wails3 init -n gitvela -t vue
cd gitvela

# 安装前端基础依赖
cd frontend
npm install pinia @tanstack/vue-virtual
npm install @codemirror/state @codemirror/view @codemirror/merge
cd ..

# 开发模式
wails3 dev
```

后续按需安装 Tailwind CSS / Reka UI，遵循当时的官方安装步骤。Wails 3 的生成绑定存放在 `frontend/bindings/`，不要照搬 Wails 2 的 `wailsjs` 用法。

## 14. 给 Codex 的实施指令

> 请在现有 GitVela 仓库内按照本文件开发。先实现 M0 和 M1，不要提前实现 Push、Rebase 或 AI 功能。每完成一个模块都要有可运行的演示入口与自动测试。优先保证 IDEA 风格的 Git Log → Changed Files → Diff 操作链，确保大仓库可用、特殊 Git 状态正确、外部 Codex 修改仓库时界面能及时刷新。避免不必要的依赖，不直接复制许可不兼容的第三方项目源码。提交实现时列出修改文件、测试命令和已知限制。

## 15. 参考项目与文档

- [Wails 3 官方文档](https://v3.wails.io/)
- [Wails 3 版本日志](https://v3.wails.io/changelog/)
- [Wails 3 服务绑定](https://v3.wails.io/features/bindings/methods/)
- [Vue 3 官方文档](https://vuejs.org/)
- [TanStack Virtual](https://tanstack.com/virtual/latest)
- [CodeMirror Merge](https://github.com/codemirror/merge)
- [Git 官方文档](https://git-scm.com/docs)
- [Rebased](https://github.com/DetachHead/rebased)（交互参考）
- [SourceGit](https://github.com/sourcegit-scm/sourcegit)（功能与流程参考）
- [Gitnuro](https://github.com/JetpackDuba/Gitnuro)（交互参考）

> 说明：GitVela 为暂定产品名，公开发布前应检查 GitHub 仓库名、软件名称和商标冲突。借鉴其他项目时须遵守各自许可证。
