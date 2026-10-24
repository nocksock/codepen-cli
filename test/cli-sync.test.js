import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { start } from "./support/fake-api.js";
import { connected } from "./support/cli.js";

let api;
before(async () => (api = await start()));
after(() => api.close());

const until = async (check, timeout = 5000) => {
  const deadline = Date.now() + timeout;
  while (!check()) {
    if (Date.now() > deadline) throw new Error("timed out");
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
};

test("sync pushes local changes as they happen", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  const sync = ws.start(["sync"]);
  try {
    await sync.waitFor(/watching/);
    assert.match(sync.output.stdout, /^editor +https:\/\/codepen\.io\/editor\//m);
    await ws.write("a.js", "2");
    await sync.waitFor(/~ \/a\.js/);
    await until(() => api.files(id)["/a.js"] === "2");
    await ws.write("src/new.js", "3");
    await sync.waitFor(/~ \/src\/new\.js/);
    await until(() => api.files(id)["/src/new.js"] === "3");
  } finally {
    await sync.stop();
  }
  await ws.remove();
});

test("sync reports a remote edit instead of overwriting it", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  const sync = ws.start(["sync"]);
  try {
    await sync.waitFor(/watching/);
    api.edit(id, { "a.js": "from the editor" });
    await ws.write("a.js", "local");
    await sync.waitFor(/changed remotely/);
    assert.equal(api.files(id)["/a.js"], "from the editor");
  } finally {
    await sync.stop();
  }
  await ws.remove();
});

test("sync --force overwrites a remote edit", async () => {
  const { ws, id } = await connected(api, { "a.js": "1" });
  const sync = ws.start(["sync", "--force"]);
  try {
    await sync.waitFor(/watching/);
    api.edit(id, { "a.js": "from the editor" });
    await ws.write("a.js", "local");
    await sync.waitFor(/~ \/a\.js/);
    await until(() => api.files(id)["/a.js"] === "local");
  } finally {
    await sync.stop();
  }
  await ws.remove();
});
