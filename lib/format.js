// Small terminal formatting primitives.

const UNITS = ["B", "KB", "MB", "GB"];

export function bytes(n) {
  let unit = 0;
  while (n >= 1024 && unit < UNITS.length - 1) (n /= 1024), unit++;
  return `${unit ? n.toFixed(1) : n} ${UNITS[unit]}`;
}

const SPANS = [
  ["year", 31536000],
  ["month", 2592000],
  ["day", 86400],
  ["hour", 3600],
  ["minute", 60],
];
const relative = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

export function ago(date, now = Date.now()) {
  const seconds = (new Date(date) - now) / 1000;
  const [unit, size] = SPANS.find(([, size]) => Math.abs(seconds) >= size) ?? ["second", 1];
  return relative.format(Math.round(seconds / size), unit);
}

export function plural(n, word) {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

// [[label, value], ...] → aligned "label  value" lines; empty values are dropped.
export function table(rows) {
  const shown = rows.filter(([, value]) => value !== null && value !== undefined && value !== "");
  const width = Math.max(...shown.map(([label]) => label.length));
  return shown.map(([label, value]) => `${label.padEnd(width)}  ${value}`).join("\n");
}
