// push / pull: composes api, tree and diff into Pen <-> directory sync.
import { basename, resolve } from "node:path";
import * as env from "./env.js";
import * as tree from "./tree.js";
import { client, settle, body } from "./api.js";
import { byPath, diff, describe, isEmpty, isSame } from "./diff.js";
import * as ref from "./ref.js";

// { api, dir, id, versionId, urls, remote } — remote is the head's files by path.
export function open() {
  return {
    api: client(env.secret("CODEPEN_API_KEY")),
    dir: resolve(env.get("CODEPEN_DIR", ".")),
    id: process.env.CODEPEN_ID ? ref.parse(process.env.CODEPEN_ID) : null,
    versionId: null,
    urls: null,
    remote: new Map(),
  };
}

export async function fetchRemote(pen) {
  const [meta, files] = await Promise.all([pen.api.pen(pen.id), pen.api.files(pen.id)]);
  pen.urls = meta.urls;
  return track(pen, meta.saved_version?.id ?? null, files);
}

function track(pen, versionId, files) {
  pen.versionId = versionId;
  pen.remote = byPath(files);
  return pen;
}

// After a write the head may differ from what was sent (compiler patches),
// so the settled version is the source of truth.
async function finish(pen, build) {
  build = await settle(pen.api, build);
  report(build);
  const version = await pen.api.version(build.version_id);
  return track(pen, version.id, version.files);
}

function report(build) {
  for (const patch of build.applied_patches ?? []) console.log(`  patched ${patch.type} ${patch.path}`);
  for (const error of build.errors ?? []) {
    const line = error.range?.start?.line;
    console.log(`  ✗ ${error.path ?? ""}${line ? `:${line}` : ""} ${error.message}`);
  }
  console.log(`  build ${build.status}`);
}

export async function create(pen) {
  const local = await tree.read(pen.dir);
  const { changes, binaries } = diff(local, new Map());
  // No files means CodePen's starter files, which get pulled down below.
  const json = { title: basename(pen.dir), ...(local.size && { files: changes }) };
  const created = await pen.api.createPen(body("pen", json, binaries));
  pen.id = created.id;
  pen.urls = created.urls;
  env.set("CODEPEN_ID", created.id);
  console.log(`created ${created.urls.editor}`);
  await finish(pen, created.build);
  await writeBack(pen, local);
  return pen;
}

export async function push(pen, { force = false } = {}) {
  if (!pen.id) return create(pen);
  if (!pen.versionId) await fetchRemote(pen);
  const local = await tree.read(pen.dir);
  if (!local.size) throw new Error(`refusing to push: ${pen.dir} is empty (would delete every file)`);
  const changes = diff(local, pen.remote);
  if (isEmpty(changes)) return pen;
  for (const line of describe(changes)) console.log(`  ${line}`);
  const json = { files: changes.changes, ...(!force && { expected_version_id: pen.versionId }) };
  const written = await pen.api.writeFiles(pen.id, body("changes", json, changes.binaries));
  await finish(pen, written.build);
  await writeBack(pen, local);
  return pen;
}

// `penRef` (id or URL, see ref.js) connects the directory to that Pen.
// Switching away from another connected Pen needs `force`.
export async function pull(pen, { penRef, force = false } = {}) {
  const id = penRef ? ref.parse(penRef) : pen.id;
  if (!id) throw new Error("no Pen to pull: pass a Pen id or URL, or set CODEPEN_ID");
  if (pen.id && pen.id !== id && !force)
    throw new Error(`this directory is connected to ${pen.id}; pass --force to switch to ${id}`);
  const previous = pen.id;
  pen.id = id;
  await fetchRemote(pen);
  if (previous !== id) {
    env.set("CODEPEN_ID", id);
    console.log(`connected ${pen.urls.editor}`);
  }
  await writeBack(pen, await tree.read(pen.dir).catch(() => new Map()));
  return pen;
}

// Writes remote files that differ from the local copy. Never deletes locally.
async function writeBack(pen, local) {
  for (const [path, file] of pen.remote) {
    const current = local.get(path);
    if (current && isSame(current, file)) continue;
    const data = file.content ?? Buffer.from(await (await fetch(file.url)).arrayBuffer());
    await tree.write(pen.dir, path, data);
    console.log(`  ↓ ${path}`);
  }
}
