# GitVela ↔ IDEA Version Control 功能对齐清单

> 参考：IDEA Settings → Version Control（Changelists / Commit / Confirmation / Directory Mappings /
> File Status Colors / Issue Navigation / Log / Shelf / Git / GitHub 等，用户截图 2026-11-08）
> 与本地 IntelliJ Platform 源码 `F:\project\rebased`（`plugins/git4idea/backend/src/*`、
> `platform/vcs-impl`）。GitVela 是独立实现，只对齐可合法复现的行为。
> 本轮为差距盘点 + 分级路线，不含全部实现。

## 1. IDEA 设置页逐项对照（用户截图）

| IDEA 设置页 | GitVela 现状 | 证据（rebased） | 优先级 |
|---|---|---|---|
| **Changelists**（逻辑改动分组） | ❌ 有意不做：spec §0.3 规定勾选=真实 index 语义，changelist 是逻辑分组，两者模型冲突。替代：多选勾选提交 | changes/（ActiveChangelistsPanel） | 低（需产品决策） |
| **Commit**（提交前动作：格式化/优化 import/代码分析、commit message 历史） | 部分：Amend、Commit and Push 分离 ✅；**message 历史下拉 ✅（G14）**；提交前动作 N/A（无 IDE 检查） | checkin/（CommitOptionsPanel） | 批次 1 已交付 message 历史 |
| **Confirmation**（新文件 add 提示、删除 remove 提示、"不再询问"） | 部分：新文件在 Working Changes 列出、手动 Stage（不走提示流）；删除文件回滚有确认 ✅ | changes/（DeferredCommitRequestProcessor…） | 低 |
| **Directory Mappings**（多仓库根映射） | ❌ 单仓库根。Worktree 覆盖部分场景；多根/monorepo 多项目 ❌ | ProjectLevelVcsManager（platform） | 中（monorepo 支持） |
| **File Status Colors** | 部分：M/A/?? 状态徽标有颜色（`.file-status[data-status]`）；不可配置、缺 ignored/conflict 专属色 | vcs-impl FileStatusColor；changes/FileStatusColorManager | 低 |
| **Issue Navigation**（提交消息中 issue id → 链接） | ✅（Commit Details 主题/全文渲染 issue 链接；pattern + URL 模板在 Settings → Version Control 配置，$0/$1..$9 占位符）。Log 行内链接未做（虚拟列表行按钮内嵌 a 标签存在嵌套交互问题） | platform IssueNavigationConfiguration | 已交付（Details 级） |
| **Log**（历史 UI 设置） | ✅ 过滤/搜索/列显隐/Graph 交互齐备；"always expand commit message"等小项部分缺 | platform/vcs-log/impl | 低 |
| **Shelf** | 部分：Stash 列表/保存/Pop ✅（stash 实现）；缺：查看 shelf/stash 的 diff、rename、部分 shelf（IDEA shelf=补丁+排除文件）、stash 与 shelf 合并视图 | stash/、changes/shelf | 中 |
| **Git**（设置页选项） | 部分：暗色/密度/行高/文件树 ✅。缺：Git 可执行文件选择 ❌（固定系统 git）、auto-fetch ❌（有快照轮询 watch.go）、protected branches ❌、CRLF/detached HEAD/大文件/坏文件名警告 ❌（大文件有预览降级）、cherry-pick 后缀 ❌、credential helper N/A | config/GitVcsPanel.kt:101-316 | 分项：见 §3 |
| **GitHub / Gitee / GitLab / Mercurial / Perforce / Subversion** | GitHub 发布/登录部分 ✅（publish 流程）；其余超出产品范围，不做 | github/（社区插件） | N/A |

## 2. Git 交互功能对照（git4idea 行为）

### 已对齐（复核过）

分支 create/checkout/rename/delete、远程分支删除、merge、rebase+冲突解决、cherry-pick、
revert、reset（soft/mixed/hard 确认）、discard、stash、worktree 增删切换、remotes 管理+fetch、
pull/push（Commit and Push 分离、Push 失败不影响本地提交）、tag 列表、blame、文件历史、
hunk 级暂存、多选比较（051ccc3）、逐文件 Diff 多标签（051ccc3）、Console（操作/错误）、
会话恢复（布局+标签）。

### 缺失（按价值排序）

