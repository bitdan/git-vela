# GitVela

GitVela is a Windows desktop client for local Git repositories. Browse history, compare files, stage or unstage whole files, and commit staged changes. The branch panel supports creating, switching, renaming, and safely deleting local branches, plus browsing tags. For a merge commit, select the parent to compare against. Selecting a commit opens the diff for the previously selected path when available, or the first changed file.

Select **Worktrees** in the repository sidebar to list and open linked checkouts. **New…** creates a checkout at an unused absolute path, using an available local branch or a new branch from the selected commit. To remove a linked checkout, open a different worktree and confirm removal in the panel. GitVela keeps the branch and requires an unlocked checkout on a branch with no modified, untracked, or ignored files. The list checks for external changes every five seconds while the panel is open.

Use **Files** and **Commit** on the left activity bar. The Project tree loads folders as you expand them. Right-click to create, rename, copy, move, or delete items, open their location, search within a folder, or open Git actions for a file. Open **Worktrees** from the top **Git** menu and **Branches** from the current branch button or Git menu. The right-hand preview sits above Git Log and Changed Files. Use **History** in the preview header to collapse or reopen both history panels.

Use the project name in the top bar to open a folder, enter a path, switch repositories, or clone a project. Open a file from **Files**, or use **Jump to Source** (**F4**) on a file row in the Git workspace. Right-click the editor for Git actions including inline **Annotate with Git Blame**, comparisons, file and selection history, and confirmed file commit or rollback. Open **⋯** in the tab strip for editing commands. The editor supports undo and redo, Find / Replace, and Ctrl+S. Saving updates the working copy and checks for changes made by other tools before replacing the file. It preserves UTF-8 BOM and consistent LF or CRLF line endings. The editor accepts UTF-8 regular files up to 512 KiB and 12,000 line breaks.

The **Files** sidebar and **Changed Files** start in tree view. From a working file, open **File History** or **Blame**. In Git Log, use a commit as a comparison base, compare revisions, cherry-pick, revert, or reset. Select several commits with Ctrl/Shift and choose **Cherry-pick N selected…** to replay them in history order (oldest first); a conflict stops the sequence and the Git operation console continues or aborts it. Right-click a commit to **Squash into parent…** or **Fixup into parent…**. Branch actions include comparison, merge, and an interactive rebase plan with reorder and Pick, Reword, Edit, Squash, Fixup, or Drop; a Reword or Squash step takes its commit message in the plan, and an Edit step stops the rebase so the Git operation console can continue or abort it. Merge accepts a **Squash** mode that stages the combined change without creating a merge commit.

When Git reports unmerged paths, select a file in Changes and open **Resolve conflict…** to inspect Base, Ours, and Theirs above the working-file editor. Save the resolution, mark it resolved, and continue or abort the Git operation. In an unstaged or staged text diff, use **Hunks** to stage or unstage whole hunks or selected lines of a modified tracked file.

Use **Search…** in Git Log to search the full history by message, author, branch, or commit date, or look up a commit by hash. Results are paginated and open in the same Changed Files and Diff panels. Right-click a commit or press **Shift+F10** on its row to copy its hash or message, or start a branch from it. Switch Changed Files between Flat and Tree, or double-click a file to maximize the diff. Use **Settings** for theme, file layout, and resetting panel sizes.

Git metadata and changes in the worktree root trigger a debounced refresh. A five-second check covers edits in nested working directories and filesystems without reliable notifications.

The top **Git** menu contains Commit, Fetch, Pull, Push, Stash, Worktrees, and repository setup. Click or right-click a branch to see its available actions, including showing its commit in Git Log, creating a branch from its tip, or checking out a local branch.

Use **Amend…** in Working Changes to edit the current commit message and include staged changes. The dialog shows the original author and date and can reset the author or add a `Signed-off-by` trailer. Review the HEAD warning before confirming. **Stash…** saves tracked changes, optionally includes untracked files, and offers Apply, Pop, Rename, and confirmed Drop; the working-file context menu stashes a single file. **View Diff** in the Shelf panel compares a stash with the commit it was created on. Apply and Pop start from a clean working tree and can restore staged changes. Pop keeps the stash when applying fails.

To copy an existing repository, select **Git → Clone from URL…** (also on the welcome screen and in the command palette). Enter a URL or local path, pick an existing parent folder and a new folder name, and optionally a branch. GitVela refuses a target that already has files, an option-like address, or an invalid branch name, and opens the clone when it finishes.

Use **Git → Fetch…**, **Pull…**, **Push…** and **Update Project…** (**Ctrl+T**) to synchronize with a remote. Fetch only updates remote-tracking branches: it can prune remote-tracking branches deleted on the remote, choose whether to download all tags, only tags that point to fetched commits, or no tags, and fetch every configured remote at once. Pull integrates a remote branch you pick (the upstream is preselected) into the checked-out branch with Merge, Rebase, Fast-forward only, No fast-forward or Squash; it requires a clean working tree unless you choose Autostash, and protects ignored files from being overwritten. Update Project fetches the checked-out branch's upstream and merges or rebases onto it (Settings → Git → Update method), stashing local changes automatically. In the branch list, **Update** on the checked-out branch does the same; on any other local branch it only fast-forwards that branch to its upstream without touching HEAD or the working tree, and refuses a diverged branch or one checked out in another worktree. **Push…** works for any local branch and uses that branch's own upstream. A branch without an upstream can publish under its current name and set its upstream on success. On a remote branch, **Pull into 'current' Using Rebase/Merge** integrates it into the checked-out branch. Protected branch patterns under **Settings → Version Control** refuse a force push to a matching remote branch. Results appear as notifications in the lower right. Authentication uses your configured Git credential helper and SSH setup. If Git needs terminal interaction, complete it in a terminal and retry.

