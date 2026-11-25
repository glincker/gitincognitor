# gitincognito v0 spec

Private notes that live inside any repo, never show up in it, and are backed up to a private remote you own. CLI only. OSS, MIT.

## Problem

Personal notes (docs-local, CLAUDE.local.md, research) sit untracked in shared and OSS repos. Hiding them needs a `.gitignore` edit the team would see, and a disk loss deletes them. Agents must find them without following a second repo.

## Model

- One **store** repo (`~/.incognito/store`) with one private remote.
- One **orphan branch per project** in the store (`proj/<name>`).
- `link` checks the branch out as a real folder at `<project>/.incognito/` via `git worktree add --orphan`.
- `<project>/.git/info/exclude` lists `.incognito/` and `CLAUDE.local.md`. No tracked file in the project changes.
- A project can use its own dedicated private repo instead (`store: dedicated`). Same commands, different remote.
- The tool only runs `git`. Any remote works (GitHub, GitLab, Bitbucket, Gitea, self-hosted). Creating a remote is an optional helper (`gh`, `glab`); the core takes a URL.

## Commands

All accept `--json`. Exit codes: 0 ok, 1 error, 2 unsafe state (needs a human).

| Command | Behavior |
| --- | --- |
| `init --remote <url>` | Create the store, push a seed `main`, write the registry. Refuses a remote it cannot verify as private when a provider API is available. |
| `link [--store dedicated --remote <url>]` | Create branch and worktree, write exclude entries, write the `CLAUDE.local.md` pointer, register the project. Idempotent. |
| `adopt [--dry-run] <path>` | Move an existing folder (for example docs-local) into the project's branch, then link. Refuses if the target is non-empty and tracked by the project repo. `--dry-run` writes nothing (works before `init`), reports copy and skip counts and bytes, and exits 2 on a refusal. |
| `sync [--all]` | Commit and push notes. No-op when clean. Never force-pushes. |
| `restore [--all]` | Clone the store if missing, recreate each worktree, exclude entries and pointer from the registry. Prunes stale worktrees. |
| `status` | Per project: linked, dirty, ahead or behind, last sync. |
| `doctor` | Checks: exclude entries present, nothing under `.incognito/` tracked by the project, remote is not the project's own remote, registry matches worktrees. |
| `unlink` | Remove worktree and exclude entries, keep the branch. |
| `hooks install` | Explicit opt-in. Adds a Claude Code Stop hook and a launchd timer that run `sync --quiet`. Never run implicitly by `link`. |

## Ignoring paths on adopt

`<folder>/.incognitoignore` (gitignore syntax) lists paths adopt leaves exactly where they are, for example reference clones or bulky files.

- Matching is done by `git check-ignore` in a scratch repo, one batch per directory level. An ignored directory is never descended into.
- Ignored paths skip the nested `.git` and symlink checks, the copy, the tracked verification and the deletion.
- After a verified push only copied files are deleted, then directories that became empty. A directory holding ignored content stays.
- `.incognitoignore` is copied into the notes but also stays in the folder, so a rerun keeps the rules and is a no-op.

## Registry

`~/.incognito/store/registry.yml`, committed on the store's `main` (the store checkout is its only copy):

```yaml
version: 1
projects:
  - name: thesvg
    path: ~/G-Development/GLINRV6/thesvg
    store: central
    branch: proj/thesvg
```

## Auto-sync (day 1)

`sync --quiet` exits 0 to hooks, commits locally when the push fails, retries next run, and skips when the last sync is newer than `GITINCOGNITO_SYNC_DEBOUNCE_SECS`. Triggers: Claude Code Stop hook (agent-written notes) and a launchd timer (hand edits).

## Safety rules

1. Never write to a tracked project file. Only `.git/info/exclude` and untracked files.
2. `link` and `doctor` fail (exit 2) if the notes remote equals any remote of the project repo.
3. A pre-commit and pre-push guard (installed by `link --guard`, via the project's local hooks) blocks any staged path under `.incognito/`.
4. `sync` never force-pushes and never deletes branches.
5. No secrets are scanned or stored by the tool. `doctor` warns on `.env`-like filenames in the notes.

## Agent integration

- The `CLAUDE.local.md` pointer names `.incognito/`, the commands, and "run `git incognito sync` after writing notes".
- No MCP server. JSON output from the CLI is the agent interface.

## Implementation

- Go, single static binary `git-incognito` (so `git incognito ...` works). Shells out to `git`; requires git 2.42 or newer (`worktree add --orphan`).
- Layout: `cmd/`, `internal/{git,registry,store,project,guard}`.
- Tests use temp repos and local bare remotes only. No network.

## Verified by spike (2026-11-24, git 2.49)

Outer `git status` stayed clean with the exclude entry. The orphan branch pushed to a bare remote. After deleting store and project, a fresh clone plus `worktree add` restored the notes. The seed `main` must be pushed or the remote's HEAD dangles.

## Out of scope for v0

Desktop app, encryption, team sharing of notes, conflict-merge UI, Windows path support (design for it, test later).

## Open items

- Worktree metadata is per store clone. `restore` rebuilds it, so it is not backed up.
