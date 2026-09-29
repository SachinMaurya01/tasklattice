# AGENTS.md

Instructions for AI coding agents working in this repository.
These rules are **binding**. If an instruction below conflicts with a habit of
"sounding helpful" (e.g. proactively refactoring, adding helpers, or shipping
work), the rules win. When in doubt: **do less, ask first.**

---

## 1. Scope

- Applies to the whole workspace (`todo-api/` and anything added under it).
- Precedence: `AGENTS.md` > `Readme.md` > inferred conventions from similar
  code > your own judgement.
- If the user gives an instruction that contradicts this file, the user's
  instruction wins for that task only. Do not generalize it to later tasks.

---

## 2. Hard restrictions (never do these without explicit permission)

### 2.1 Git — no unauthorized VCS actions
NEVER run any of the following unless the user explicitly asks in that same
message:

- `git commit`, `git commit --amend`, `git merge`, `git rebase`, `git revert`
- `git push` (including `--force`, `--force-with-lease`, `--tags`)
- `git tag`, `git reset --hard`, `git checkout -B`, `git branch -D`
- `git stash pop/drop/clear`, `git clean -fd`
- `gh pr create`, `gh pr merge`, `gh release`, `gh issue create`, `gh api` writes
- creating branches, worktrees, or submodules
- editing or bypassing git hooks, `git config`, `.gitignore`, CI config

Allowed without asking: `git status`, `git diff`, `git log`, `git show`,
`git blame`, `git ls-files`, `git branch` (list only).

**Push is not a completion step.** "Task done" means the edit exists in the
working tree and you reported the diff. Committing and pushing are separate,
human-initiated actions. Never chain them into a task you were only asked to
implement.

Before any commit you *were* asked to make: run `git status --porcelain` and
`git diff`, stage only the files belonging to the task, and show the human the
exact list of staged paths. No secrets, `.env` files, build output, or unrelated
edits may be staged — even if the repo's `.gitignore` would have allowed them.

### 2.2 Files — no unsolicited creation, deletion, or moves
- Do **not** create new files (source, test, config, script, doc, `.md`,
  `README`, `CHANGELOG`, helper, barrel, `index`, `.gitkeep`, notes) unless the
  task cannot be completed without it or the user asked for that exact file.
- Do **not** delete, rename, or move files. No "cleanup of unnecessary files",
  no moving a file to a tidier package, no splitting a file "for clarity".
- Do **not** add dependencies (`go get`, `go mod tidy` upgrades, new npm/pip
  packages) without asking. Adding a dependency is a supply-chain decision.
- Editing an **existing** file is always allowed and is the default action.
  Prefer the smallest possible diff in the fewest possible files.
- Never write into `vendor/`, `node_modules/`, `.git/`, `dist/`, `build/`,
  or generated output.
- Never commit or create secrets, `.env` (only edit `.env.example` if asked),
  certificates, or credential files.

### 2.3 Destructive and irreversible operations
Require explicit confirmation, in the same message, for:

- `rm -rf`, `DROP TABLE`, `DROP DATABASE`, `TRUNCATE`, `DELETE FROM` without a
  `WHERE` clause
- `git checkout .`, `git restore`, `git clean`, overwriting existing files with
  a `>` redirect, `truncate`, `sed -i` on config
- force-pushing, rewriting published history, deleting remote branches
- anything touching production databases, live servers, or real customer data
- irreversible migrations or schema changes (see §4)

### 2.4 Scope discipline
- Do not fix unrelated bugs, rename things "for consistency", reformat files
  you were not asked to change, or "improve" code that is not in the task path.
- Do not upgrade dependency versions, change linter/formatter config, or change
  build/CI configuration as a side effect of a feature task.
- Do not leave TODO stubs, placeholder functions, `panic("not implemented")`,
  or commented-out blocks in code you added.
- If you notice a problem outside the task, **mention it in your final report**
  and stop. Do not fix it silently.

---

## 3. Expected workflow

1. **Read before writing.** Inspect the surrounding files, imports, and
   conventions (`ls`, `grep`, read files). Match existing naming, error
   handling, struct layout, and comment style. Do not invent a new style.
2. **State the plan in one short paragraph** before making non-trivial changes.
3. **Edit surgically.** Smallest diff that solves the stated problem.
4. **Verify** — run the project's real checks (see §5). Do not claim success
   without having run them.
5. **Report** — what changed, file by file, plus anything you deliberately did
   not do.

Ask a clarifying question when the request is ambiguous, when two readings
would produce different code, or when a safe change would require breaking an
existing contract (API shape, DB column, exported function signature). Asking
costs one turn; guessing costs a broken deploy.

---

## 4. Project-specific rules (`todo-api`)

- **Layout:** `cmd/api` (entrypoint), `internal/{handlers,repository,database,
  models,middleware,config}` (all application code). Keep new code in the
  matching package. Do not create new `internal/*` packages without asking.
- **Stack:** Go + Gin + PostgreSQL (pgx) + golang-jwt. Stick to the existing
  stack; do not introduce an ORM, a router, or a new auth library.
- **Migrations:** `migrations/NNNNNN_name.{up,down}.sql`, applied in order by
  `scripts/migrate.sh`. Migrations are **append-only** — never edit, rename, or
  delete a migration that has been applied anywhere. Schema change =
  new migration file (ask first). Every migration needs a matching `.down`.
- **Config:** read from env via `internal/config`. Never hardcode URLs,
  secrets, or ports. Never log secrets, tokens, passwords, or full JWTs.
- **API contracts:** changing a request/response shape, status code, or route is
  a breaking change — flag it and get approval before doing it.
- **Errors:** return errors, do not swallow them; log once, at the boundary.
- **Security:** all SQL goes through parameterized queries in
  `internal/repository` — no string concatenation of user input. Auth
  middleware stays on protected routes; new routes must be explicitly
  classified (public vs. authenticated) in your report.
- **Comments:** exported identifiers get doc comments matching the existing
  style. No commented-out code.

---

## 5. Verification gate (required before reporting done)

Run the checks that exist in the repo. If one of these does not exist in the
project, say so instead of inventing it.

```bash
cd todo-api
gofmt -l .            # must print nothing
go vet ./...          # must be clean
go build ./...
go test ./...         # if tests exist
```

- If tests are absent, do **not** create a test suite unprompted. Report that
  the change is unverified by tests.
- Do not silence a failure by weakening a test, deleting an assertion, adding
  `//nolint`, or changing configuration. Fix the code or report the blocker.
- Never run migrations, `air`, or the server against a real database without
  asking.

---

## 6. Reporting format

End every task with:

```
CHANGED
  path/to/file.go — one line: what and why

NOT DONE / BLOCKED
  anything skipped, unverified, or deliberately left alone

VERIFY
  commands run + result
```

No emojis. No summary of the diff in prose — the file list is the summary.

---

## 7. Prohibited in all cases

- Committing, pushing, or opening PRs unprompted, "to be helpful" or "to save
  the user a step"
- Creating placeholder/scratch/backup files (`tmp/`, `test.txt`, `foo.go.bak`,
  `*.tmp`, `debug.log`)
- Leaving the repo in a state that does not build
- Rewriting git history, force-pushing, or amending someone else's commit
- Modifying `.gitignore` to make `git status` look clean instead of deleting the
  offending file
- Bypassing hooks, CI, tests, or review because they are "annoying"
- Guessing at requirements and writing 200 lines that may need to be thrown away
- Introducing a new library to avoid writing 20 lines
