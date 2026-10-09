import { test } from "node:test";
import assert from "node:assert/strict";
import { parse } from "../lib/ref.js";

const UUID = "01a120bd-45fb-7095-a3ea-622e774cf8f0";

test("bare ids", () => {
  assert.equal(parse(UUID), UUID);
  assert.equal(parse(" PwWmgeN\n"), "PwWmgeN");
});

test("2.0 editor url", () => {
  assert.equal(
    parse(`https://codepen.io/editor/nocksock/pen/${UUID}/3f4d5706ac5597fbe5baf7e5126fa018`),
    UUID,
  );
});

test("classic and public urls", () => {
  for (const url of [
    "https://codepen.io/nocksock/pen/PwWmgeN",
    "https://codepen.io/nocksock/pen/PwWmgeN/23e786f57891fa2384306566d37b3f34",
    "https://codepen.io/nocksock/full/PwWmgeN",
    "https://codepen.io/nocksock/details/PwWmgeN?foo=bar#x",
    "https://codepen.io/nocksock/embed/preview/PwWmgeN",
    "https://cdpn.io/username/fullcpgrid/PwWmgeN/23e786f57891fa2384306566d37b3f34/",
    "codepen.io/nocksock/pen/PwWmgeN",
  ]) assert.equal(parse(url), "PwWmgeN", url);
});

test("template urls", () => {
  assert.equal(parse(`https://codepen.io/pen/?template=${UUID}`), UUID);
  assert.equal(parse("https://codepen.io/pen?template=PwWmgeN"), "PwWmgeN");
});

test("rejects refs without an id", () => {
  for (const ref of ["", "https://codepen.io/nocksock", "https://3f4d.codepen.dev/", "not an id"])
    assert.throws(() => parse(ref), /can't find a Pen id/, ref);
});
