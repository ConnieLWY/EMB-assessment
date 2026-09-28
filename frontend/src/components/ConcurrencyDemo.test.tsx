import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../lib/api";
import type { Charger } from "../types/api";
import { ConcurrencyDemo } from "./ConcurrencyDemo";

afterEach(() => vi.restoreAllMocks());

it("runs the two-user check and shows all ten outcomes", async () => {
  const chargers: Charger[] = [{
    id: "charger-1", name: "Charger 1", location: "Bay A", status: "AVAILABLE", updated_at: "",
  }];
  const results = Array.from({ length: 10 }, (_, i) => ({
    number: i + 1,
    user: i % 2 ? "demo2" : "demo",
    status: i === 0 ? 201 : 409,
    code: i === 0 ? undefined : "RESERVATION_CONFLICT",
    requested_at: "2026-10-05T12:00:00.123Z",
    responded_at: "2026-10-05T12:00:00.147Z",
    duration_ms: 24,
    request: {
      method: "POST",
      url: "http://127.0.0.1:8080/api/chargers/charger-1/reserve",
      headers: { Cookie: ["ev_session=[redacted]"], Origin: ["http://localhost:8080"] },
      body: { user_id: i % 2 ? "demo2-id" : "demo-id", start_time: "2026-10-05T12:00:00Z", end_time: "2026-10-05T13:00:00Z" },
    },
    response: {
      status: i === 0 ? 201 : 409,
      headers: { "Content-Type": ["application/json"] },
      body: i === 0 ? { reservation: { id: "winner" } } : { error: { code: "RESERVATION_CONFLICT" } },
    },
  }));
  const run = vi.spyOn(api, "runConcurrencyDemo").mockResolvedValue({
    charger_id: "charger-1",
    start_time: "2026-10-05T12:00:00Z",
    end_time: "2026-10-05T13:00:00Z",
    created: 1,
    conflicts: 9,
    persisted: 1,
    passed: true,
    results,
  });
  const onComplete = vi.fn();

  render(<ConcurrencyDemo chargers={chargers} onComplete={onComplete} />);
  await userEvent.click(screen.getByRole("button", { name: "Run concurrency check" }));

  expect(run).toHaveBeenCalledWith("charger-1");
  expect(await screen.findByText(/1 created · 9 conflicts · 1 saved/)).toBeInTheDocument();
  expect(screen.getAllByText(/Attempt #/)).toHaveLength(10);
  await userEvent.click(screen.getByText("Attempt #1"));
  const detail = screen.getByText("Attempt #1").closest("details") as HTMLElement;
  expect(detail).toHaveAttribute("open");
  expect(within(detail).getByText("2026-10-05T12:00:00.123Z")).toBeInTheDocument();
  expect(within(detail).getByText("2026-10-05T12:00:00.147Z")).toBeInTheDocument();
  expect(within(detail).getByText("http://127.0.0.1:8080/api/chargers/charger-1/reserve")).toBeInTheDocument();
  expect(within(detail).getByText(/"user_id": "demo-id"/)).toBeInTheDocument();
  expect(within(detail).getByText(/"id": "winner"/)).toBeInTheDocument();
  expect(onComplete).toHaveBeenCalled();
});