| # | 功能 | rebased 证据 | GitVela 现状 | 优先级 |
|---|---|---|---|---|
| G1 | **Tag 管理**：create/delete/push tag、checkout tag | branch/tag 操作、GitTag| 仅 tag 列表显示（refs/tags）；无创建/删除/推送/检出 | **高**（成本中） |
| G2 | **push 选项**：force-with-lease（受保护流程）、push tags、push 后设置 upstream、推送预览列表 | push/GitPushDialog（GitVela 有意 `--no-force`，sync.go:136） | push 固定安全参数；无 force/tags/upstream 选项 | **高**（force 须带确认对话框） |
| G3 | **pull 选项**：merge / rebase（GitPullSettings）、--ff-only、autostash | config/GitPullSettings.java | pull 固定策略 | 高 |
| G4 | **Interactive rebase**：todo 编辑（squash/fixup/reword/edit/drop）、continue/abort | rebase/（GitRebaseTodoEditor） | rebase+冲突 ✅ 但无 todo 编辑 | 高 |
| G5 | **add to .gitignore**（右键未跟踪文件）、.gitignore 校验提示 | ignore/ | ✅ 右键 `??` 文件 → Add to .gitignore（仓库根追加锚定 pattern、幂等去重、拒绝 .git/越界路径）；无校验提示 | **已交付**（高频小功能） |
| G6 | **incoming/outgoing 指示**：分支面板 ↑/↓ 计数、Log 中 merged 高亮 | branch/GitBranchIncomingOutgoingManager.java、DeepComparator | ❌ 无同步状态徽标 | 中 |
| G7 | **Cherry-pick 多选 + 继续/中止流** | cherrypick/、CherryPickedCommitsHighlighter | 单提交 ✅；多选菜单已禁用（T06） | 中 |
| G8 | **Squash merge**、squash/fixup 提交 | merge/、branch/GitMergeOperation | ❌ | 中 |
| G9 | **Clone from URL**（仓库初始化流程内） | checkout/GitCloneDialog | 有 init/发布；无 URL 克隆 | 中 |
| G10 | Fetch 选项：prune、tags 模式（GitFetchTagsMode） | config/GitVcsPanel.kt:355 | fetch 基础 ✅，无选项 | 低 |
| G11 | stash diff 查看 / stash 应用时恢复 index（--index） | stash/ | stash 保存/Pop ✅，无 diff 查看 | 中 |
| G12 | Amend 时显示原 message 的 author/date 选项、sign-off | commit/（GitCheckinOptions） | amend ✅ 基础 | 低 |
| G13 | Bisect、cleanup/GC、fetch --all | 零散 | ❌ | 低 |
| G14 | Commit message 模板（commit.template）与历史记忆 | checkin/CommitMessageHistory | ✅ 历史记忆（每仓库 localStorage 最近 20 条，提交表单 History 下拉）；模板 ❌ | 历史已交付；模板待定 |

### 文件编辑对齐（IDEA editor）

| # | 项 | 现状 | 优先级 |
|---|---|---|---|
| E1 | 状态栏显示编码/换行符（IDEA 右下角），切换 CRLF/LF、编码 | ✅ 状态栏与编辑器工具条均显示 UTF-8(+BOM)/换行符；换行符可点击切换（干净文件立即转换，脏文件下次保存生效，SaveTextFile 新增 lineEnding 参数）；编码仅显示（编辑器只支持 UTF-8） | **已交付** |
| E2 | Diff 标签会话持久化（重开仓库恢复打开的 diff） | ✅ diffTabs（key/path/hash/parent/index/range/expanded）与激活 key 入 session（≤12 条），恢复后激活标签惰性重载 comparison；标签持久化补齐 | 已交付 |
| E3 | Editor 内 Ctrl+F 搜索面板（CodeMirror search 已有？确认交互一致）、Go to Line（Ctrl+G） | 部分缺 | 中 |
| E4 | Soft-wrap、字号、tab 大小设置 | Settings 有密度/行高；无编辑器项 | 低 |
| E5 | 未保存提示（脏标记+关闭确认）✅、外部修改保护 ✅、reload ✅ | 已对齐 | — |

## 3. 建议实施批次

1. **批次 1（高频低成本）**：✅ 已完成（2026-11-08）——G5 add to .gitignore、E1 状态栏编码/换行
   切换、G14 message 历史、Issue Navigation（Details 级）、E2 diff 标签持久化。
   提交：`<批次1提交号>`。
2. **批次 2（Git 安全写操作扩展）**：G1 tag 管理、G3 pull 选项、G2 push 选项（force-with-lease
   必须走 ConfirmationDialog + 目标校验）、G6 分支同步徽标。
3. **批次 3（深水区）**：G4 interactive rebase、G8 squash、G9 clone、G11 stash diff、
   Directory Mappings（多根）。
4. **明确不做**：Mercurial/Perforce/Subversion/Gitee/GitLab 集成、Changelists（与真实
   index 语义冲突，spec §0.3）、IDE 提交前代码检查类动作。

> 行为对齐原则不变：以 Rebased 源码佐证交互细节，以用户截图定布局；无法确认的先标待验证。
