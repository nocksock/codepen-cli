import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { start } from "./support/fake-api.js";
import { workspace } from "./support/cli.js";

let api;
before(async () => (api = await start()));
after(() => api.close());

test("--help prints usage and exits 0", async () => {
  const ws = await workspace();
  const { code, stdout } = await ws.run(["--help"]);
  assert.equal(code, 0);
  for (const command of ["init", "push", "pull", "sync", "info", "open"]) assert.match(stdout, new RegExp(`cdpn ${command}`));
  await ws.remove();
});

test("no or unknown command prints usage and exits 1", async () => {
  const ws = await workspace();
  for (const args of [[], ["bogus"]]) {
    const { code, stdout } = await ws.run(args);
    assert.equal(code, 1, `args: ${args}`);
    assert.match(stdout, /cdpn push/);
  }
  await ws.remove();
});

test("missing API key is reported", async () => {
  const ws = await workspace({}, { api });
  const { code, stderr } = await ws.run(["push"], { env: { CODEPEN_API_KEY: undefined } });
  assert.equal(code, 1);
  assert.match(stderr, /CODEPEN_API_KEY is not set/);
  await ws.remove();
});

test("an invalid API key is reported", async () => {
  const ws = await workspace({ "a.js": "1" }, { api });
  const { code, stderr } = await ws.run(["push"], { env: { CODEPEN_API_KEY: "cp_user_wrong" } });
  assert.equal(code, 1);
  assert.match(stderr, /401 UNAUTHENTICATED/);
  await ws.remove();
});

test("info and open need a connected Pen", async () => {
  const ws = await workspace({}, { api });
  for (const command of ["info", "open"]) {
    const { code, stderr } = await ws.run([command]);
    assert.equal(code, 1, command);
    assert.match(stderr, /no Pen connected/);
  }
  await ws.remove();
});

test("an unknown Pen explains the API's scope", async () => {
  const ws = await workspace({}, { api });
  const { code, stderr } = await ws.run(["pull", "01a120bd-0000-7000-8000-000000000000"]);
  assert.equal(code, 1);
  assert.match(stderr, /only reaches your own Pens/);
  await ws.remove();
});
