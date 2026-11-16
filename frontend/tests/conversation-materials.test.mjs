import assert from "node:assert/strict";
import { test } from "node:test";
import {
  sharedMaterialURL,
  visibleConversationMaterials,
} from "../src/conversationMaterials.ts";

test("shared materials accept explicit HTTP(S) links without credentials or active schemes", () => {
  for (const url of [
    "https://example.com/docs?q=%D0%B0#part",
    "http://localhost/test",
    "HTTPS://example.com/",
  ])
    assert.equal(sharedMaterialURL(url), url);
  for (const url of [
    undefined,
    "",
    "javascript:alert(1)",
    "data:text/html,test",
    "/api/v1/auth/me",
    "//example.com",
    "https://user:secret@example.com",
    "https://example.com\\@host",
    "https://example.com\n",
    " https://example.com",
    "https://",
  ])
    assert.equal(sharedMaterialURL(url), undefined);
});

const material = (
  id,
  sequence,
  version = 1,
  deletedAt = null,
  attachment = { id: "file", status: "attached" },
) => ({ message: { id, sequence, version, deletedAt }, attachment });

test("materials deduplicate overlapping pages and exclude history cleared for this account", () => {
  const first = material("new", "9007199254740993");
  const hidden = material("old", "3");
  const second = material("other", "4");
  assert.deepEqual(
    visibleConversationMaterials(
      [{ items: [first, second] }, { items: [second, hidden] }],
      3,
    ),
    [first, second],
  );
});

test("a deleted newer message does not expose its old materials from another page", () => {
  const old = material("same", "4");
  const removed = material("same", "4", 2, "2026-11-16T12:00:00Z");
  assert.deepEqual(
    visibleConversationMaterials([{ items: [old] }, { items: [removed] }]),
    [],
  );
  assert.deepEqual(
    visibleConversationMaterials([
      {
        items: [
          material("bad", "x"),
          material("pending", "5", 1, null, { id: "file", status: "pending" }),
        ],
      },
    ]),
    [],
  );
});
