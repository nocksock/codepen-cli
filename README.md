# codepen-api-prototyping / Hub

This is the hub of the `codepen-api-prototyping` repository, checked out on the orphan branch `hub`.

Every subfolder is a git worktree of the main repository. `./main` holds `main`.
New worktrees branch off `main` and can use sparse-checkout to materialize only
the paths they need. All subfolders are ignored by the hub.

## Tasks

Tasks are tracked with beans in `./.beans` on the hub branch. The beans CLI
searches upward for `.beans.yml`, so every worktree shares the same tasks. Commit
bean changes here in the hub, not in the worktrees.

Launchers in `.beans.yml` open a bean in an agent. The `worktree-*` ones run
`scripts/bean-worktree`: one worktree per bean at `./<bean-id>` on branch
`task/<bean-id>` off `main`. The others start in `./main`.

## Agent docs

Specs, plans, ADRs, session handoffs, `CONTEXT.md` and other notes written by agents live
here in the hub under `docs/` (and `CONTEXT.md` at the hub root), never inside a worktree.
Commit them on the hub branch. Claude Code also reads this file as `CLAUDE.md` from every
worktree below it.

## Available commands

    just default                    # Default: list available tasks
    just status                     # Show git status and the last 5 commits for every checked-out worktree
    just unworktree dir *args       # Usage: just unworktree <dir> [--force]
    just worktree dir branch *paths # e.g. just worktree apps-web feat/web apps/web packages/ui
    just worktrees                  # Show current worktrees

## Go development

Before any Go coding, review, debugging, troubleshooting, or setup task, load the `samber/cc-skills-golang@golang-how-to` skill first — it routes to whichever other Go skills the task needs.
