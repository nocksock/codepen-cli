// Turns local files vs. remote Pen files into an API change list.
import { createHash } from "node:crypto";
import { isText } from "./tree.js";

export function hash(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

export function byPath(files) {
  return new Map(files.map((file) => [file.path, file]));
}

export function isSame(buffer, remote) {
  if (!remote) return false;
  if (remote.content !== null) return buffer.toString("utf8") === remote.content;
  return hash(buffer) === remote.content_hash;
}

// local: Map<path, Buffer>, remote: Map<path, File>
// Empty files are skipped on both sides: the API refuses empty content.
export function diff(local, remote) {
  const changes = [];
  const binaries = [];
  for (const [path, buffer] of local) {
    if (!buffer.length || isSame(buffer, remote.get(path))) continue;
    if (isText(buffer)) changes.push({ path, content: buffer.toString("utf8") });
    else binaries.push({ path, buffer });
  }
  for (const path of remote.keys()) {
    if (!local.has(path)) changes.push({ path, delete: true });
  }
  return { changes, binaries };
}

export function isEmpty({ changes, binaries }) {
  return !changes.length && !binaries.length;
}

export function describe({ changes, binaries }) {
  return [
    ...changes.map((c) => `${c.delete ? "-" : "~"} ${c.path}`),
    ...binaries.map((b) => `~ ${b.path} (binary)`),
  ];
}
