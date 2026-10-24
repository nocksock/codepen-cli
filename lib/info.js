// cdpn info: the connected Pen's details plus how the local directory compares.
import * as tree from "./tree.js";
import { byPath, diff } from "./diff.js";
import { ago, bytes, plural, table } from "./format.js";

export async function gather(pen) {
  const [meta, files, versions, local] = await Promise.all([
    pen.api.pen(pen.id),
    pen.api.files(pen.id),
    pen.api.count(`/pens/${pen.id}/versions`),
    tree.read(pen.dir).catch(() => new Map()),
  ]);
  return { meta, files, versions, changes: diff(local, byPath(files)) };
}

function size(files) {
  const text = files.filter((file) => file.content !== null);
  const total = text.reduce((sum, file) => sum + Buffer.byteLength(file.content), 0);
  const binaries = files.length - text.length;
  return `${plural(files.length, "file")}, ${bytes(total)} text${binaries ? ` + ${binaries} binary` : ""}`;
}

function status({ changes, binaries }) {
  const deletes = changes.filter((change) => change.delete).length;
  const writes = changes.length - deletes + binaries.length;
  if (!writes && !deletes) return "in sync";
  return [writes && `${writes} to upload`, deletes && `${deletes} to delete`].filter(Boolean).join(", ") +
    " (cdpn push)";
}

function build(version) {
  if (!version) return null;
  const errors = version.build?.errors?.length;
  return `${version.build?.status ?? "none"}${errors ? `, ${plural(errors, "error")}` : ""}`;
}

export function render({ meta, files, versions, changes }) {
  const head = meta.saved_version;
  return table([
    ["title", meta.title],
    ["id", `${meta.id} (${meta.editor})`],
    ["access", meta.access],
    ["tags", meta.tags?.join(", ")],
    ["created", ago(meta.created_at)],
    ["updated", ago(meta.updated_at)],
    ["versions", `${versions}${head ? `, head ${ago(head.created_at)}` : ""}`],
    ["build", build(head)],
    ["files", size(files)],
    ["local", status(changes)],
    ["deployed", meta.deployment ? meta.deployment.url : "no"],
    ["popularity", meta.popularity],
    ["editor", meta.urls?.editor],
    ["preview", meta.urls?.preview],
  ]);
}
