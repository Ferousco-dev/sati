import { test } from "node:test";
import assert from "node:assert/strict";
import { formatClock, hasAnyHours, openStatus } from "./hours.ts";
import type { WeekHours } from "./content/types.ts";

const week: WeekHours = {
  mon: { open: "09:00", close: "21:00" },
  tue: { open: "09:00", close: "21:00" },
  wed: null,
  thu: null,
  fri: null,
  sat: { open: "10:00", close: "18:00" },
  sun: null,
};

// 2026-10-05 is a Monday. Lagos is UTC+1 all year.
const lagos = (iso: string) => new Date(`${iso}+01:00`);

test("open during the day", () => {
  const s = openStatus(week, "Africa/Lagos", false, lagos("2026-10-05T12:00:00"));
  assert.equal(s.open, true);
  assert.equal(s.label, "Open now · closes 9:00 PM");
});

test("opening and closing times are exact", () => {
  assert.equal(openStatus(week, "Africa/Lagos", false, lagos("2026-10-05T09:00:00")).open, true);
  assert.equal(openStatus(week, "Africa/Lagos", false, lagos("2026-10-05T21:00:00")).open, false);
});

test("before opening says opens today", () => {
  const s = openStatus(week, "Africa/Lagos", false, lagos("2026-10-05T07:30:00"));
  assert.equal(s.label, "Closed · opens today 9:00 AM");
});

test("after closing says opens tomorrow", () => {
  const s = openStatus(week, "Africa/Lagos", false, lagos("2026-10-05T22:00:00"));
  assert.equal(s.label, "Closed · opens tomorrow 9:00 AM");
});

test("a run of closed days names the next opening day", () => {
  const s = openStatus(week, "Africa/Lagos", false, lagos("2026-10-07T12:00:00"));
  assert.equal(s.label, "Closed · opens Saturday 10:00 AM");
});

test("the store timezone decides, not the visitor's", () => {
  // 20:30 UTC is 21:30 in Lagos, after closing.
  assert.equal(openStatus(week, "Africa/Lagos", false, new Date("2026-10-05T20:30:00Z")).open, false);
  // 08:30 UTC is 09:30 in Lagos, already open.
  assert.equal(openStatus(week, "Africa/Lagos", false, new Date("2026-10-05T08:30:00Z")).open, true);
});

test("a temporary closure overrides the schedule", () => {
  const s = openStatus(week, "Africa/Lagos", true, lagos("2026-10-05T12:00:00"));
  assert.deepEqual(s, { open: false, label: "Temporarily closed" });
});

test("no hours at all", () => {
  const empty: WeekHours = { mon: null, tue: null, wed: null, thu: null, fri: null, sat: null, sun: null };
  assert.equal(openStatus(empty, "Africa/Lagos", false).label, "Closed");
  assert.equal(hasAnyHours(empty), false);
  assert.equal(hasAnyHours(week), true);
});

test("times are shown in 12 hour form", () => {
  assert.equal(formatClock("00:00"), "12:00 AM");
  assert.equal(formatClock("12:00"), "12:00 PM");
  assert.equal(formatClock("13:30"), "1:30 PM");
  assert.equal(formatClock("21:05"), "9:05 PM");
});
