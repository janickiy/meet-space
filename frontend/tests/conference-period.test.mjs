import assert from "node:assert/strict";
import test from "node:test";
import { conferencePeriodRange } from "../src/conferencePeriod.ts";

test("all time clears both meeting date bounds", () => {
  assert.deepEqual(conferencePeriodRange("all"), { from: "", to: "" });
});

test("quick periods include today and update both local date bounds", () => {
  const now = new Date(2026, 9, 28, 12, 30);
  for (const [period, from] of [
    ["7", "2026-10-22"],
    ["30", "2026-09-29"],
    ["90", "2026-07-31"],
  ]) {
    assert.deepEqual(conferencePeriodRange(period, now), {
      from,
      to: "2026-10-28",
    });
  }
  assert.equal(now.getDate(), 28, "the reference date must not be mutated");
});

test("calendar periods cross year and leap-day boundaries", () => {
  assert.deepEqual(conferencePeriodRange("7", new Date(2026, 0, 3)), {
    from: "2025-12-28",
    to: "2026-01-03",
  });
  assert.deepEqual(conferencePeriodRange("7", new Date(2024, 2, 2)), {
    from: "2024-02-25",
    to: "2024-03-02",
  });
});

test("period dates stay local across a daylight-saving transition", () => {
  const previous = process.env.TZ;
  try {
    process.env.TZ = "America/New_York";
    assert.deepEqual(conferencePeriodRange("7", new Date(2026, 2, 10, 0, 15)), {
      from: "2026-03-04",
      to: "2026-03-10",
    });
  } finally {
    if (previous === undefined) delete process.env.TZ;
    else process.env.TZ = previous;
  }
});
