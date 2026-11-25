# gitincognitor

Personal notes (docs-local, CLAUDE.local.md, research) tend to sit untracked inside shared and open source repos. Hiding them needs a `.gitignore` edit your team would see, and a disk loss deletes them. gitincognitor keeps them in a real `.incognito/` folder inside each project, hides it through `.git/info/exclude`, and backs it up to a private remote you own. No tracked file in the project changes. The design reference is [docs/v0-spec.md](docs/v0-spec.md).

## Install

```
go install github.com/glincker/gitincognitor/cmd/git-incognitor@latest
```

Prebuilt binaries for macOS and Linux (amd64 and arm64) are on the Releases page. Put the binary on your PATH as `git-incognitor` so `git incognitor ...` works.

## Quickstart

```
git incognitor init --remote <private-url>
cd project && git incognitor link
git incognitor adopt docs-local
git incognitor sync
git incognitor restore --all --remote <url>
```

Requires git 2.42 or newer (`git worktree add --orphan`). Any git remote works.

## How it works

One store repo at `~/.incognito/store` holds one orphan branch per project (`proj/<name>`). `link` checks that branch out as `<project>/.incognito/`. A project can use its own dedicated private repo instead with `link --store dedicated --remote <url>`.

## Commands

All commands accept `--json`. Exit codes: 0 ok, 1 error, 2 unsafe state that needs a human.

| Command | Behavior |
| --- | --- |
| `init --remote <url>` | Create the store, push a seed `main`, write the registry. |
| `link` | Create branch and worktree, write exclude entries and the `CLAUDE.local.md` pointer, register the project. |
| `adopt [--dry-run] <folder>` | Move an existing folder into the notes, push, then remove the original. A `.incognitoignore` file in the folder (gitignore syntax) keeps reference clones or bulky files where they are. `--dry-run` previews and writes nothing. |
| `sync [--all]` | Commit and push notes. No-op when clean. Never force-pushes. |
| `restore [--all]` | Recreate the store, worktrees, exclude entries and pointers after a crash. |
| `status` | Per project: linked, dirty, ahead, last sync. |
| `doctor` | Check excludes, tracked notes, remote collisions and registry health. |
| `unlink` | Remove the worktree and exclude entries, keep the branch. |
| `version` | Print the version. |
| `hooks install` | Opt in to a Claude Code Stop hook and a launchd timer running `sync --quiet`. |

## Safety rules

1. Never writes to a tracked project file, only `.git/info/exclude` and untracked files.
2. `link` and `doctor` refuse a notes remote that matches any remote of the project.
3. With `link --guard`, a pre-commit and pre-push guard blocks any staged path under `.incognito/`.
4. `sync` never force-pushes and never deletes branches.
5. `adopt` removes the original folder only after a verified push.

## Status

v0.x. Tested on macOS and Linux, not tested on Windows. Known limits:

- The remote collision check does not normalize ssh ports or `insteadOf` rewrites.
- The private-remote check cannot verify hosts other than GitHub.
- Commit and push guards are installed only with `link --guard`.

## Development

```
go vet ./... && go test ./...
go build -o bin/git-incognitor ./cmd/git-incognitor
```

Tests use temp directories and local bare remotes only. No network.

## License

MIT, copyright GLINCKER.
