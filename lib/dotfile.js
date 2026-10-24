// Non-destructive writes to small line-based config files (.env, .envrc, .gitignore).
import { existsSync, readFileSync, writeFileSync } from "node:fs";

// "created" or "kept". Never touches an existing file (.env may be a 1Password mount).
export function ensure(file, content) {
  if (existsSync(file)) return "kept";
  writeFileSync(file, content);
  return "created";
}

// Appends the lines missing from file: "created", "updated" or "kept".
export function ensureLines(file, lines) {
  if (!existsSync(file)) return ensure(file, lines.join("\n") + "\n");
  const text = readFileSync(file, "utf8");
  const present = new Set(text.split("\n").map((line) => line.trim()));
  const missing = lines.filter((line) => !present.has(line));
  if (!missing.length) return "kept";
  const separator = text && !text.endsWith("\n") ? "\n" : "";
  writeFileSync(file, text + separator + missing.join("\n") + "\n");
  return "updated";
}
