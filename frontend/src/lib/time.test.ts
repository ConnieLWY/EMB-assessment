import { describe, expect, it } from "vitest";
import { compareTimestamp, localToUtc, mergeChargers } from "./time";
import type { Charger } from "../types/api";

const charger = (updated_at: string, status: Charger["status"]): Charger => ({
  id: "charger-1",
  name: "Charger 1",
  location: "Level 1, Bay A",
  status,
  updated_at,
});

describe("time handling", () => {
  it("compares backend microseconds without dropping precision", () => {
    expect(
      compareTimestamp(
        "2026-10-01T10:00:00.123457Z",
        "2026-10-01T10:00:00.123456Z",
      ),
    ).toBeGreaterThan(0);
    expect(
      compareTimestamp("2026-10-01T10:00:00Z", "2026-10-01T10:00:00.000001Z"),
    ).toBeLessThan(0);
    expect(
      mergeChargers(
        [charger("2026-10-01T10:00:00.123456Z", "CHARGING")],
        [charger("2026-10-01T10:00:00.123455Z", "AVAILABLE")],
      )[0].status,
    ).toBe("CHARGING");
  });

  it("converts a valid local datetime to UTC and rejects invalid input", () => {
    const local = "2026-10-01T14:30";
    expect(localToUtc(local)).toBe(new Date(local).toISOString());
    expect(() => localToUtc("not-a-date")).toThrow();
  });
});
