// cdpn init: makes the current directory ready for push / pull / sync.
import { execFileSync } from "node:child_process";
import { ensure, ensureLines } from "./dotfile.js";
import { SECRETS_FILE, STATE_FILE } from "./env.js";

export const DEFAULT_KEY = "op://Private/CodePen/API Key";

export function scaffold({ key = DEFAULT_KEY } = {}) {
  return {
    [SECRETS_FILE]: ensure(SECRETS_FILE, `CODEPEN_API_KEY="${key}"\n`),
    // later files win in direnv, so state comes after secrets
    ".envrc": ensureLines(".envrc", [`dotenv_if_exists ${SECRETS_FILE}`, `dotenv_if_exists ${STATE_FILE}`]),
    ".gitignore": ensureLines(".gitignore", [SECRETS_FILE, STATE_FILE]),
  };
}

// true when direnv exists and accepted the .envrc.
export function allowDirenv() {
  try {
    execFileSync("direnv", ["allow", "."], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

export function report(results) {
  for (const [file, status] of Object.entries(results)) console.log(`  ${status.padEnd(7)} ${file}`);
}
