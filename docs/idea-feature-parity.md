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
| G1 | **Tag 管理**：create/delete/push tag、checkout tag | branch/tag 操作、GitTag | ✅ 已交付（2026-11-08）：lightweight/annotated 创建、删除确认、无 force 推送、detached 检出；`internal/git/tags.go` + Tags 面板右键菜单 | **已交付** |
| G2 | **push 选项**：force-with-lease（受保护流程）、push tags、push 后设置 upstream、推送预览列表 | push/GitPushDialog（GitVela 有意 `--no-force`，sync.go:136） | ✅ 已交付（2026-11-08）：push 确认对话框内 Push annotated tags（`--follow-tags`）、显式 re-point upstream、force-with-lease（必选确认 + 后端要求本地 remote-tracking ref，禁止无租约强推）；无推送预览列表 | **已交付** |
| G3 | **pull 选项**：merge / rebase（GitPullSettings）、--ff-only、autostash | config/GitPullSettings.java | ✅ 已交付（2026-11-08）：Sync 面板 Pull 策略单选（Fast-forward only / Merge / Rebase）+ Autostash 复选框；`PullOptions{Mode,Autostash}` | **已交付** |
| G4 | **Interactive rebase**：todo 编辑（squash/fixup/reword/edit/drop）、continue/abort | rebase/（GitRebaseTodoEditor） | ✅ 已交付（2026-11-09）：计划对话框 Pick / Reword / Edit / Squash / Fixup / Drop + 上下移动；reword 与带消息的 squash 用 `pick|fixup`＋`exec git commit --amend -F <临时文件>` 实现（不进编辑器，`RebaseStep.Message` 按序落盘、结束即删）；`edit` 停在提交上，Continue/Abort 走 Console；`internal/git/rebase.go` | **已交付** |
| G5 | **add to .gitignore**（右键未跟踪文件）、.gitignore 校验提示 | ignore/ | ✅ 右键 `??` 文件 → Add to .gitignore（仓库根追加锚定 pattern、幂等去重、拒绝 .git/越界路径）；无校验提示 | **已交付**（高频小功能） |
| G6 | **incoming/outgoing 指示**：分支面板 ↑/↓ 计数、Log 中 merged 高亮 | branch/GitBranchIncomingOutgoingManager.java、DeepComparator | ✅ 已交付（2026-11-08）：本地分支行 ↑ahead/↓behind 徽标 + gone 标记（`BranchSyncCounts` 基于 remote-tracking refs，随仓库刷新更新；Log 内 merged 高亮未做） | **已交付**（面板级） |
| G7 | **Cherry-pick 多选 + 继续/中止流** | cherrypick/、CherryPickedCommitsHighlighter | ✅ 已交付（2026-11-09）：Ctrl/Shift 多选后菜单 `Cherry-pick N selected…`，按历史顺序（旧→新）重放并在确认框逐条列出；拒绝重复目标、合并提交与超过 100 条；`--empty=drop` 跳过已应用提交；冲突由 sequencer 保留，Console 的 Continue / Abort 继续剩余提交（`internal/git/history_operations.go`、`commitSelection.ts`） | **已交付** |
| G8 | **Squash merge**、squash/fixup 提交 | merge/、branch/GitMergeOperation | ✅ 已交付（2026-11-09）：历史右键 Squash/Fixup into parent…（`FirstParent` 取祖父为 rebase 基，父提交 pick、目标 squash/fixup，默认消息=两 subject 拼接，可编辑）；Merge 对话框 Mode 增加 Squash（只暂存合并结果、不建合并提交） | **已交付** |
| G9 | **Clone from URL**（仓库初始化流程内） | checkout/GitCloneDialog | ✅ 已交付（2026-11-09）：顶栏 `Clone…`、欢迎页与命令面板入口；目标须不存在或为空、父目录须存在、拒绝 `-` 开头地址与非法分支名、10 分钟超时，成功后直接打开；`internal/git/clone.go` | **已交付** |
| G10 | Fetch 选项：prune、tags 模式（GitFetchTagsMode） | config/GitVcsPanel.kt:355 | ✅ 已交付（2026-11-09）：Sync 面板 Fetch options——Prune 删除远端已删除分支的 remote-tracking ref；Tags 三档（指向已取对象 / 全部 `--tags` / 不取 `--no-tags`，默认遵循 `remote.<name>.tagOpt`） | **已交付** |
| G11 | stash diff 查看 / stash 应用时恢复 index（--index） | stash/ | ✅ 已交付（2026-11-09）：`StashEntry.Parent`（`stash^1`）+ Shelf 面板 View Diff（按钮与右键）以 `stash^1 → stash` 打开比较；恢复 index 已有 "Restore staged changes" | **已交付** |
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
   提交：`8edd0ea`。
2. **批次 2（Git 安全写操作扩展）**：✅ 已完成（2026-11-08）——G1 tag 管理、G3 pull 选项、
   G2 push 选项（force-with-lease 走 ConfirmationDialog + 后端 remote-tracking ref 校验）、
   G6 分支同步徽标。测试：`internal/git/tags_test.go`（tag 生命周期/push 保护/pull 三策略/
   autostash/upstream/lease/sync 计数）、`tests/batch2-smoke.mjs` 桌面冒烟；
   desktop-smoke M2 段已适配新 Pull 按钮并加固 amend 点击竞态。
3. **批次 3（深水区）**：✅ 已完成（2026-11-09）——G4 interactive rebase（reword/edit/squash
   消息）、G8 squash into parent / squash merge、G9 clone from URL、G11 stash diff。
   测试：`internal/git/advanced_test.go` 新增 reword+squash、edit 停止后 continue、首个
   提交不能 squash、clone 校验与重复目标等用例；`tests/batch3-smoke.mjs`（squash→reword→
   edit+continue→clone→stash diff 全链路）。
4. **批次 4（多选与远端选项，2026-11-09）**：✅ 已完成——G7 多选 cherry-pick（顺序重放、
   冲突后 sequencer 继续剩余提交）、G10 fetch 选项（prune + tags 三档）、仓库目录消失后
   停止 5 秒轮询（`CheckRepositoryPath`）；界面图标改用 Ant Design 官方线性图标
   （iconfont.cn 同源，MIT，内联 SVG）。测试：`TestCherryPickMultipleCommitsInHistoryOrder`、
   `TestCherryPickMultipleContinuesAfterAConflict`、`TestFetchOptionsPruneAndTags`、
   `TestCheckRepositoryPath`、`tests/batch4-smoke.mjs`（多选 cherry-pick→fetch 选项→
   删除仓库后无重复告警）。**剩余**：Directory Mappings（多根仓库）、G12 amend author/date、
   G13 bisect/GC、E3 Ctrl+F / Go to Line。
5. **桌面冒烟脚本加固（2026-11-09）**：`tests/desktop-smoke.mjs` 改为在虚拟列表里滚到
   首个提交（仓库提交数已超过一屏），多选比较改为"重建选择→开菜单→点比较"的可重试
   单步（后台刷新会折叠多选并关闭菜单）；`batch3-smoke` 用 `.git/rebase-merge` 判断
   rebase 停点，避免 `git status` 的临时 index.lock 与应用的 rebase 抢锁。
6. **明确不做**：Mercurial/Perforce/Subversion/Gitee/GitLab 集成、Changelists（与真实
   index 语义冲突，spec §0.3）、IDE 提交前代码检查类动作。

> 行为对齐原则不变：以 Rebased 源码佐证交互细节，以用户截图定布局；无法确认的先标待验证。
