# CodePen API v2: the ADHD version

> **TL;DR:** It's **git push with CI attached**, for your own Pens. You write source files, CodePen compiles them, and sometimes the compiler **edits your files** on the way. Always re-read after writing.

---

## 🧠 The mental model (30 seconds)

```
your files ──PUT──▶ new version ──▶ build ──▶ (patches?) ──▶ another version (= head)
   (src)                                                          │
                                                       build.files (dist) ──▶ preview / deploy
```

- **Version** = a commit (source files). Every file write creates one.
- **Build** = CI run for a version. Same ID as its version.
- **Patches** = the compiler fixing your files (adds `tsconfig.json`, `type="module"`, `package.json` deps, formatting).
- **Deployment** = putting a version on a public `*.codepen.app` URL.

## 🔑 Basics

| Thing     | Value                                                      |
| --------- | ---------------------------------------------------------- |
| Base URL  | `https://api.codepen.io/v2`                                |
| Auth      | `Authorization: Bearer cp_user_…` (or `X-API-Key`)         |
| Surfaces  | REST (snake_case) **or** GraphQL (`/v2/graphql`): same powers |
| Scope     | **Only your own stuff.** Other people's Pens → 404           |
| Key types | `cp_user_` = you, `cp_team_` = a team (team must be PRO)   |

## 🧰 What you can actually do

| Area            | Verbs                                     | Endpoint                                             |
| --------------- | ----------------------------------------- | ---------------------------------------------------- |
| **Pens**        | list, filter, create, read, update, trash, fork | `/pens`, `/pens/:id`, `/pens/:id/fork`         |
| **Files**       | read all/one, write many as one version, write one, delete one/dir | `/pens/:id/files[/*path]`   |
| **Versions**    | list, inspect (with files), restore old → head | `/pens/:id/versions`, `/versions/:id`, `…/restore` |
| **Builds**      | poll status, read diagnostics + compiled output | `/builds/:id`                                  |
| **Deploy**      | deploy a version, undeploy                | `PUT/DELETE /pens/:id/deployment`                    |
| **Settings**    | auto_deploy, auto_run, auto_save, indent, tab size | `PATCH /pens/:id/settings`                  |
| **Collaborators** | invite, change role, remove, list invites | `/pens/:id/collaborators`, `/pens/:id/invites`     |
| **Collections** | CRUD, add/remove Pens                     | `/collections[/:id/pens/:pen_id]`                    |
| **Assets**      | upload, rename/replace, read, delete (hosted files, not in any Pen) | `/assets`         |
| **Screenshots** | free JPEGs at 320/512/800/1280 px wide    | `pen.screenshot_urls`                                |

Classic Pens: **read-only** (as files). Versions/builds/deploys are **2.0 Pens only**.

## 🔁 The one loop you must get right

1. **Write** with `expected_version_id` = the head you based it on.
2. **Wait** for the build (the write waits 45s by default; `wait` up to 75, or `0`).
3. If `Queued`/`Running` → poll `/builds/:id`. If `Canceled` with `next_id` → poll **that**.
4. **Re-read** files from `/versions/:build.version_id`. Not your local copy.
5. Use that version ID as the next `expected_version_id`.

Skip step 4 and you'll undo the compiler's fixes and it will just redo them.

| `build.status`           | Meaning                                         |
| ------------------------ | ----------------------------------------------- |
| `Queued` / `Running`     | keep polling                                    |
| `Success`                | done ✅                                          |
| `Failed`                 | done, files **saved anyway**, read `build.errors` |
| `Canceled` + `next_id`   | API is applying patches, follow `next_id`       |
| `Canceled`, no `next_id` | stuck: apply `build.patches` yourself           |

## ⚠️ Gotchas (the stuff that bites)

- **Empty file content → 400.** You can't write a 0-byte file.
- **409 CONFLICT** = someone (the editor, another script) moved the head. Re-read, retry.
- An **API write kicks open editor tabs into read-only.** The API always wins.
- **A failed build is not an HTTP error.** You get a 200 with `build.status: "Failed"`.
- `content_hash` is **sha256 hex**, so you can diff binaries locally without downloading them.
- Binary files go as **multipart**: the JSON goes in a `changes` (PUT) or `pen` (POST) field, plus one part per file **named by its path**.
- Delete = **trash for 3 days**, and it does *not* refund your daily create allowance.
- Non-Public access needs **PRO**. Free gets a 403, never a silent fallback.

## 🚦 Limits

| Plan | Requests | Reads     | Creates/day                |
| ---- | -------- | --------- | -------------------------- |
| Free | 100/h    | 1,000/day | 3 Pens (incl. forks) + 3 Collections |
| PRO  | 3,000/h  | 10,000/day| 50 + 50                    |

Pages max out at 100 items. JSON body max 16 MB, multipart 120 MB, one file 30 MB.

## 💡 Ideas this unlocks

- **Local dev → Pen mirror** (that's `pen`, in this repo)
- **CI publishes demos**: build step pushes `examples/*` → one Pen each, then deploys
- **Collections as galleries**: auto-sync a folder of Pens into a Collection
- **AI agent playground**: an agent writes files, reads `build.errors`, fixes, loops
- **Screenshot thumbnails** for docs/READMEs, straight from `screenshot_urls`
- **Version time-travel**: list versions → restore → deploy a known-good one
- **Asset CDN**: push fonts/images to Assets, reference `assets.codepen.io/...` URLs
- **Bulk housekeeping**: filter by `tags`/`search_term`/`deployed`, then retag or trash

## 📚 Source

- Overview: https://blog.codepen.io/docs/api/api-v2/
- REST: https://blog.codepen.io/docs/api/api-v2/rest-api-v2/
- GraphQL: https://blog.codepen.io/docs/api/api-v2/graphql-api-v2/
