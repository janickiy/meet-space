import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  attachmentAccept,
  attachmentExtensions,
  attachmentLimit,
  validateAttachment,
} from "../src/attachmentPolicy.ts";

test("attachment picker and server allow exactly the same extensions", () => {
  const source = readFileSync(
    new URL("../../internal/domain/chat/attachment_types.go", import.meta.url),
    "utf8",
  );
  const server = [...source.matchAll(/^\s*"\.([a-z0-9]+)":/gm)].map(
    (match) => match[1],
  );
  assert.equal(new Set(attachmentExtensions).size, attachmentExtensions.length);
  assert.deepEqual([...attachmentExtensions].sort(), server.sort());
  assert.deepEqual(
    attachmentAccept.split(","),
    attachmentExtensions.map((extension) => `.${extension}`),
  );
});

test("all supported documents and archives pass client validation", () => {
  for (const extension of attachmentExtensions) {
    for (const size of [1, attachmentLimit])
      assert.equal(
        validateAttachment({ name: `document.${extension.toUpperCase()}`, size }),
        null,
        extension,
      );
  }
  for (const name of ["backup.tar.gz", "backup.tar.bz2", "backup.tar.xz"])
    assert.equal(validateAttachment({ name, size: 1024 }), null);
});

test("client still rejects active extensions and invalid attachment sizes", () => {
  for (const name of [
    "program.exe",
    "script.js",
    "page.html",
    "image.svg",
    "document.pdf.exe",
    "no-extension",
  ])
    assert.equal(typeof validateAttachment({ name, size: 1 }), "string");
  for (const size of [0, -1, NaN, Infinity, attachmentLimit + 1])
    assert.equal(
      typeof validateAttachment({ name: "document.zip", size }),
      "string",
    );
});
