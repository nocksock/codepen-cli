// Turns anything that points at a Pen into its API id:
//   01a120bd-45fb-7095-a3ea-622e774cf8f0                       2.0 Pen id
//   PwWmgeN                                                     Classic Pen id
//   https://codepen.io/editor/user/pen/<uuid>/<hash>           2.0 editor
//   https://codepen.io/user/pen/PwWmgeN  (/full, /details, …)  Classic / public
//   https://codepen.io/pen/?template=<id>                      template
//   codepen.io/user/embed/preview/PwWmgeN                      without scheme

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const CLASSIC = /^[A-Za-z0-9]{5,10}$/;
// Path segments naming a view of a Pen; the Classic id follows one of them.
const VIEWS = new Set([
  "pen", "full", "details", "debug", "embed", "preview", "pres", "live",
  "fullpage", "fullcpgrid", "fullembedgrid", "editor",
]);

const isId = (value) => UUID.test(value) || (CLASSIC.test(value) && !VIEWS.has(value));

export function parse(ref) {
  const value = ref.trim();
  if (isId(value)) return value;
  const id = fromUrl(value);
  if (id) return id;
  throw new Error(`can't find a Pen id in "${ref}"`);
}

function fromUrl(value) {
  let url;
  try {
    url = new URL(/^[a-z]+:\/\//i.test(value) ? value : `https://${value}`);
  } catch {
    return null;
  }
  const template = url.searchParams.get("template");
  if (template && isId(template)) return template;
  const segments = url.pathname.split("/").filter(Boolean);
  const uuid = segments.find((segment) => UUID.test(segment));
  if (uuid) return uuid;
  const view = segments.findIndex((segment) => VIEWS.has(segment));
  if (view === -1) return null;
  return segments.slice(view + 1).find(isId) ?? null;
}
