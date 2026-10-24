import { spawn } from "node:child_process";

const OPENERS = {
  darwin: ["open"],
  win32: ["cmd", "/c", "start", ""],
};

// Opens url in the default browser without waiting for it.
export function open(url) {
  const [command, ...args] = OPENERS[process.platform] ?? ["xdg-open"];
  const child = spawn(command, [...args, url], { detached: true, stdio: "ignore" });
  child.on("error", () => console.log(`could not launch a browser; open ${url}`));
  child.unref();
}
