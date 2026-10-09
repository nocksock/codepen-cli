// Local directory <-> { "/path": Buffer } maps.
import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, relative, sep } from "node:path";

const IGNORED = new Set([".git", "node_modules", ".env", ".envrc", ".DS_Store"]);

// "/src/app.js" style path, as the API uses them.
export function penPath(root, file) {
  return "/" + relative(root, file).split(sep).join("/");
}

export function isIgnored(path) {
  return path.split("/").some((part) => IGNORED.has(part) || part.startsWith(".env."));
}

export async function read(root) {
  const entries = await readdir(root, { recursive: true, withFileTypes: true });
  const files = new Map();
  for (const entry of entries) {
    if (!entry.isFile()) continue;
    const file = join(entry.parentPath, entry.name);
    const path = penPath(root, file);
    if (!isIgnored(path)) files.set(path, await readFile(file));
  }
  return files;
}

export async function write(root, path, data) {
  const file = join(root, path);
  await mkdir(dirname(file), { recursive: true });
  await writeFile(file, data);
}

const utf8 = new TextDecoder("utf-8", { fatal: true });

export function isText(buffer) {
  if (buffer.includes(0)) return false;
  try {
    utf8.decode(buffer);
    return true;
  } catch {
    return false;
  }
}
