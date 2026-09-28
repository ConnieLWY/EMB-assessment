import { useState } from "react";
import { api, ApiError } from "../lib/api";
import type { Charger, ConcurrencyDemoResult } from "../types/api";

const pretty = (value: unknown) => JSON.stringify(value, null, 2) ?? "null";

export function ConcurrencyDemo({
  chargers,
  onComplete,
  onUnauthorized,
}: {
  chargers: Charger[];
  onComplete: () => void;
  onUnauthorized?: () => void;
}) {
  const [selection, setSelection] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ConcurrencyDemoResult | null>(null);
  const eligible = chargers.filter((charger) => charger.status !== "MAINTENANCE");
  const chargerId = eligible.some((charger) => charger.id === selection)
    ? selection
    : eligible[0]?.id ?? "";

  const run = async () => {
    if (!chargerId || running) return;
    setRunning(true);
    setError(null);
    setResult(null);
    try {
      const next = await api.runConcurrencyDemo(chargerId);
      setResult(next);
      onComplete();
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) onUnauthorized?.();
      setError(cause instanceof Error ? cause.message : "Could not run the check.");
    } finally {
      setRunning(false);
    }
  };

  return (
    <section className="concurrency-demo" aria-labelledby="concurrency-demo-title">
      <div className="concurrency-demo-heading">
        <div>
          <span className="section-kicker">Live demo</span>
          <h3 id="concurrency-demo-title">Concurrency check</h3>
          <p>
            Ten parallel HTTP requests, split between demo and demo2, for one
            charger and time slot. A successful run creates one real reservation.
          </p>
        </div>
        <div className="concurrency-demo-controls">
          <label htmlFor="concurrency-charger">Charger</label>
          <select
            id="concurrency-charger"
            value={chargerId}
            onChange={(event) => setSelection(event.target.value)}
            disabled={running || eligible.length === 0}
          >
            {eligible.length === 0 && <option value="">No charger available</option>}
            {eligible.map((charger) => (
              <option key={charger.id} value={charger.id}>
                {charger.name}
              </option>
            ))}
          </select>
          <button
            className="button-primary"
            type="button"
            onClick={() => void run()}
            disabled={running || !chargerId}
          >
            {running ? "Running…" : "Run concurrency check"}
          </button>
        </div>
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      {result && (
        <div className="concurrency-demo-result" role="status">
          <p className={result.passed ? "demo-pass" : "demo-fail"}>
            <strong>{result.passed ? "Passed" : "Check failed"}</strong>
            <span>
              {result.created} created · {result.conflicts} conflicts · {result.persisted} saved
            </span>
          </p>
          <p className="concurrency-demo-slot">
            {result.charger_id} · {new Date(result.start_time).toLocaleString()} – {new Date(result.end_time).toLocaleString()}
          </p>
          <div className="concurrency-attempts">
            {result.results.map((attempt) => (
              <details className="concurrency-attempt" key={attempt.number}>
                <summary>
                  <strong>Attempt #{attempt.number}</strong>
                  <span>{attempt.user}</span>
                  <span className={attempt.status === 201 ? "demo-pass" : "demo-fail"}>
                    HTTP {attempt.status || "failed"} · {attempt.code ?? "CREATED"}
                  </span>
                  <span className="concurrency-attempt-duration">{attempt.duration_ms.toFixed(1)} ms</span>
                  <time className="concurrency-attempt-time" dateTime={attempt.requested_at}>Sent {attempt.requested_at}</time>
                </summary>
                <div className="concurrency-attempt-detail">
                  <div className="concurrency-timing">
                    <div><span>Request sent</span><time dateTime={attempt.requested_at}>{attempt.requested_at}</time></div>
                    <div><span>Response received</span><time dateTime={attempt.responded_at}>{attempt.responded_at}</time></div>
                    <div><span>Elapsed</span><strong>{attempt.duration_ms.toFixed(1)} ms</strong></div>
                  </div>
                  <div className="concurrency-exchange">
                    <section aria-label={`Request ${attempt.number}`}>
                      <h4>Request</h4>
                      <p className="concurrency-request-line"><strong>{attempt.request.method}</strong> <code>{attempt.request.url}</code></p>
                      <h5>Headers</h5>
                      <pre>{pretty(attempt.request.headers)}</pre>
                      <h5>Body</h5>
                      <pre>{pretty(attempt.request.body)}</pre>
                    </section>
                    <section aria-label={`Response ${attempt.number}`}>
                      <h4>Response</h4>
                      <p className="concurrency-request-line"><strong>HTTP {attempt.response.status || "failed"}</strong></p>
                      <h5>Headers</h5>
                      <pre>{pretty(attempt.response.headers)}</pre>
                      <h5>Body</h5>
                      <pre>{pretty(attempt.response.body)}</pre>
                    </section>
                  </div>
                </div>
              </details>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}
