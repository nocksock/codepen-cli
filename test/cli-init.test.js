import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { start } from "./support/fake-api.js";
import { workspace } from "./support/cli.js";

let api;
before(async () => (api = await start()));
after(() => api.close());

test("creates .env, .envrc and .gitignore and allows direnv", async () => {
  const ws = await workspace();
  const { code, stdout } = await ws.run(["init"]);
  assert.equal(code, 0);
  assert.match(stdout, /created .env\n/);
  assert.match(stdout, /direnv allowed/);
  assert.equal(await ws.read(".env"), 'CODEPEN_API_KEY="op://Private/CodePen/API Key"\n');
  assert.equal(await ws.read(".envrc"), "dotenv_if_exists .env\ndotenv_if_exists .env.local\n");
  assert.equal(await ws.read(".gitignore"), ".env\n.env.local\n");
  assert.deepEqual(await ws.calls(), ["direnv allow ."]);
  await ws.remove();
});

test("--key sets the key of a new .env", async () => {
  const ws = await workspace();
  await ws.run(["init", "--key", "cp_user_abc"]);
  assert.equal(await ws.read(".env"), 'CODEPEN_API_KEY="cp_user_abc"\n');
  await ws.remove();
});

test("running it twice changes nothing", async () => {
  const ws = await workspace();
  await ws.run(["init"]);
  const before = await Promise.all([".env", ".envrc", ".gitignore"].map(ws.read));
  const { stdout } = await ws.run(["init"]);
  assert.equal(stdout.match(/kept/g).length, 3);
  assert.deepEqual(await Promise.all([".env", ".envrc", ".gitignore"].map(ws.read)), before);
  await ws.remove();
});

test("appends to existing files and leaves an existing .env alone", async () => {
  const ws = await workspace({
    ".env": "CODEPEN_API_KEY=mine\n",
    ".envrc": "export FOO=1",
    ".gitignore": "node_modules\n.env\n",
  });
  const { stdout } = await ws.run(["init", "--key", "ignored"]);
  assert.match(stdout, /kept {4}.env\n/);
  assert.match(stdout, /updated .envrc/);
  assert.equal(await ws.read(".env"), "CODEPEN_API_KEY=mine\n");
  assert.equal(await ws.read(".envrc"), "export FOO=1\ndotenv_if_exists .env\ndotenv_if_exists .env.local\n");
  assert.equal(await ws.read(".gitignore"), "node_modules\n.env\n.env.local\n");
  await ws.remove();
});

test("works without direnv installed", async () => {
  const ws = await workspace();
  const { code, stdout } = await ws.run(["init"], { env: { PATH: "/nonexistent" } });
  assert.equal(code, 0);
  assert.doesNotMatch(stdout, /direnv allowed/);
  await ws.remove();
});

test("init PEN connects and pulls", async () => {
  const pen = api.seed({ "index.html": "<p>hi</p>" });
  const ws = await workspace({}, { api });
  const { code } = await ws.run(["init", pen.id]);
  assert.equal(code, 0);
  assert.equal(await ws.read("index.html"), "<p>hi</p>");
  assert.equal(await ws.read(".env.local"), `CODEPEN_ID=${pen.id}\n`);
  await ws.remove();
});
