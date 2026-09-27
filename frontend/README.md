# Frontend

The React and TypeScript frontend shows the seeded chargers, follows live device status over WebSocket, and lets a signed-in user reserve a future time slot or cancel an unstarted reservation.

For the complete application in containers, run `docker compose up --build -d --wait` from the repository root and open `http://localhost:3000`. The production build is served by nginx, which proxies `/api` and WebSocket upgrades to the backend.

## Local development

Start the database and backend from the repository root, then run:

```bash
cd frontend
npm ci
npm run dev
```

Open `http://localhost:3000`. Vite proxies `/api` (including the WebSocket upgrade) to `http://localhost:8080`. Use the seeded `demo` or `demo2` account with password `DemoPass123!`. The frontend uses a same-origin, credentialed session cookie and does not store the password.

## Checks

```bash
npm test -- --run
npm run build
```

The development server retries the public charger snapshot after a failure and reconnects the WebSocket after a disconnect. Reservations are refreshed after booking or cancellation, on login, on live-feed reconnection, when the page becomes visible, and every five seconds while the authenticated page is visible. The cancel action is available for scheduled and waiting reservations; cancelled records remain in the list.
