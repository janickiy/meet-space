import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { api, configureAuth } from "../src/api.ts";

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});

test("the client no longer exposes speech-processing operations", () => {
  for (const name of [
    "transcript",
    "transcriptSegments",
    "retryTranscript",
    "summary",
    "regenerateSummary",
    "captions",
    "setCaptions",
    "captionFinals",
    "analytics",
    "reindex",
  ])
    assert.equal(Object.hasOwn(api, name), false, name);
});

test("recording creation and authorized playback remain available", async () => {
  const requests = [];
  globalThis.fetch = async (input, init) => {
    requests.push({ input, init });
    return Response.json({
      item: { uuid: "record", status: "ready", files: [] },
    });
  };
  await api.startRecording("conference", "audio_only");
  await api.recording("conference", "record");
  assert.equal(requests[0].input, "/api/v1/conferences/conference/recordings");
  assert.equal(requests[0].init.method, "POST");
  assert.deepEqual(JSON.parse(requests[0].init.body), {
    mode: "audio_only",
    segmentDurationSec: 5,
  });
  assert.equal(
    requests[1].input,
    "/api/v1/conferences/conference/recordings/record",
  );
});

test("conference keyword search preserves membership and date filters", async () => {
  globalThis.fetch = async (input) => {
    const url = new URL(input, "https://synthetic.invalid");
    assert.equal(url.pathname, "/api/v1/search");
    assert.equal(url.searchParams.get("q"), "План");
    assert.equal(url.searchParams.get("source"), "conference");
    assert.equal(url.searchParams.get("mode"), "keyword");
    assert.equal(url.searchParams.get("membership"), "owned");
    assert.equal(url.searchParams.get("participantId"), "participant");
    assert.equal(url.searchParams.get("from"), "2026-10-01T00:00:00Z");
    assert.equal(url.searchParams.get("offset"), "20");
    return Response.json({
      items: [],
      total: 0,
      limit: 20,
      offset: 20,
      effectiveMode: "keyword",
    });
  };
  await api.search(
    {
      q: "План",
      source: "conference",
      mode: "keyword",
      membership: "owned",
      participantId: "participant",
      from: "2026-10-01T00:00:00Z",
    },
    20,
  );
});

test("notification preferences contain only supported event and delivery settings", async () => {
  const preferences = {
    invitation: true,
    reminder: true,
    recording: true,
    email: false,
    push: false,
  };
  globalThis.fetch = async (input, init) => {
    assert.equal(input, "/api/v1/notifications/preferences");
    assert.deepEqual(JSON.parse(init.body), preferences);
    return Response.json(preferences);
  };
  assert.deepEqual(
    await api.saveNotificationPreferences(preferences),
    preferences,
  );
});
