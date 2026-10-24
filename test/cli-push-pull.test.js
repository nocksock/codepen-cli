import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { start } from "./support/fake-api.js";
import { connected, workspace } from "./support/cli.js";

const PNG = Buffer.from("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489", "hex");

let api;
before(async () => (api = await start()));
after(() => api.close());

const writes = (id) => api.calls(new RegExp(`^PUT /v2/pens/${id}/files$`)).length;

test("push creates a Pen and writes CODEPEN_ID to .env.local", async () => {
  const ws = await workspace({ "index.html": "<h1>x</h1>", "src/app.js": "1" }, { api });
  const { code, stdout } = await ws.run(["push"]);
  assert.equal(code, 0);
  assert.match(stdout, /^created https:\/\/codepen\.io\/editor\//);
  const id = (await ws.read(".env.local")).match(/^CODEPEN_ID=(.+)$/m)[1];
  assert.equal(api.files(id)["/index.html"], "<h1>x</h1>");
  assert.equal(api.files(id)["/src/app.js"], "1");
  // the compiler's patch comes back down
  assert.ok(ws.exists(".codepen/pen.config.json"));
  await ws.remove();
});

test("push in an empty directory creates a Pen with starter files", async () => {
  const ws = await workspace({}, { api });
  const { code } = await ws.run(["push"]);
  assert.equal(code, 0);
  assert.equal(await ws.read("index.html"), "<h1>starter</h1>");
  await ws.remove();
});

test("push with nothing changed makes no write", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  const { code, stdout } = await ws.run(["push"]);
  assert.equal(code, 0);
  assert.equal(stdout, "");
  assert.equal(writes(id), 0);
  await ws.remove();
});

test("push uploads changes and new files and deletes missing ones", async () => {
  const { ws, id } = await connected(api, { "a.js": "1", "b.js": "2" });
  await ws.write("a.js", "changed");
  await ws.write("c.js", "new");
  await ws.rm("b.js");
  const { code, stdout } = await ws.run(["push"]);
  assert.equal(code, 0);
  assert.match(stdout, /~ \/a\.js/);
  assert.match(stdout, /~ \/c\.js/);
  assert.match(stdout, /- \/b\.js/);
  const files = api.files(id);
  assert.equal(files["/a.js"], "changed");
  assert.equal(files["/c.js"], "new");
  assert.ok(!("/b.js" in files));
  await ws.remove();
});

test("binary files go up as multipart and are diffed by hash", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  await ws.write("img/dot.png", PNG);
  const first = await ws.run(["push"]);
  assert.match(first.stdout, /~ \/img\/dot\.png \(binary\)/);
  const file = api.head(id).files.find((f) => f.path === "/img/dot.png");
  assert.ok(file.is_binary);
  assert.deepEqual(file.buffer, PNG);
  const second = await ws.run(["push"]);
  assert.equal(second.stdout, "");
  assert.equal(writes(id), 1);
  await ws.remove();
});

test("push skips empty and ignored files", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  await ws.write("empty.js", "");
  await ws.write("node_modules/dep/index.js", "x");
  await ws.write(".env.production", "SECRET=1");
  await ws.write(".envrc", "dotenv");
  const { code, stdout } = await ws.run(["push"]);
  assert.equal(code, 0);
  assert.equal(stdout, "");
  assert.equal(writes(id), 0);
  await ws.remove();
});

test("push refuses to empty a connected Pen", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  for (const path of ["a.js", ".codepen"]) await ws.rm(path);
  const { code, stderr } = await ws.run(["push"]);
  assert.equal(code, 1);
  assert.match(stderr, /refusing to push/);
  assert.equal(writes(id), 0);
  await ws.remove();
});

test("push polls a build that is still running", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  await ws.write("a.js", "2");
  api.state.queueBuilds = true;
  try {
    const { code, stdout } = await ws.run(["push"]);
    assert.equal(code, 0);
    assert.match(stdout, /build Success/);
    assert.ok(api.calls(/^GET \/v2\/builds\//).length > 0);
    assert.equal(api.files(id)["/a.js"], "2");
  } finally {
    api.state.queueBuilds = false;
  }
  await ws.remove();
});

test("pull PEN accepts an editor URL and downloads text and binary files", async () => {
  const pen = api.seed({ "index.html": "<p>hi</p>", "dot.png": PNG });
  const ws = await workspace({ "keep.txt": "local only" }, { api });
  const { code, stdout } = await ws.run(["pull", `https://codepen.io/editor/me/pen/${pen.id}/abc123`]);
  assert.equal(code, 0);
  assert.match(stdout, /connected/);
  assert.equal(await ws.read("index.html"), "<p>hi</p>");
  assert.deepEqual(await readFile(join(ws.dir, "dot.png")), PNG);
  // pull never deletes local files
  assert.equal(await ws.read("keep.txt"), "local only");
  await ws.remove();
});

test("pull overwrites local edits with the remote version", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  api.edit(id, { "a.js": "remote" });
  await ws.write("a.js", "local");
  const { stdout } = await ws.run(["pull"]);
  assert.match(stdout, /↓ \/a\.js/);
  assert.equal(await ws.read("a.js"), "remote");
  await ws.remove();
});

test("pull without a Pen fails", async () => {
  const ws = await workspace({}, { api });
  const { code, stderr } = await ws.run(["pull"]);
  assert.equal(code, 1);
  assert.match(stderr, /no Pen to pull/);
  await ws.remove();
});

test("switching to another Pen needs --force", async () => {
  const { ws } = await connected(api, { "a.js": "1" });
  const other = api.seed({ "b.js": "2" });
  const refused = await ws.run(["pull", other.id]);
  assert.equal(refused.code, 1);
  assert.match(refused.stderr, /pass --force/);
  const forced = await ws.run(["pull", other.id, "--force"]);
  assert.equal(forced.code, 0);
  assert.equal(await ws.read(".env.local"), `CODEPEN_ID=${other.id}\n`);
  assert.equal(await ws.read("b.js"), "2");
  await ws.remove();
});
