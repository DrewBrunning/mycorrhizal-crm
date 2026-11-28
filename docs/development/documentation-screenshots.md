---
title: Documentation screenshots
parent: Development
nav_order: 21
---

# Documentation screenshots

The screenshots in the user guide (`docs/assets/screenshots/*.png`) are generated, not captured by hand, so they can be refreshed when the UI changes (issue #2244).

```sh
scripts/docs-screenshots.sh
```

The script starts the pen-test/demo stack (`docker-compose.pentest.yml`, `demo` profile: one user with 35 contacts and clean names; see [Pen-test and demo environment](pentest-environment.html)), reads its seeded login, and runs the Playwright generator in `frontend/e2e/docs-screenshots/` with `frontend/playwright.docs.config.ts`. It needs Docker, `jq`, Node 24.15 or newer and `yarn install` in `frontend/`. Set `DOCS_KEEP_STACK=1` to leave the stack running. The compose file pins the app's container name, so stop a pen-test stack you already have running first; the script refuses to start while one exists.

- The PNGs are committed. Regenerate them in the same PR as a visible UI change, and review the diff like any other.
- The generator is deliberately outside the CI test suites (its own config, and the main config ignores its directory), so a UI change never fails a build because of a screenshot.
- To add a screenshot, add a `shot(page, 'name')` call in `frontend/e2e/docs-screenshots/docsScreenshots.spec.ts` and reference `assets/screenshots/name.png` from the page.
- The demo data is the same canonical manifest the pen-test environment uses, so screenshots show realistic data and never real contacts. Do not point the script at a real instance.
