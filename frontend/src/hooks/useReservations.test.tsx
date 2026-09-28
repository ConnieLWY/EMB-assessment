import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../lib/api";
import type { Reservation } from "../types/api";
import { useReservations } from "./useReservations";

const reservation: Reservation = {
  id: "reservation-1",
  user_id: "user-1",
  charger_id: "charger-1",
  status: "SCHEDULED",
  start_time: "2026-10-01T14:00:00Z",
  end_time: "2026-10-01T15:00:00Z",
  created_at: "2026-09-27T10:00:00Z",
  updated_at: "2026-09-27T10:00:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("useReservations", () => {
  it("keeps backend order and loads the selected page", async () => {
    const items: Reservation[] = [
      {
        ...reservation,
        id: "past-old",
        start_time: "2026-09-28T14:00:00Z",
        end_time: "2026-09-28T15:00:00Z",
        status: "COMPLETED",
      },
      {
        ...reservation,
        id: "future-late",
        start_time: "2026-10-03T14:00:00Z",
        end_time: "2026-10-03T15:00:00Z",
      },
      {
        ...reservation,
        id: "past-recent",
        start_time: "2026-09-30T14:00:00Z",
        end_time: "2026-09-30T15:00:00Z",
        status: "COMPLETED",
      },
      {
        ...reservation,
        id: "current",
        start_time: "2026-10-01T11:00:00Z",
        end_time: "2026-10-01T13:00:00Z",
        status: "ACTIVE",
      },
      {
        ...reservation,
        id: "future-near",
        start_time: "2026-10-01T14:00:00Z",
        end_time: "2026-10-01T15:00:00Z",
      },
    ];
    const list = vi
      .spyOn(api, "listReservations")
      .mockImplementation(async (_group, page = 1) => ({
        reservations: page === 1 ? items.slice(0, 2) : items.slice(2, 4),
        page,
        limit: 2,
        total: 5,
      }));
    const { result } = renderHook(() =>
      useReservations("user-1", vi.fn(), 0, "history"),
    );
    await waitFor(() =>
      expect(result.current.reservations.map((item) => item.id)).toEqual([
        "past-old",
        "future-late",
      ]),
    );
    expect(result.current.total).toBe(5);
    act(() => result.current.goToPage(2));
    await waitFor(() =>
      expect(result.current.reservations.map((item) => item.id)).toEqual([
        "past-recent",
        "current",
      ]),
    );
    expect(list).toHaveBeenLastCalledWith(
      "history",
      2,
      expect.any(AbortSignal),
    );
  });

  it("keeps upcoming and history page numbers independent", async () => {
    const list = vi
      .spyOn(api, "listReservations")
      .mockImplementation(async (group, page = 1) => ({
        reservations: [{ ...reservation, id: `${group}-${page}` }],
        page,
        limit: 5,
        total: 21,
      }));
    const { result } = renderHook(() => ({
      upcoming: useReservations("user-1", vi.fn(), 0, "upcoming"),
      history: useReservations("user-1", vi.fn(), 0, "history"),
    }));
    await waitFor(() =>
      expect(result.current.history.reservations).toHaveLength(1),
    );
    expect(result.current.upcoming.pageCount).toBe(5);
    act(() => result.current.upcoming.goToPage(2));
    await waitFor(() =>
      expect(result.current.upcoming.reservations[0]?.id).toBe("upcoming-2"),
    );
    expect(result.current.history.page).toBe(1);
    expect(list).toHaveBeenCalledWith("history", 1, expect.any(AbortSignal));
  });

  it("returns to the last page when a group shrinks", async () => {
    const list = vi
      .spyOn(api, "listReservations")
      .mockResolvedValueOnce({
        reservations: [reservation],
        page: 1,
        limit: 5,
        total: 6,
      })
      .mockResolvedValueOnce({ reservations: [], page: 2, limit: 5, total: 5 })
      .mockResolvedValueOnce({
        reservations: [reservation],
        page: 1,
        limit: 5,
        total: 5,
      });
    const { result } = renderHook(() => useReservations("user-1", vi.fn(), 0));
    await waitFor(() => expect(result.current.total).toBe(6));
    act(() => result.current.goToPage(2));
    await waitFor(() => expect(result.current.page).toBe(1));
    await waitFor(() => expect(result.current.reservations).toHaveLength(1));
    expect(list).toHaveBeenLastCalledWith(
      "upcoming",
      1,
      expect.any(AbortSignal),
    );
  });

  it("clears private data on logout and rejects a late response", async () => {
    const pending = deferred<{
      reservations: Reservation[];
      page: number;
      limit: number;
      total: number;
    }>();
    vi.spyOn(api, "listReservations")
      .mockReturnValueOnce(
        Promise.resolve({
          reservations: [reservation],
          page: 1,
          limit: 10,
          total: 1,
        }),
      )
      .mockReturnValueOnce(pending.promise);
    const onUnauthorized = vi.fn();
    const { result, rerender } = renderHook(
      ({ userId }) => useReservations(userId, onUnauthorized, 0),
      { initialProps: { userId: "user-1" as string | null } },
    );
    await waitFor(() => expect(result.current.reservations).toHaveLength(1));
    act(() => result.current.refresh());
    rerender({ userId: null });
    expect(result.current.reservations).toEqual([]);
    await act(async () =>
      pending.resolve({
        reservations: [reservation],
        page: 1,
        limit: 10,
        total: 1,
      }),
    );
    expect(result.current.reservations).toEqual([]);
  });

  it("clears a signed-in user after a 401", async () => {
    vi.spyOn(api, "listReservations").mockRejectedValue(
      new ApiError(401, "UNAUTHENTICATED", "Sign in to continue."),
    );
    const onUnauthorized = vi.fn();
    const { result } = renderHook(() =>
      useReservations("user-1", onUnauthorized, 0),
    );
    await waitFor(() => expect(onUnauthorized).toHaveBeenCalledOnce());
    expect(result.current.reservations).toEqual([]);
  });

  it("refreshes after a live-feed reconnect without duplicating the login fetch", async () => {
    const list = vi
      .spyOn(api, "listReservations")
      .mockResolvedValue({ reservations: [], page: 1, limit: 10, total: 0 });
    const onUnauthorized = vi.fn();
    const { rerender } = renderHook(
      ({ userId, epoch }) => useReservations(userId, onUnauthorized, epoch),
      {
        initialProps: { userId: null as string | null, epoch: 1 },
      },
    );
    rerender({ userId: "user-1", epoch: 1 });
    await waitFor(() => expect(list).toHaveBeenCalledTimes(1));
    rerender({ userId: "user-1", epoch: 2 });
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  });

  it("pauses polling while hidden and refreshes when visible again", async () => {
    vi.useFakeTimers();
    let hidden = false;
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => (hidden ? "hidden" : "visible"),
    });
    const list = vi
      .spyOn(api, "listReservations")
      .mockResolvedValue({ reservations: [], page: 1, limit: 10, total: 0 });
    const onUnauthorized = vi.fn();
    renderHook(() => useReservations("user-1", onUnauthorized, 0));
    await act(async () => Promise.resolve());
    expect(list).toHaveBeenCalledTimes(1);
    hidden = true;
    await act(async () => vi.advanceTimersByTimeAsync(10_000));
    expect(list).toHaveBeenCalledTimes(1);
    hidden = false;
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    expect(list).toHaveBeenCalledTimes(2);
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
  });
});