To publish a new project, select **Git → Initialize Project…** and choose its existing folder. Review the Working Changes, stage the files and create the first commit, then select **Git → Publish to GitHub…**. Check the signed-in account, review the name and visibility, and choose **Create and push**. Visibility defaults to private. The flow creates the repository under your personal GitHub account, adds `origin`, and pushes the current branch. **Git → Manage Remotes…** lets you add, edit, or remove remote configuration, including when you already created the repository elsewhere.

Press **Ctrl+K** to open the command palette and run Git operations, switch workspace views, reopen a recent repository, or jump to a branch or tag. The top-right **Settings** button opens a dialog with a left-hand section list (Appearance, Editor, Version Control, Git, Startup & Window, Keymap); **Keymap** records new bindings, refuses conflicts, and can unbind or reset each command; `Alt+Up/Down` moves between changed files and `Alt+Shift+Up/Down` between diff hunks. Settings also offers compact rows, fullscreen, and reuse of the last repository. On startup GitVela restores that repository with its selected commit (or comparison range), file, sidebar and view; **Forget saved session** clears it. The **Roots** menu maps up to 20 repositories for this workspace — switching a root reopens it and brings back the tabs it had when you left it. **Git → Bisect…** walks between a known bad and a known good revision and names the first bad commit, and **Git → Cleanup (git gc)…** shows a diagnostic count of unreachable commits before you confirm maintenance. Git decides which objects to remove based on repository retention settings. **Settings → Git** can point GitVela at a different Git executable.

A preview stops at 512 KiB or 12,000 lines by default. **Load full preview** raises the limit to 8 MiB or 120,000 lines for that one file. PNG, JPEG, GIF, WebP and BMP changes render as Before / After images; other binary content still falls back to a notice.

See [GitVela Specification](docs/GITVELA_SPEC.md)for product requirements, architecture, and interaction standards.

## Windows development

Run from the project root in a PowerShell session with Go, Git and fnm available. Wails and its JavaScript runtime are pinned to 3.1.0. Node.js 24 is selected per session.

```powershell
fnm env --shell powershell | Out-String | Invoke-Expression
fnm install 24
fnm use 24
$env:GOTOOLCHAIN = 'go1.27.3'
$env:GOBIN = Join-Path $PWD '.tools/bin'
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
$gitRoot = Split-Path (Split-Path (Get-Command git).Source)
$env:Path = "$env:GOBIN;$gitRoot/usr/bin;$env:Path"
wails3 doctor
wails3 dev
```

Stop the dev process before running a production build. Both modes write to the same output directory.

```powershell
wails3 build
go test ./...
go vet ./...
npm --prefix frontend run typecheck
npm --prefix frontend test
```

The production executable is at `bin/git-vela.exe`. Bindings are generated by the Wails build. Run the Go and frontend test suites one at a time: on a busy Windows machine, parallel runs can trip the 20 s Git CLI timeout used by the repository fixtures.

## Desktop smoke test

Start `wails3 dev -tags desktop_smoke` to enable WebView2 debugging on port 9331 using an isolated profile under `.tools`. In a second PowerShell session with fnm initialized and Node 24 selected, run `node tests/desktop-smoke.mjs` from the project root. The smoke test opens this repository and checks the initial commit's real Git diff through the desktop UI. To exercise Worktree creation, switching, and removal, pass a disposable clone on branch main as the first argument and set GITVELA_SMOKE_WORKTREE to an unused absolute directory path. The smoke test creates a branch in the clone. Set `GITVELA_SMOKE_MVP=1` to also check history search, commit actions, settings, and diff maximization. To test Amend, Stash, Fetch, Pull, and Push, use a disposable clone and set `GITVELA_SMOKE_M2_REMOTE` to an unused absolute path for a local bare test repository. Set `GITVELA_SMOKE_PROJECT=1` to test initialization, first commit, remote management, and the publication form. Optionally set `GITVELA_SMOKE_GITHUB_LOGIN` to verify the signed-in account and review screen against GitHub without creating a repository. Set `GITVELA_SMOKE_EDITOR=1` with a disposable clone to verify editing, undo, save, staging, and protection against external edits. Set `GITVELA_SMOKE_M3=1` with a disposable clone to test branch comparison, rebase, conflict resolution, file history, blame, and hunk staging. Set `GITVELA_SMOKE_M4=1` to verify the command palette, configurable shortcuts, compact rows and session persistence. Run `node tests/batch3-smoke.mjs` against the same instance to check squash into parent, reword, edit plus continue, clone from URL, and the stash diff on a throwaway fixture. Run `node tests/batch4-smoke.mjs` to check multi-commit cherry-pick, fetch prune and tag modes, and that polling stops when the open repository folder disappears.

```powershell
go test ./internal/git -run '^$' -bench BenchmarkReadHistoryAndDiff -benchmem -benchtime 5x
```

The benchmark creates a temporary 10,000-commit repository. It measures Go and Git CLI time, excluding WebView rendering.
