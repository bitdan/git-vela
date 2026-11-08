# GitVela ↔ IntelliJ IDEA 布局与交互差距分析（阶段 A）

> 基准：`docs/idea-ui-final-spec.md` + `docs/reference/idea-git-layout.png`（S0，1907×1027）
> 行为证据来源：Rebased 1.1.23（intellij-community fork）`platform/vcs-log/impl` 源码 + GitVela 当前代码（本次审查时 HEAD）。
> 本轮仅分析，不含实现结论；实现按"分阶段改造计划"推进。

## 1. 组件映射（spec §6 → GitVela 现状）

| spec 组件 | GitVela 现有实现 | 位置 | 判定 |
|---|---|---|---|
| AppShell | `App.vue`（1718 行，模板+状态+Git 写操作全集中） | frontend/src/App.vue | 超载，需持续拆分 |
| MainToolbar | `.topbar`（brand、Git 菜单、仓库路径输入、Browse、Commands、Settings） | App.vue:1301-1365 | 有；缺当前分支徽章/仓库 dirty 展示，仓库路径输入框不属于 IDEA 顶栏语义 |
| ActivityBar | `.workspace-rail`：内容网格内的**文字按钮**（Files/Commit，可拖动换序） | App.vue:1470-1478, styles.css:88 | **不符**：非 IDEA 竖排图标工具窗口条 |
| CommitToolWindow | `WorkingChanges.vue` 挂在 `workspace-sidebar`（sidebar==='changes'），无标题栏 | App.vue:1490 | 位置对（左上）；缺 "Commit" 工具窗口标题与窗口操作；宽度 235–315px，S0 约 33% 宽 |
| WorkingChanges / CommitForm | `WorkingChanges.vue`（勾选=SetStaged 真实 index 语义、Amend、Commit/Commit and Push、提交信息固定底部） | components/changed-files/WorkingChanges.vue | **符合** spec §2.3/4.3 核心要求 |
| EditorWorkspace | `.upper` 单一 preview-panel + 模式按钮（Working file/Unstaged/Staged/Commit diff/Editor） | App.vue:1501-1601 | **不符**：非多标签；同一时刻只有一个内容视图；编辑器仅支持单文件（`editorPath` 单值） |
| EditorTabs | 无 | — | **缺失**（S0 右上为 7 个文件标签并列） |
| GitToolWindow | `#history-dock` `.lower` 三栏（branch-panel + CommitLog + ChangedFiles），无窗口级标题/标签/最大化 | App.vue:1606-1674 | 三栏结构对；缺窗口级 chrome（S0 有 [Git][Log][Console] 标签与右上窗口按钮）、缺最大化/恢复 |
| GitToolWindowTabs / GitActionToolbar | 无（History 显隐开关藏在 preview header 的 "History" 按钮） | — | **缺失** |
| BranchesPanel | branch-panel（Local/Remote 树、过滤、New…、右键菜单） | App.vue:1607-1669 | 结构符合；单击行为见 §3-I02 |
| CommitLog（含 GraphCell） | `CommitLog.vue`（虚拟化、Graph 列、过滤/搜索、右键菜单、pin+compare、列显隐） | components/git-log/CommitLog.vue | **基本符合**；单选模型，无 Ctrl/Shift 多选（见 §3-I04） |
| ChangedFiles | `ChangedFiles.vue`（树/平铺、右键、双击=maximize） | components/changed-files/ChangedFiles.vue | 文件树符合；双击行为见 §3-I03；commit-summary 混在其头部 |
| CommitDetails | **无独立面板**；摘要（hash/作者/主题/父提交）内嵌在 ChangedFiles 头部 commit-summary | ChangedFiles.vue:89-100 | **缺失**：S0/IDEA 为右栏"上文件树、下详情"上下堆叠 |
| StatusBar | `.statusbar`（仓库路径+分支、Ready/Git version） | App.vue:1713-1716 | 基本符合；缺编码/行列（次要，延后） |

## 2. 布局差异（对照 S0）

