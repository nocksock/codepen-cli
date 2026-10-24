// Thin client for the CodePen REST API v2.
// https://blog.codepen.io/docs/api/api-v2/rest-api-v2/

// Both can be overridden through env, e.g. to point the CLI at a fake API in tests.
const BASE = "https://api.codepen.io/v2";
const POLL_MS = 2000;

export class ApiError extends Error {
  constructor(status, body) {
    super(`${status} ${body.code ?? ""}: ${body.message ?? "request failed"}`);
    this.status = status;
    this.code = body.code;
    this.body = body;
  }
}

// Total items of a list, from the rel="last" page at per_page=1.
export function total(link, items) {
  const last = link?.match(/<([^>]+)>;\s*rel="last"/);
  return last ? Number(new URL(last[1]).searchParams.get("page")) : items.length;
}

export function client(key, {
  base = process.env.CODEPEN_API_URL || BASE,
  pollMs = Number(process.env.CODEPEN_POLL_MS) || POLL_MS,
} = {}) {
  async function send(method, path, body) {
    const isForm = body instanceof FormData;
    const response = await fetch(`${base}${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${key}`,
        ...(body && !isForm && { "Content-Type": "application/json" }),
      },
      body: body && (isForm ? body : JSON.stringify(body)),
    });
    const json = response.status === 204 ? null : await response.json().catch(() => ({}));
    if (!response.ok) throw new ApiError(response.status, json);
    return { response, json };
  }

  const request = async (...args) => (await send(...args)).json;

  async function count(path) {
    const { response, json } = await send("GET", `${path}?per_page=1`);
    return total(response.headers.get("link"), json);
  }

  return {
    pollMs,
    request,
    count,
    pen: (id) => request("GET", `/pens/${id}`),
    files: (id) => request("GET", `/pens/${id}/files`),
    version: (id) => request("GET", `/versions/${id}`),
    build: (id) => request("GET", `/builds/${id}`),
    createPen: (body) => request("POST", "/pens", body),
    writeFiles: (id, body) => request("PUT", `/pens/${id}/files`, body),
  };
}

// Follows a build until it is done, through next_id when the API is
// applying the compiler's patches.
export async function settle(api, build) {
  for (;;) {
    const running = build.status === "Queued" || build.status === "Running";
    const next = build.status === "Canceled" ? build.next_id : null;
    if (!running && !next) return build;
    await new Promise((resolve) => setTimeout(resolve, api.pollMs));
    build = await api.build(next || build.id);
  }
}

// Text changes go as JSON; binary ones need multipart with one part per path.
// `field` is the name of the JSON part: "pen" for POST /pens, "changes" for PUT files.
export function body(field, json, binaries) {
  if (!binaries.length) return json;
  const form = new FormData();
  form.set(field, JSON.stringify(json));
  if (json.expected_version_id) form.set("expected_version_id", json.expected_version_id);
  for (const { path, buffer } of binaries) form.set(path, new Blob([buffer]), path.split("/").pop());
  return form;
}
