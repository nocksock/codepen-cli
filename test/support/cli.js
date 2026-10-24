// Runs bin/cdpn as a child process inside a throwaway workspace.
import { spawn } from "node:child_process";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { KEY } from "./fake-api.js";

const BIN = fileURLToPath(new URL("../../bin/cdpn", import.meta.url));
// Stand-ins on PATH: they record their arguments instead of opening a browser
// or touching the real direnv allow list.
const STUBS = ["xdg-open", "open", "direnv"];

// { dir, run, start, read, exists, write, calls, remove }
export async function workspace(files = {}, { api } = {}) {
  const root = await mkdtemp(join(tmpdir(), "cdpn-test-"));
  const dir = join(root, "work");
  const bin = join(root, "bin");
  const log = join(root, "stubs.log");
  await mkdir(dir);
  await mkdir(bin);
  for (const stub of STUBS) {
    const file = join(bin, stub);
    await writeFile(file, `#!/bin/sh\necho "${stub} $*" >> "${log}"\n`);
    await chmod(file, 0o755);
  }

  const ws = {
    dir,
    write: async (path, content) => {
      await mkdir(dirname(join(dir, path)), { recursive: true });
      await writeFile(join(dir, path), content);
    },
    read: (path) => readFile(join(dir, path), "utf8"),
    rm: (path) => rm(join(dir, path), { recursive: true }),
    exists: (path) => existsSync(join(dir, path)),
    // Lines the stubs recorded, e.g. ["xdg-open https://…"].
    calls: async () => (existsSync(log) ? (await readFile(log, "utf8")).trim().split("\n") : []),
    env: (extra = {}) => {
      const env = {
        PATH: bin,
        HOME: root,
        CODEPEN_API_KEY: KEY,
        CODEPEN_POLL_MS: "5",
        ...(api && { CODEPEN_API_URL: api.url }),
        ...extra,
      };
      return Object.fromEntries(Object.entries(env).filter(([, value]) => value !== undefined));
    },
    run: (args, options) => run(ws, args, options),
    start: (args, options) => start(ws, args, options),
    remove: () => rm(root, { recursive: true, force: true }),
  };
  for (const [path, content] of Object.entries(files)) await ws.write(path, content);
  return ws;
}

// A workspace connected to a fresh Pen holding `files`, set up via `cdpn pull`.
export async function connected(api, files, meta) {
  const pen = api.seed(files, meta);
  const ws = await workspace({}, { api });
  const { code, stderr } = await ws.run(["pull", pen.id]);
  if (code !== 0) throw new Error(`setup pull failed: ${stderr}`);
  return { ws, id: pen.id };
}

function spawnCli(ws, args, { env } = {}) {
  return spawn(process.execPath, [BIN, ...args], { cwd: ws.dir, env: ws.env(env) });
}

function collect(child) {
  const output = { stdout: "", stderr: "" };
  child.stdout.on("data", (data) => (output.stdout += data));
  child.stderr.on("data", (data) => (output.stderr += data));
  return output;
}

// Resolves with { code, stdout, stderr } once the command exits.
export function run(ws, args, options) {
  const child = spawnCli(ws, args, options);
  const output = collect(child);
  return new Promise((resolve, reject) => {
    child.on("error", reject);
    child.on("close", (code) => resolve({ code, ...output }));
  });
}

// For long-running commands (sync): poll the output with waitFor, then stop().
export function start(ws, args, options) {
  const child = spawnCli(ws, args, options);
  const output = collect(child);
  return {
    output,
    waitFor: (pattern, timeout = 5000) => waitFor(() => pattern.test(output.stdout + output.stderr), timeout, output),
    stop: () =>
      new Promise((resolve) => {
        child.on("close", resolve);
        child.kill();
      }),
  };
}

async function waitFor(check, timeout, output) {
  const deadline = Date.now() + timeout;
  while (!check()) {
    if (Date.now() > deadline) throw new Error(`timed out; output so far:\n${output.stdout}${output.stderr}`);
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
}