| # | 差异 | 证据 | 严重度 |
|---|---|---|---|
| L01 | 无最左侧竖向 Activity Bar（≈38px 图标条）；现为内容网格内文字按钮列 | styles.css:75 `grid-template-columns: 54px …`、App.vue:1470 | 高 |
| L02 | Editor 非多标签，无 EditorTabs 条；S0 右上为多文件标签并列 | App.vue:1503-1521 | 高 |
| L03 | 底部 Git 工具窗口无窗口级标题条（[Git][Log][Console] 标签、隐藏/最大化按钮） | App.vue:1606 | 高 |
| L04 | 右栏 Changed Files 与 Commit Details 未上下堆叠；摘要挤在文件树头部，不可拖拽分隔 | ChangedFiles.vue:82-100；Rebased `MainFrame.java:172` `OnePixelSplitter(true, DETAILS_SPLITTER_PROPORTION, 0.7f)` | 高 |
| L05 | Commit 工具窗口无标题栏；宽度默认 23%（S0≈33%） | App.vue:1479-1500 | 中 |
| L06 | 顶栏无当前分支徽章；分支信息只在状态栏 | App.vue:1301-1365 | 中 |
| L07 | 面板尺寸持久化不全：仅 `previewHeight` 持久化；sidebar 宽度、lower 三栏宽度、右栏上下比例均为 CSS clamp 写死 | preferences.ts（仅 dark/previewHeight/fileTree/density/commitFirst）；Rebased `MainFrame.java:60-62` 三个 splitter proportion 均持久化 | 高 |
| L08 | 默认上下分界 60%，spec/S0≈58% | preferences.ts:4 `defaultPreviewHeight = 60` | 低 |
| L09 | 底部窗口最大化（覆盖 Editor）缺失；只有 diffExpanded 单向扩展 | App.vue:947-962 | 中 |

## 3. 交互差异

### 已符合（本轮复核确认，不重复实现）

- T05/T18 右键菜单 + ConfirmationDialog 危险操作确认（CommitLog/分支/文件/Reset/Rebase/Discard/Abort）。
- T09 勾选 = 真实 index 语义（`SetStaged`），非前端假选中（WorkingChanges include → api）。
- T10/T11 Commit 与 Commit and Push 分离；Push 失败不影响本地 commit 结果展示（App.vue:814-843）。
- T12 提交输入框快捷键隔离（`isTextEntry`/`isCommitField`，App.vue:1116-1135）；T13 编辑器 Ctrl+F 归编辑器（`editorScopeShortcuts` + `.cm-editor` 判定）。
- T14 异步防竞态（`LatestRequest` 全链路）；T17 虚拟化（CommitLog/ChangedFiles/RepositoryFiles）。
- Graph 列可点击（GraphCell 有节点/标签交互；引用右侧布局开关）。

### 需改造

