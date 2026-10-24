// In-memory stand-in for the CodePen REST API v2: just enough for cdpn.
import { createServer } from "node:http";
import { createHash, randomUUID } from "node:crypto";

export const KEY = "cp_user_test";
const CONFIG = "/.codepen/pen.config.json";

const now = () => new Date().toISOString();
const sha256 = (data) => createHash("sha256").update(data).digest("hex");
const normalize = (path) => (path.startsWith("/") ? path : `/${path}`);

class ApiFailure extends Error {
  constructor(status, code, message, extra = {}) {
    super(message);
    Object.assign(this, { status, code, extra });
  }
}

export async function start() {
  const state = {
    pens: new Map(),
    versions: new Map(),
    requests: [],
    // Writes answer with a Queued build; GET /builds then reports Success.
    queueBuilds: false,
    // Mimics the compiler adding its config file on the first build.
    patchConfig: true,
  };
  const server = createServer((req, res) => handle(state, req, res, origin));
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;

  return {
    url: `${origin}/v2`,
    state,
    // Seeds a Pen directly, without going through the API. Buffer values become binary files.
    seed: (files, meta) => createPen(state, origin, { files: textChanges(files), ...meta }, uploads(files)),
    head: (id) => head(state.pens.get(id)),
    files: (id) => Object.fromEntries(head(state.pens.get(id)).files.map((f) => [f.path, fileJson(f).content])),
    // Simulates someone saving in the CodePen editor.
    edit: (id, files) => write(state, origin, state.pens.get(id), textChanges(files), []),
    calls: (pattern) => state.requests.filter((r) => pattern.test(`${r.method} ${r.path}`)),
    close: () => new Promise((resolve) => server.close(resolve)),
  };
}

const textChanges = (files = {}) =>
  Object.entries(files).filter(([, v]) => typeof v === "string").map(([path, content]) => ({ path, content }));
const uploads = (files = {}) =>
  Object.entries(files).filter(([, v]) => Buffer.isBuffer(v)).map(([path, buffer]) => ({ path: normalize(path), buffer }));
const head = (pen) => pen.versions.at(-1);

async function handle(state, req, res, origin) {
  const url = new URL(req.url, origin);
  state.requests.push({ method: req.method, path: url.pathname });
  try {
    const raw = url.pathname.match(/^\/raw\/([^/]+)\/([^/]+)$/);
    if (raw) return send(res, 200, findFile(state, raw[1], raw[2]).buffer, "application/octet-stream");
    if (req.headers.authorization !== `Bearer ${KEY}`)
      throw new ApiFailure(401, "UNAUTHENTICATED", "invalid or missing API key");
    const result = await route(state, req, url, origin, await readBody(req));
    json(res, result.status ?? 200, result.body, result.headers);
  } catch (error) {
    if (!(error instanceof ApiFailure)) throw error;
    json(res, error.status, { message: error.message, code: error.code, ...error.extra });
  }
}

async function route(state, req, url, origin, body) {
  const at = (pattern) => url.pathname.match(pattern);
  const pen = (id) => {
    const found = state.pens.get(id);
    if (!found) throw new ApiFailure(404, "NOT_FOUND", "no such Pen");
    return found;
  };
  let match;

  if (req.method === "POST" && url.pathname === "/v2/pens") {
    const { json, uploads } = await parse(req, body, "pen");
    const created = createPen(state, origin, json, uploads);
    return { status: 201, body: { ...penJson(created), build: head(created).build } };
  }
  if ((match = at(/^\/v2\/pens\/([^/]+)$/)) && req.method === "GET") return { body: penJson(pen(match[1])) };
  if ((match = at(/^\/v2\/pens\/([^/]+)\/files$/))) {
    const found = pen(match[1]);
    if (req.method === "GET") return { body: head(found).files.map(fileJson) };
    if (req.method === "PUT") {
      const { json, uploads } = await parse(req, body, "changes");
      const expected = json.expected_version_id;
      if (expected && expected !== head(found).id)
        throw new ApiFailure(409, "CONFLICT", "the Pen's head moved", {
          expected_version_id: expected,
          current_version_id: head(found).id,
        });
      const version = write(state, origin, found, json.files ?? [], uploads);
      return { body: { pen: penJson(found), version: versionJson(version), build: version.build } };
    }
  }
  if ((match = at(/^\/v2\/pens\/([^/]+)\/versions$/))) {
    const versions = pen(match[1]).versions.toReversed();
    const perPage = Number(url.searchParams.get("per_page") ?? 30);
    const last = Math.max(1, Math.ceil(versions.length / perPage));
    const link = `<${origin}${url.pathname}?page=${last}&per_page=${perPage}>; rel="last"`;
    return { body: versions.slice(0, perPage).map(versionSummary), headers: { link } };
  }
  if ((match = at(/^\/v2\/versions\/([^/]+)$/))) return { body: versionJson(findVersion(state, match[1])) };
  if ((match = at(/^\/v2\/builds\/([^/]+)$/))) {
    const { build } = findVersion(state, match[1]);
    build.status = "Success";
    return { body: build };
  }
  throw new ApiFailure(404, "NOT_FOUND", `no route ${req.method} ${url.pathname}`);
}

