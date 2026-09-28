import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../lib/api";
import type { Charger, StatusEvent } from "../types/api";
import { useChargers } from "./useChargers";

const makeCharger = (
  status: Charger["status"],
  updated_at: string,
): Charger => ({
  id: "charger-1",
  name: "Charger 1",
  location: "Level 1, Bay A",
  status,
  updated_at,
});

class FakeSocket {
  static instances: FakeSocket[] = [];
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  readyState = 0;
  constructor(public url: string) {
    FakeSocket.instances.push(this);
  }
  close() {
    this.readyState = 3;
    this.onclose?.();
  }
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  emit(event: StatusEvent) {
    this.onmessage?.({ data: JSON.stringify(event) } as MessageEvent<string>);
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  FakeSocket.instances = [];
  vi.stubGlobal("WebSocket", FakeSocket);
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("useChargers", () => {
  it("keeps a newer event that arrives before a stale snapshot", async () => {
    const snapshot = deferred<Charger[]>();
    vi.spyOn(api, "listChargers").mockReturnValue(snapshot.promise);
    const { result } = renderHook(() => useChargers());
    expect(FakeSocket.instances).toHaveLength(1);
    const socket = FakeSocket.instances[0];
    act(() =>
      socket.emit({
        event: "CHARGER_STATUS_UPDATED",
        data: {
          charger_id: "charger-1",
          status: "CHARGING",
          updated_at: "2026-10-01T10:00:00.123457Z",
        },
      }),
    );
    await act(async () =>
      snapshot.resolve([
        makeCharger("AVAILABLE", "2026-10-01T10:00:00.123456Z"),
      ]),
    );
    expect(result.current.chargers[0].status).toBe("CHARGING");
  });

  it("ignores old connection snapshots and stale events after reconnect", async () => {
    vi.useFakeTimers();
    const first = deferred<Charger[]>();
    const second = deferred<Charger[]>();
    vi.spyOn(api, "listChargers")
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => useChargers());
    const old = FakeSocket.instances[0];
    act(() => old.close());
    await act(async () => vi.advanceTimersByTimeAsync(1000));
    expect(FakeSocket.instances).toHaveLength(2);
    await act(async () =>
      second.resolve([makeCharger("CHARGING", "2026-10-01T10:00:00.123457Z")]),
    );
    await act(async () =>
      first.resolve([makeCharger("AVAILABLE", "2026-10-01T10:00:00.123456Z")]),
    );
    act(() =>
      old.emit({
        event: "CHARGER_STATUS_UPDATED",
        data: {
          charger_id: "charger-1",
          status: "MAINTENANCE",
          updated_at: "2026-10-01T10:00:00.123458Z",
        },
      }),
    );
    expect(result.current.chargers[0].status).toBe("CHARGING");
  });

  it("retries a failed snapshot instead of showing an empty successful state", async () => {
    vi.useFakeTimers();
    const list = vi
      .spyOn(api, "listChargers")
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce([
        makeCharger("AVAILABLE", "2026-10-01T10:00:00Z"),
      ]);
    const { result } = renderHook(() => useChargers());
    await act(async () => Promise.resolve());
    expect(result.current.error).toMatch(/retry/i);
    await act(async () => vi.advanceTimersByTimeAsync(1000));
    expect(list).toHaveBeenCalledTimes(2);
    expect(result.current.chargers).toHaveLength(1);
  });
});
