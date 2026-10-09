import { watch as fsWatch } from "node:fs";
import { isIgnored } from "./tree.js";

export function debounce(fn, ms) {
  let timer;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

// Never runs fn concurrently; calls arriving mid-run collapse into one rerun.
export function serial(fn) {
  let running = null;
  let again = false;
  const run = () => {
    if (running) {
      again = true;
      return running;
    }
    running = fn().finally(() => {
      running = null;
      if (again) (again = false), run();
    });
    return running;
  };
  return run;
}

export function watch(dir, onChange, { delay = 400 } = {}) {
  const fire = debounce(onChange, delay);
  return fsWatch(dir, { recursive: true }, (_event, file) => {
    if (file && !isIgnored("/" + file.replaceAll("\\", "/"))) fire();
  });
}
