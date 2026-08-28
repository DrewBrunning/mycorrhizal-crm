---
title: Testing
parent: Development
nav_order: 4
---

# Testing

## Backend Tests

### Running Tests

```sh
cd backend
go test ./...
```

### Test Database

Tests use an in-memory SQLite database that is auto-migrated before each test. No external dependencies required.

### Writing Tests

Use the test helpers in the `_test` files alongside each package. Set up a test DB with `database.NewTestDB()` (or equivalent), create a Gin test context, and call controller functions directly. Assert on the `httptest.ResponseRecorder`.

## Frontend Tests

There are no isolated frontend tests with yarn test, since this requires code/logic duplication for mocking. Instead Playwright is used to run integrated E2E tests.

### Setup

```sh
cd frontend
yarn playwright install  # install browsers (first time only)
```

The E2E tests expect the full application running on `http://localhost:7300`. `global-setup.ts` seeds a test user before the suite runs.

### Running E2E Tests

```sh
yarn test:e2e           # headless
yarn test:e2e:headed    # with browser visible
yarn test:e2e:ui        # Playwright UI mode
yarn test:e2e:debug     # debug mode
```

### Writing E2E Tests

Tests live in `frontend/e2e/`. Use the shared fixtures from `fixtures.ts` for login/logout helpers and the `TEST_USER` credentials from `global-setup.ts`. Group tests with `test.describe` and keep each test independent.

### Visual Regression (issue #258)

`e2e/visual.spec.ts` adds screenshot-based regression testing for a small, curated set of stable views — the dashboard, the contacts list, a contact detail page and the "Add reminder" dialog, at desktop (1280×720) and phone (390×844) widths. An unintended layout or theme change to a pinned view fails the e2e job as a pixel diff.

- Baselines are committed under `frontend/e2e/visual.spec.ts-snapshots/` and compared in CI as part of the normal Playwright run.
- Regenerate them after an **intentional** visual change:

  ```sh
  cd frontend
  npx playwright test visual.spec.ts --update-snapshots
  ```

  Review the regenerated images (the HTML report or `--ui` shows before/after and the diff) before committing them.
- The shots are made deterministic on purpose — see the header comment in `visual.spec.ts`. Dates are pinned with a frozen page clock, the app's primary font is injected as a committed webfont so rendering never depends on host fonts, and the dashboard/list responses are intercepted with fixed payloads (the dashboard's "Stay in Touch" column is server-random). Only add a view here that you are willing to regenerate when the design changes.
