import type { Charger, ConcurrencyDemoResult, Reservation, ReserveInput, User } from "../types/api";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export interface ReservationPage {
  reservations: Reservation[];
  page: number;
  limit: number;
  total: number;
}

export type ReservationGroup = "upcoming" | "history";
export const RESERVATION_PAGE_SIZE = 5;

async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(path, {
    method,
    credentials: "include",
    signal,
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as {
      error?: { code?: string; message?: string };
    } | null;
    throw new ApiError(
      response.status,
      payload?.error?.code ?? "HTTP_ERROR",
      payload?.error?.message ?? `Request failed (${response.status}).`,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const api = {
  login: async (username: string, password: string, signal?: AbortSignal) =>
    (
      await request<{ user: User }>(
        "/api/auth/login",
        "POST",
        { username, password },
        signal,
      )
    ).user,
  logout: (signal?: AbortSignal) =>
    request<void>("/api/auth/logout", "POST", undefined, signal),
  me: async (signal?: AbortSignal) =>
    (await request<{ user: User }>("/api/auth/me", "GET", undefined, signal))
      .user,
  listChargers: async (signal?: AbortSignal) =>
    (
      await request<{ chargers: Charger[] }>(
        "/api/chargers",
        "GET",
        undefined,
        signal,
      )
    ).chargers,
  reserve: async (
    chargerId: string,
    input: ReserveInput,
    signal?: AbortSignal,
  ) =>
    (
      await request<{ reservation: Reservation }>(
        `/api/chargers/${encodeURIComponent(chargerId)}/reserve`,
        "POST",
        input,
        signal,
      )
    ).reservation,
  listReservations: (group: ReservationGroup, page = 1, signal?: AbortSignal) =>
    request<ReservationPage>(
      `/api/reservations?group=${group}&page=${page}&limit=${RESERVATION_PAGE_SIZE}`,
      "GET",
      undefined,
      signal,
    ),
  cancelReservation: async (id: string, signal?: AbortSignal) =>
    (
      await request<{ reservation: Reservation }>(
        `/api/reservations/${encodeURIComponent(id)}/cancel`,
        "POST",
        undefined,
        signal,
      )
    ).reservation,
  runConcurrencyDemo: (chargerId: string) =>
    request<ConcurrencyDemoResult>("/api/demo/concurrency", "POST", {
      charger_id: chargerId,
    }),
};

export function websocketURL(): string {
  const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${scheme}//${window.location.host}/api/ws`;
}
