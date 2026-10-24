import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { start } from "./support/fake-api.js";
import { connected } from "./support/cli.js";

let api;
before(async () => (api = await start()));
after(() => api.close());

const LIVE = "https://cheerful-term-baboon.codepen.app/";

test("info shows the Pen and that the directory is in sync", async () => {
  const { ws, id } = await connected(api, { "a.js": "12345" }, { title: "Demo", tags: ["api", "demo"] });
  const { code, stdout } = await ws.run(["info"]);
  assert.equal(code, 0);
  assert.match(stdout, /^title +Demo$/m);
  assert.match(stdout, new RegExp(`^id +${id} \\(V2\\)$`, "m"));
  assert.match(stdout, /^tags +api, demo$/m);
  assert.match(stdout, /^versions +1, head /m);
  assert.match(stdout, /^build +Success$/m);
  assert.match(stdout, /^files +2 files, 18 B text$/m);
  assert.match(stdout, /^local +in sync$/m);
  assert.match(stdout, /^deployed +no$/m);
  assert.match(stdout, /^editor +https:\/\/codepen\.io\/editor\//m);
  await ws.remove();
});

test("info counts what a push would change", async () => {
  const { ws } = await connected(api, { "a.js": "1", "b.js": "2" });
  await ws.write("a.js", "changed");
  await ws.write("c.js", "new");
  await ws.rm("b.js");
  const { stdout } = await ws.run(["info"]);
  assert.match(stdout, /^local +2 to upload, 1 to delete \(cdpn push\)$/m);
  await ws.remove();
});

test("info shows the deployment URL", async () => {
  const { ws } = await connected(api, { "a.js": "1" }, { deployment: { url: LIVE } });
  const { stdout } = await ws.run(["info"]);
  assert.match(stdout, new RegExp(`^deployed +${LIVE}$`, "m"));
  await ws.remove();
});

test("open launches the editor, preview or live URL", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" }, { deployment: { url: LIVE } });
  const { urls } = (await (await fetch(`${api.url}/pens/${id}`, { headers: { Authorization: "Bearer cp_user_test" } })).json());
  for (const [target, url] of [[[], urls.editor], [["preview"], urls.preview], [["live"], LIVE]]) {
    const { code, stdout } = await ws.run(["open", ...target]);
    assert.equal(code, 0, `open ${target}`);
    assert.equal(stdout.trim(), url);
  }
  // the opener is detached, give it a moment to record its call
  await new Promise((resolve) => setTimeout(resolve, 200));
  const opener = process.platform === "darwin" ? "open" : "xdg-open";
  assert.deepEqual(await ws.calls(), [urls.editor, urls.preview, LIVE].map((url) => `${opener} ${url}`));
  await ws.remove();
});

test("open fails for unknown targets and undeployed Pens", async () => {
  const { ws } = await connected(api, { "a.js": "1" });
  const live = await ws.run(["open", "live"]);
  assert.equal(live.code, 1);
  assert.match(live.stderr, /no live URL/);
  const bogus = await ws.run(["open", "bogus"]);
  assert.equal(bogus.code, 1);
  assert.match(bogus.stderr, /unknown target "bogus"/);
  assert.deepEqual(await ws.calls(), []);
  await ws.remove();
});
