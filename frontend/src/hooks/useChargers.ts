import { useCallback, useEffect, useRef, useState } from "react";
import { api, websocketURL } from "../lib/api";
import { compareTimestamp, mergeChargers } from "../lib/time";
import type { Charger, StatusEvent } from "../types/api";

function validEvent(value: unknown): value is StatusEvent {
  if (!value || typeof value !== "object") return false;
  const event = value as Partial<StatusEvent>;
  return (
    event.event === "CHARGER_STATUS_UPDATED" &&
    typeof event.data?.charger_id === "string" &&
    ["AVAILABLE", "CHARGING", "MAINTENANCE"].includes(
      event.data?.status ?? "",
    ) &&
    typeof event.data?.updated_at === "string"
  );
}

export function useChargers() {
  const [chargers, setChargers] = useState<Charger[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [connectionEpoch, setConnectionEpoch] = useState(0);
  const refreshRef = useRef<() => void>(() => {});

  useEffect(() => {
    let disposed = false;
    let generation = 0;
    let socket: WebSocket | null = null;
    let snapshotController: AbortController | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let snapshotTimer: ReturnType<typeof setTimeout> | null = null;
    let reconnectDelay = 1000;
    let snapshotDelay = 1000;
    const events = new Map<string, StatusEvent>();

    const clearSnapshotTimer = () => {
      if (snapshotTimer) clearTimeout(snapshotTimer);
      snapshotTimer = null;
    };
    const load = (currentGeneration: number) => {
      if (disposed || currentGeneration !== generation) return;
      clearSnapshotTimer();
      snapshotController?.abort();
      const controller = new AbortController();
      snapshotController = controller;
      api
        .listChargers(controller.signal)
        .then((snapshot) => {
          if (
            disposed ||
            currentGeneration !== generation ||
            controller.signal.aborted
          )
            return;
          const withEvents = snapshot.map((charger) => {
            const event = events.get(charger.id);
            return event &&
              compareTimestamp(event.data.updated_at, charger.updated_at) > 0
              ? {
                  ...charger,
                  status: event.data.status,
                  updated_at: event.data.updated_at,
                }
              : charger;
          });
          setChargers((current) => mergeChargers(withEvents, current));
          setLoading(false);
          setError(null);
          snapshotDelay = 1000;
        })
        .catch((cause: unknown) => {
          if (
            disposed ||
            currentGeneration !== generation ||
            controller.signal.aborted
          )
            return;
          if (cause instanceof DOMException && cause.name === "AbortError")
            return;
          setLoading(false);
          setError("Could not load chargers. Retrying…");
          snapshotTimer = setTimeout(
            () => load(currentGeneration),
            snapshotDelay,
          );
          snapshotDelay = Math.min(snapshotDelay * 2, 10_000);
        });
    };

    const connect = () => {
      if (disposed) return;
      const currentGeneration = ++generation;
      snapshotController?.abort();
      clearSnapshotTimer();
      socket?.close();
      const currentSocket = new WebSocket(websocketURL());
      socket = currentSocket;
      setConnected(false);
      // Opening the stream first lets incoming events win over an older snapshot.
      load(currentGeneration);
      currentSocket.onopen = () => {
        if (disposed || currentGeneration !== generation) return;
        setConnected(true);
        setConnectionEpoch((epoch) => epoch + 1);
        reconnectDelay = 1000;
        load(currentGeneration);
      };
      currentSocket.onmessage = (message) => {
        if (disposed || currentGeneration !== generation) return;
        try {
          const parsed: unknown = JSON.parse(message.data as string);
          if (!validEvent(parsed)) return;
          const previous = events.get(parsed.data.charger_id);
          if (
            previous &&
            compareTimestamp(
              parsed.data.updated_at,
              previous.data.updated_at,
            ) <= 0
          )
            return;
          events.set(parsed.data.charger_id, parsed);
          setChargers((current) => {
            const existing = current.find(
              (charger) => charger.id === parsed.data.charger_id,
            );
            if (!existing) return current;
            return mergeChargers(current, [
              {
                ...existing,
                status: parsed.data.status,
                updated_at: parsed.data.updated_at,
              },
            ]);
          });
        } catch {
          /* Ignore malformed public events. */
        }
      };
      currentSocket.onerror = () => currentSocket.close();
      currentSocket.onclose = () => {
        if (disposed || currentGeneration !== generation) return;
        setConnected(false);
        reconnectTimer = setTimeout(connect, reconnectDelay);
        reconnectDelay = Math.min(reconnectDelay * 2, 10_000);
      };
    };

    refreshRef.current = () => load(generation);
    const visible = () => {
      if (document.visibilityState !== "visible") return;
      load(generation);
      if (socket?.readyState !== WebSocket.OPEN) {
        if (reconnectTimer) clearTimeout(reconnectTimer);
        reconnectTimer = null;
        connect();
      }
    };
    document.addEventListener("visibilitychange", visible);
    connect();
    return () => {
      disposed = true;
      generation++;
      document.removeEventListener("visibilitychange", visible);
      if (reconnectTimer) clearTimeout(reconnectTimer);
      clearSnapshotTimer();
      snapshotController?.abort();
      socket?.close();
      refreshRef.current = () => {};
    };
  }, []);

  const refresh = useCallback(() => refreshRef.current(), []);
  return { chargers, loading, error, connected, connectionEpoch, refresh };
}