| ID | 差异 | 现状证据 | Rebased/IDEA 证据 | 改造 |
|---|---|---|---|---|
| I01 | 单击历史提交会把中央视图强制切到 commit diff，破坏 Editor/Working 视图（违反 T02/T04） | App.vue:1670-1673 `@select="view = 'commit'"`（CommitLog 与 ChangedFiles 均如此） | MainFrame.java:142-156：table selection → `VcsLogTableCommitSelectionListener` → `commitDetailsLoader` 只驱动 details/changes 更新，不抢占编辑器 | select 事件改为：仅当当前已是 commit diff 视图时保持；否则只更新右栏数据 |
| I02 | 分支**单击**打开操作菜单（与右键相同），非"选择/跳转"语义（T07 偏差） | App.vue:1650-1655 `@click="openBranchActions(...)"` | IDEA 分支单击=在 Log 中定位；菜单在右键/Shift+F10 | 单击→`jump(hash)`；右键/Shift+F10→菜单 |
| I03 | 双击历史文件=maximize diff，而非在 Editor 打开 Diff 标签（T03） | ChangedFiles.vue:37 `@dblclick="emit('maximize')"` | MainFrame.java:181-189 `FrameDiffPreview` + `VcsLogChangesBrowser.getDiffRequestProducer`（diff 经由 diff preview/编辑器承载，VCS_LOG_VIEW place） | 双击→聚焦 Editor 区的 Commit diff 标签（本轮共享单 Diff 标签；逐文件 Diff 多标签列为后续项） |
| I04 | 提交列表无 Ctrl/Shift 多选（T06）；以 pin+compare 两步比较替代 | CommitLog.vue:126-135 单选 `store.selected` | `VcsLogGraphTable` 标准 JTable 多选（MULTIPLE_INTERVAL_SELECTION） | 本轮保留 pin+compare，多选列为后续项（在报告中明示） |
| I05 | Editor 单文件：打开另一文件丢弃当前编辑器（有未保存确认，但无并行标签）（T04） | App.vue:404-441 `editor/editorPath` 单值；`withSavedEditor` 拦截 | S0 多标签并列；IDEA 每标签独立文档 | 实现多标签 EditorTabs（每标签独立 file/draft/dirty，v-show 保活） |
| I06 | Git 工具窗口无 Console/操作状态标签、无最大化/恢复（T16 部分） | App.vue:1444-1465 操作面板在顶部 daily-panel；historyVisible 无最大化 | MainFrame.java:245 `showDetails(boolean)`；工具窗口可最大化 | 增加 Log/Console 标签（Console 承载进行中操作/错误），最大化按钮 |
| I07 | 会话恢复缺 Editor 打开标签、各 splitter 尺寸（T15 部分） | session.ts:1-98（path/hash/file/sidebar/view/historyVisible） | splitter proportion 持久化（MainFrame.java:60-62） | 扩展 session + preferences 持久化字段 |

## 4. 分阶段改造计划

### 阶段 B — 布局骨架（对应 L01–L05, L07, L08）

1. 新增 `ActivityBar`：最左竖向图标条（Commit/Files/Worktrees/Git 定位 + 底部 Settings），替换 `.workspace-rail` 文字按钮。
2. Commit 工具窗口标题栏（"Commit" + 隐藏按钮）；sidebar 宽度改为可拖拽 splitter 并持久化。
3. Git 工具窗口标题条：Log/Console 标签 + 右侧最大化/隐藏按钮；Console 承载进行中操作与错误。
4. 右栏拆分：`ChangedFiles`（上）+ 新 `CommitDetails`（下），可拖拽比例持久化；ChangedFiles 移除内嵌 commit-summary。
5. lower 三栏宽度改为持久化 splitter（branches/files）；`defaultPreviewHeight` 58。
6. 顶栏加当前分支徽章（点击定位 Branches）。

### 阶段 C — Git Log 交互（对应 I01–I02, I06）

7. 单击提交/文件不再抢占 Editor 视图（I01）。
8. 分支单击=跳转 Log，右键=菜单（I02）。

### 阶段 D — Editor/Diff（对应 I03, I05, I07）

9. 多标签 EditorTabs：多文件并行编辑、脏标记、关闭/切换保活（I05）。
10. 双击历史文件→Editor 区 Commit diff 标签聚焦（I03，共享单 Diff 标签）。
11. 会话持久化扩展：editor 标签列表、最大化状态、全部面板尺寸（I07）。

### 验收方式

- `npm --prefix frontend run typecheck`、`npm --prefix frontend test`、`go test ./...`。
- 按 spec §7.2 T01–T18 手工核对；布局与 S0 同尺寸对照。
- 明示残余差距：提交多选（I04）、逐文件 Diff 多标签、状态栏行列信息、App.vue 进一步拆分。

## 5. 待验证项（无源码/截图证据，不凭空实现）

- 分支面板在 S0 中的默认宽度比例（截图测量≈20%，CSS 现为 clamp(145px,18%,260px)）— 以截图对照微调。
- Commit Details 是否需要完整 body 文本：Rebased `CommitDetailsListPanel` 展示完整 message；GitVela 有 `CommitMessage(hash)` API，本轮 Details 面板按需加载完整信息。

