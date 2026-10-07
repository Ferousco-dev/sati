import type { DayHours, WeekHours } from "./content/types";

const DAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;
const DAY_NAMES: Record<(typeof DAYS)[number], string> = {
  sun: "Sunday",
  mon: "Monday",
  tue: "Tuesday",
  wed: "Wednesday",
  thu: "Thursday",
  fri: "Friday",
  sat: "Saturday",
};

/** Monday first, the way a shop lists its week. */
export const WEEK_ORDER = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"] as const;

export function dayName(day: (typeof DAYS)[number]): string {
  return DAY_NAMES[day];
}

/** 13:30 becomes 1:30 PM. */
export function formatClock(clock: string): string {
  const [h, m] = clock.split(":").map(Number);
  const suffix = h >= 12 ? "PM" : "AM";
  const hour = h % 12 === 0 ? 12 : h % 12;
  return `${hour}:${String(m).padStart(2, "0")} ${suffix}`;
}

function minutes(clock: string): number {
  const [h, m] = clock.split(":").map(Number);
  return h * 60 + m;
}

/** What the clock says in the store's own timezone, whatever timezone the visitor is in. */
function localParts(now: Date, timezone: string): { day: number; minutes: number } {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: timezone,
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(now);
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  const day = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].indexOf(get("weekday"));
  return { day, minutes: Number(get("hour")) * 60 + Number(get("minute")) };
}

export type OpenStatus = {
  open: boolean;
  /** A short line such as "Open now · closes 9:00 PM". */
  label: string;
};

export function openStatus(
  hours: WeekHours,
  timezone: string,
  temporarilyClosed: boolean,
  now: Date = new Date(),
): OpenStatus {
  const { day, minutes: current } = localParts(now, timezone);
  const today: DayHours = hours[DAYS[day]];

  if (!temporarilyClosed && today && current >= minutes(today.open) && current < minutes(today.close)) {
    return { open: true, label: `Open now · closes ${formatClock(today.close)}` };
  }
  if (temporarilyClosed) {
    return { open: false, label: "Temporarily closed" };
  }

  for (let offset = 0; offset <= 7; offset++) {
    const key = DAYS[(day + offset) % 7];
    const hoursForDay = hours[key];
    if (!hoursForDay) continue;
    if (offset === 0 && minutes(hoursForDay.open) <= current) continue;
    const when = offset === 0 ? "today" : offset === 1 ? "tomorrow" : DAY_NAMES[key];
    return { open: false, label: `Closed · opens ${when} ${formatClock(hoursForDay.open)}` };
  }
  return { open: false, label: "Closed" };
}

export function hasAnyHours(hours: WeekHours): boolean {
  return Object.values(hours).some(Boolean);
}
