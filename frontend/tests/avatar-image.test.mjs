import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { avatarSourceError, prepareAvatar } from "../src/avatarImage.ts";

const bitmap = globalThis.createImageBitmap;
const documentValue = globalThis.document;
afterEach(() => {
  if (bitmap === undefined) delete globalThis.createImageBitmap;
  else globalThis.createImageBitmap = bitmap;
  if (documentValue === undefined) delete globalThis.document;
  else globalThis.document = documentValue;
});

test("avatar source allows photos but excludes active and oversized formats", () => {
  for (const type of ["image/png", "image/jpeg", "image/webp"])
    assert.equal(avatarSourceError({ size: 1024, type }), undefined);
  for (const file of [
    { size: 0, type: "image/png" },
    { size: 11 * 1024 * 1024, type: "image/png" },
    { size: 100, type: "image/svg+xml" },
    { size: 100, type: "image/gif" },
  ])
    assert.equal(typeof avatarSourceError(file), "string");
});

test("avatar preparation closes late decoded images after cancellation", async () => {
  const controller = new AbortController();
  let close = 0;
  globalThis.createImageBitmap = async () => {
    controller.abort();
    return {
      close() {
        close++;
      },
    };
  };
  await assert.rejects(
    prepareAvatar(
      new File(["synthetic"], "photo.png", { type: "image/png" }),
      controller.signal,
    ),
    { name: "AbortError" },
  );
  assert.equal(close, 1);
});

test("avatar preparation crops phone photos to a small metadata-free JPEG", async () => {
  let closed = 0;
  let args;
  globalThis.createImageBitmap = async () => ({
    width: 4000,
    height: 3000,
    close() {
      closed++;
    },
  });
  const canvas = {
    width: 0,
    height: 0,
    getContext: () => ({
      fillRect() {},
      drawImage(...values) {
        args = values;
      },
    }),
    toBlob(callback, type) {
      callback(new Blob(["synthetic-jpeg"], { type }));
    },
  };
  globalThis.document = { createElement: () => canvas };
  const file = await prepareAvatar(
    new File(["synthetic"], "photo.png", { type: "image/png" }),
    new AbortController().signal,
  );
  assert.equal(canvas.width, 512);
  assert.equal(canvas.height, 512);
  assert.deepEqual(args.slice(1), [500, 0, 3000, 3000, 0, 0, 512, 512]);
  assert.equal(file.type, "image/jpeg");
  assert.equal(closed, 1);
});