## 6. 实施结果（阶段 B/C/D，本轮已交付）

| 计划项 | 状态 | 实现 |
|---|---|---|
| ActivityBar（L01） | ✅ | 新增 `layout/ActivityBar.vue`：38px 竖向图标条（Commit/Files/Worktrees/Git + Settings），替换 `.workspace-rail` 文字按钮 |
| Commit 工具窗口标题（L05） | ✅ | sidebar 增加 "Commit/Files/Worktrees" 标题栏 + 隐藏按钮；宽度可拖拽并持久化（`gitvela.sidebarWidth`，200–520，默认 300≈S0 的 33%） |
| Git 工具窗口 chrome（L03/I06） | ✅ | `#history-dock` 增加标题条：Log/Console 标签 + 最大化/隐藏按钮；新增 `operations/GitConsole.vue` 承载进行中操作/冲突/错误（原顶部 advanced-panel 迁入，操作启动时自动切到 Console） |
| Changed Files + Commit Details 上下堆叠（L04） | ✅ | 新增 `git-log/CommitDetails.vue`（hash/作者/日期/主题/完整 message/父提交/refs/合并父比较），经可拖拽比例（`gitvela.detailsHeight`）堆叠在 Changed Files 下；ChangedFiles 移除内嵌 commit-summary |
| lower 三栏宽度持久化（L07） | ✅ | `gitvela.branchesWidth`（140–420，默认 230）、`gitvela.filesWidth`（200–480，默认 320），全部带键盘（方向键）调整 |
| 默认分界 58%（L08） | ✅ | `defaultPreviewHeight = 58` |
| 顶栏分支徽章（L06） | ✅ | 顶栏新增当前分支 chip，点击定位 Branches 过滤框 |
| 单击提交不抢编辑器（I01/T02） | ✅ | `handleHistorySelect`：编辑器/冲突/检查视图中不切换；其余显示 commit diff（与 IDEA 变更浏览器跟随选择一致，经冒烟实测校准） |
| 分支单击=跳转（I02/T07） | ✅ | 单击 `jump(hash)` 定位 Log；右键/Shift+F10 才出操作菜单 |
| 双击历史文件开 Diff（I03/T03） | ✅ | `ChangedFiles` 双击 → `openDiff` → Editor 区 commit diff 标签聚焦（Diff 为共享单标签，逐文件 Diff 多标签列为后续项） |
| 多标签 Editor（I05/T04） | ✅ | `editorTabs` 多实例 + preview-tabs 内文件标签条（脏标记 •、关闭需确认、v-show 保活滚动/撤销状态）；会话保存/恢复打开的标签与活动标签 |
| 会话持久化扩展（I07/T15） | ✅ | session 新增 `historyMaximized/editorFiles/editorActive`；preferences 新增 4 个布局尺寸键 |
| 提交多选（I04/T06） | ⏸ 未实施 | 保留 pin+compare 两步比较；真实 Ctrl/Shift 多选列为后续项 |

### 验证结果

- `npm --prefix frontend run typecheck` ✅
- `npm --prefix frontend test`：7 文件 35 用例全过 ✅
- `go test ./...`：`internal/git` ok（152s，Windows 顺序运行）✅
- 桌面冒烟（`wails3 dev -tags desktop_smoke` + `node tests/desktop-smoke.mjs`，隔离 WebView2 配置）：
  - 基础流程（Go 绑定/仓库/Graph 画布/Changed Files/只读 Diff）✅
  - `GITVELA_SMOKE_MVP=1`（一次性克隆：历史搜索、提交动作、设置、文件树、Diff 最大化）✅
  - 几何断言（activity bar/sidebar/preview/lower/branches/log/files 同线关系）通过 CDP 实测确认 ✅
- 残余差距：提交列表 Ctrl/Shift 多选、逐文件 Diff 多标签、状态栏行列信息、App.vue 进一步按 spec §6 拆分。
