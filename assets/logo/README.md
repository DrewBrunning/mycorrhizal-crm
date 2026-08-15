# Logo

Canonical logo assets — the mushroom-and-mycelium mark, shared across the web app (`frontend/public/`)
and any future mobile clients, same reasoning as `assets/fonts/` and `assets/colors/`.

There are now **two silhouettes**, not one, because a single mark can't serve both jobs well:

- **Normal mark** (`mark-mycelium-light-*`, `mark-mycelium-dark-*`, `mark-white-1024.png`) — the full mark
  with its fine root/branch detail, used everywhere that detail can survive: the login/settings logo and
  the 192px+ app icons.
- **Simplified mark** (`simplified-mark-mycelium-light-*`, `simplified-mark-white-*`) — a bolder, reduced
  silhouette purpose-built to stay legible at tiny sizes. Used for the favicon (16/32px) and OS
  notification icons (96px), where the normal mark's detail used to collapse into a soft blob (see
  "Known limitation, resolved" below).

Both families are a single flat color with anti-aliased edges (not a gradient or multi-color
illustration), so recoloring loses nothing and stays pixel-perfect at the edges.

## File inventory

**Masters** (1024px, source of truth — everything else is resized/recolored/composited from these):

| File | Color | Background |
|---|---|---|
| `mark-mycelium-light-1024.png` | `mycelium` light-mode (`#3E543E`) | transparent |
| `mark-mycelium-dark-1024.png` | `mycelium` dark-mode (`#9EB698`) | transparent |
| `mark-white-1024.png` | white | transparent |
| `icon-1024.png` | `mark-mycelium-light` | filled `bone` light (`#FAF5EA`) |
| `icon-dark-1024.png` | `mark-mycelium-dark` | filled `#2B261B` (see note below) |

`icon-*` are the app-icon composites: the mark plus a filled background, at the generous margin already
present in the source composition (~64% of the canvas — comfortably inside the ~80% safe zone maskable
icons need, no extra padding required).

Note on `icon-dark-*`'s background: sampled at exactly `#2B261B`, which is close to but not identical to
any named `bone` dark-mode token in `assets/colors/tokens.json` (nearest is `bone.active` at `#2A261E`).
Worth snapping to an actual token next time these are regenerated — `#2B261B` isn't derived from the
palette file, just close to it.

**Generated sizes**, resized/recolored from the masters above:

| Prefix | Sizes | Color/background |
|---|---|---|
| `mark-mycelium-light-*` | 192, 512 | light mycelium, transparent |
| `mark-mycelium-dark-*` | 192, 512 | dark mycelium, transparent |
| `icon-*` | 192, 512 | light mycelium, filled bone |
| `icon-dark-*` | 192, 512 | dark mycelium, filled `#2B261B` |
| `simplified-mark-mycelium-light-*` | 16, 32, 96, 192, 512 | light mycelium, transparent |
| `simplified-mark-white-*` | 32, 96, 192, 512 | white, transparent |
| `favicon.png` | 48 | light mycelium, transparent — source frame for `favicon.ico` |
| `favicon.ico` | multi-res (16/24/32/48/64) | light mycelium, transparent |

## Where each is used

| `assets/logo/` file | Copied to (`frontend/public/`) | Used for |
|---|---|---|
| `favicon.ico` | `favicon.ico` | Browser tab icon |
| `simplified-mark-mycelium-light-16.png` | `favicon-16x16.png` | Browser tab icon |
| `simplified-mark-mycelium-light-32.png` | `favicon-32x32.png` | Browser tab icon |
| `mark-mycelium-light-192.png` / `-512.png` | `mycorrhizal-logo-light_192.png` / `_512.png` | Login page / Settings "About" logo, light mode (`BrandLogo.tsx`) |
| `mark-mycelium-dark-192.png` / `-512.png` | `mycorrhizal-logo-dark_192.png` / `_512.png` | Same, dark mode |
| `icon-192.png` / `-512.png` | `icons/light/icon-192.png` / `icon-512.png` | PWA home-screen icon, light mode (`manifest.json`, `prefers-color-scheme: light`) |
| `icon-dark-192.png` / `-512.png` | `icons/dark/icon-192.png` / `icon-512.png` | Same, dark mode |
| `mark-mycelium-light-512.png` | `icons/light/maskable-icon-512.png` | PWA maskable icon, light mode — reuses the transparent mark directly rather than a separate filled asset; the OS applies its own background/mask |
| `mark-mycelium-dark-512.png` | `icons/dark/maskable-icon-512.png` | Same, dark mode |
| `simplified-mark-white-96.png` | `icons/light/notification-icon-96.png` and `icons/dark/notification-icon-96.png` (identical) | OS notification icon — generated ahead of need, not yet referenced by any app code. Ticket: `N9` |
| — (`icon-1024.png`, bone-filled, resized to 180px) | `apple-touch-icon.png` | iOS home screen (`index.html`'s `<link rel="apple-touch-icon">` — Apple doesn't read `manifest.json`) |

**Not currently consumed anywhere**: `mark-white-1024.png`, `simplified-mark-white-32/192/512.png`
(reserved for future use — e.g. more notification-icon densities or dark/colored-background contexts),
and the 1024px masters themselves (kept only as regeneration sources).

**Orphaned in `frontend/public/`**: `android-chrome-192x192.png` and `android-chrome-512x512.png`.
`manifest.json` used to point at these; it was repointed to `icons/light/` and `icons/dark/` with
`prefers-color-scheme` variants, and nothing references the `android-chrome-*` files anymore. Safe to
delete — flagging here rather than deleting silently since it's a file, not an oversight to just fix.

## Recoloring / regenerating

The mask-based workflow the previous version of this doc described (`mark-alpha-mask_1024.png`,
"flat-fill a color + apply this mask as opacity") no longer applies — that mask file was deleted when
this asset set was regenerated (`22ac8ae Remove old icons`), and this round's masters were produced
externally without keeping an equivalent reusable source. If you need a new color variant of either mark,
you're regenerating from scratch, not recoloring in place.

If you anticipate needing more color variants later (a third brand color, a themed variant, etc.), it's
worth producing and committing a fresh alpha mask *this* time so the next recolor is `convert -size
1024x1024 xc:"#TARGET_HEX" mask.png -compose CopyOpacity -composite output.png` instead of a full
re-export. Not done proactively here since there's no concrete second use yet.

## Known limitation, resolved (2026-08-04)

The previous version of this doc noted the normal mark's fine root/branch detail didn't hold up at
16×16/32×32 — it read as a soft textured blob rather than a crisp shape. That's what the simplified mark
family exists to fix: a bolder, reduced silhouette used specifically at the sizes where detail can't
survive (favicon, notification icon). The normal mark is still used everywhere 192px and up, where it
reads clearly.
