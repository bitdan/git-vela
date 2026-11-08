# GitVela agent guide

## Scope and desktop stack

- GitVela uses Wails v3. Check the version pinned in `go.mod` and the existing project Taskfiles before using a Wails API or CLI flag. Read the nearest subsystem code and work within the owning component.
- Keep Git command execution and validation in `internal/git`, Wails services in `internal/app`, and workspace interaction in the Vue frontend. Reuse the existing service bindings and store.
- When a Go binding or event changes, regenerate the TypeScript interfaces with `wails3 generate bindings -ts -i` from the repository root and review the generated diff. Keep the contract and its generated output together.
- Use Rebased 1.1.23 as the desktop UI and interaction reference. Keep the Windows executable and version-resource display name `git-vela`.

## Confirmation and Git safety

- Put every second-step confirmation in a dedicated modal dialog. Use `ConfirmationDialog.vue` for GitVela confirmations, including push and publication, deletion, discarding edits, and confirming or aborting history operations. Keep the main workspace usable after cancellation.
- Identify the exact target and consequence in the dialog. Focus Cancel by default. Escape and the close button cancel the pending action. Execute the mutation only from the explicit confirmation button, and prevent duplicate submission while Git is running.
- Retain Go-side validation of the repository and target. A dialog is a UX boundary and backend checks are still required for refs, revisions, paths, and concurrent repository changes.
- Verify write actions in temporary Git repositories or disposable clones. Keep native desktop smoke builds on an isolated WebView2 profile.

## Local verification

- Use Node 24. Run `npm --prefix frontend run typecheck`, `npm --prefix frontend test`, and `go test ./...`. Run Git-heavy suites sequentially on Windows.
- Use the desktop smoke procedure in `README.md`. For a confirmation flow, check the modal, initial focus, cancellation without mutation, and successful explicit confirmation on a temporary target.

## Upstream reference

- [Wails v3.1.0 repository guidance](https://github.com/wailsapp/wails/blob/v3.1.0/AGENTS.md)
- [Wails v3.1.0 module guidance](https://github.com/wailsapp/wails/blob/v3.1.0/v3/AGENTS.md)
- [Wails dialog APIs](https://v3.wails.io/reference/dialogs/)
- [Rebased 1.1.23](https://github.com/DetachHead/rebased/releases/tag/1.1.23)