function findVersion(state, id) {
  const version = state.versions.get(id);
  if (!version) throw new ApiFailure(404, "NOT_FOUND", "no such version");
  return version;
}

function findFile(state, penId, fileId) {
  return head(state.pens.get(penId)).files.find((file) => file.id === fileId);
}

function createPen(state, origin, json, uploads) {
  const id = randomUUID();
  const pen = {
    id,
    title: json.title ?? "Untitled",
    tags: json.tags ?? [],
    access: json.access ?? "Private",
    created_at: now(),
    versions: [],
    deployment: json.deployment ?? null,
  };
  state.pens.set(id, pen);
  const starter = [{ path: "/index.html", content: "<h1>starter</h1>" }];
  write(state, origin, pen, json.files ?? (uploads.length ? [] : starter), uploads);
  return pen;
}

// Applies changes on top of the head as one new version, the way PUT /files does.
function write(state, origin, pen, changes, uploads) {
  const files = new Map((head(pen)?.files ?? []).map((file) => [file.path, file]));
  const applied = [];
  for (const change of changes) {
    const path = normalize(change.path);
    if (change.delete) {
      for (const key of files.keys()) if (key === path || key.startsWith(`${path}/`)) files.delete(key);
    } else if (!change.content) {
      throw new ApiFailure(400, "BAD_USER_INPUT", `empty content for ${path}`);
    } else {
      files.set(path, makeFile(origin, pen.id, files.get(path), path, Buffer.from(change.content), false));
    }
  }
  for (const { path, buffer } of uploads)
    files.set(path, makeFile(origin, pen.id, files.get(path), path, buffer, true));
  if (state.patchConfig && !files.has(CONFIG)) {
    const content = '{"blocks":[]}';
    files.set(CONFIG, makeFile(origin, pen.id, null, CONFIG, Buffer.from(content), false));
    applied.push({ type: "create", path: CONFIG, file_id: null, content });
  }

  const id = randomUUID();
  const version = {
    id,
    pen_id: pen.id,
    created_at: now(),
    files: [...files.values()],
    build: {
      id,
      version_id: id,
      status: state.queueBuilds ? "Queued" : "Success",
      rounds: 1,
      errors: [],
      diagnostics: [],
      applied_patches: applied,
      patches: [],
      next_id: null,
      previous_id: null,
    },
  };
  pen.versions.push(version);
  pen.updated_at = version.created_at;
  state.versions.set(id, version);
  return version;
}

function makeFile(origin, penId, previous, path, buffer, binary) {
  const id = previous?.id ?? randomUUID();
  return {
    id,
    path,
    name: path.split("/").pop(),
    buffer,
    is_binary: binary,
    content_hash: sha256(buffer),
    url: `${origin}/raw/${penId}/${id}`,
  };
}

const fileJson = ({ buffer, ...file }) => ({ ...file, content: file.is_binary ? null : buffer.toString("utf8") });
const versionSummary = ({ files, ...version }) => version;
const versionJson = (version) => ({ ...version, files: version.files.map(fileJson) });

function penJson(pen) {
  const latest = head(pen);
  const hash = latest.id.replaceAll("-", "");
  return {
    id: pen.id,
    title: pen.title,
    description: "",
    tags: pen.tags,
    access: pen.access,
    editor: "V2",
    popularity: 0,
    created_at: pen.created_at,
    updated_at: pen.updated_at,
    urls: { editor: `https://codepen.io/editor/test/pen/${pen.id}/${hash}`, preview: `https://${hash}.codepen.dev/` },
    saved_version: versionSummary(latest),
    deployment: pen.deployment,
  };
}

// JSON or multipart: { json, uploads: [{ path, buffer }] }
async function parse(req, body, field) {
  const type = req.headers["content-type"] ?? "";
  if (!type.startsWith("multipart/form-data")) return { json: body.length ? JSON.parse(body) : {}, uploads: [] };
  const form = await new Response(body, { headers: { "content-type": type } }).formData();
  const json = form.has(field) ? JSON.parse(form.get(field)) : {};
  if (form.has("expected_version_id")) json.expected_version_id = form.get("expected_version_id");
  const uploads = [];
  for (const [name, value] of form) {
    if (typeof value !== "string") uploads.push({ path: normalize(name), buffer: Buffer.from(await value.arrayBuffer()) });
  }
  return { json, uploads };
}

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return Buffer.concat(chunks);
}

function json(res, status, body, headers = {}) {
  send(res, status, body === undefined ? "" : JSON.stringify(body), "application/json", headers);
}

function send(res, status, data, type, headers = {}) {
  res.writeHead(status, { "content-type": type, ...headers });
  res.end(data);
}
