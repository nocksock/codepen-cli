import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";

// .env may be a 1Password Environments mount, so it is only ever read.
// Anything cdpn writes goes to .env.local, which also wins over .env.
export const ENV_FILES = [".env.local", ".env"];
export const STATE_FILE = ".env.local";

// Loads env files without overriding what direnv / the shell already set.
export function load(files = ENV_FILES) {
  for (const file of files) if (existsSync(file)) process.loadEnvFile(file);
}

export function get(name, fallback) {
  const value = process.env[name];
  if (value) return value;
  if (fallback !== undefined) return fallback;
  throw new Error(`${name} is not set (check ${ENV_FILES.join(" / ")})`);
}

// Resolves 1Password secret references (op://...) via the op CLI.
// Values already resolved by `op run` pass through untouched.
export function secret(name) {
  const value = get(name);
  if (!value.startsWith("op://")) return value;
  return execFileSync("op", ["read", value], { encoding: "utf8" }).trim();
}

// Sets KEY=value in the env file, replacing an existing line or appending.
export function set(name, value, file = STATE_FILE) {
  const text = existsSync(file) ? readFileSync(file, "utf8") : "";
  const line = `${name}=${value}`;
  const pattern = new RegExp(`^${name}=.*$`, "m");
  const next = pattern.test(text)
    ? text.replace(pattern, line)
    : `${text}${text && !text.endsWith("\n") ? "\n" : ""}${line}\n`;
  writeFileSync(file, next);
  process.env[name] = value;
}
