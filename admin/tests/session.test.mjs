import assert from "node:assert/strict";
import { test } from "node:test";
import { QueryClient } from "@tanstack/react-query";
import {
  clearAdministrativeCache,
  readSignedOut,
  rememberSignedOut,
} from "../src/sessionState.ts";

test("logout cancels both session discovery and resource requests before clearing private caches", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const signals = [];
  const completions = [];
  const request = ({ signal }) => {
    signals.push(signal);
    return new Promise((resolve) => completions.push(resolve));
  };
  client.setQueryData(["admin", "users"], { items: ["private-user"] });
  const me = client
    .fetchQuery({ queryKey: ["admin-session"], queryFn: request })
    .catch(() => null);
  const data = client
    .fetchQuery({ queryKey: ["admin", "dashboard"], queryFn: request })
    .catch(() => null);
  clearAdministrativeCache(client);
  assert(signals.every((signal) => signal.aborted));
  assert.equal(
    client.getQueryCache().findAll({ queryKey: ["admin"] }).length,
    0,
  );
  assert.equal(client.getQueryData(["admin-session"]), null);
  for (const resolve of completions)
    resolve({ administrator: { id: "previous" } });
  await Promise.all([me, data]);
  assert.equal(client.getQueryData(["admin-session"]), null);
  assert.equal(
    client.getQueryCache().findAll({ queryKey: ["admin"] }).length,
    0,
  );
  client.clear();
});

test("a failed server logout cannot silently restore cookie auth after a page reload", () => {
  const entries = new Map();
  const storage = {
    getItem: (key) => entries.get(key),
    setItem: (key, value) => entries.set(key, value),
    removeItem: (key) => entries.delete(key),
  };
  assert.equal(readSignedOut(storage), false);
  rememberSignedOut(true, storage);
  assert.equal(readSignedOut(storage), true);
  assert.deepEqual([...entries.values()], ["1"]);
  rememberSignedOut(false, storage);
  assert.equal(readSignedOut(storage), false);
  assert.equal(entries.size, 0);
});

test("disabled browser storage does not prevent local logout", () => {
  const storage = {
    getItem() {
      throw new Error("denied");
    },
    setItem() {
      throw new Error("denied");
    },
    removeItem() {
      throw new Error("denied");
    },
  };
  assert.equal(readSignedOut(storage), false);
  assert.doesNotThrow(() => rememberSignedOut(true, storage));
  assert.doesNotThrow(() => rememberSignedOut(false, storage));
});
