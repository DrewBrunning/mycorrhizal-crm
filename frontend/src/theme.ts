// theme.ts
//
// Palette values come from assets/colors/tokens.json (OKLCH-derived; see
// assets/colors/README.md for the conversion math and the hover/active/
// contrastText derivation rules). `light`/`dark` on primary/secondary are
// deliberately omitted where we don't have a bespoke tint -- MUI's own
// augmentColor() fills them in from `main`.
import { createTheme } from '@mui/material/styles';

export const lightTheme = createTheme({
  palette: {
    mode: 'light',

    primary: {
      main: '#3E543E', // mycelium
      dark: '#314631', // mycelium hover (also used by MUI for contained-button hover/emphasis)
      contrastText: '#FFFFFF',
    },

    secondary: {
      main: '#97A390', // lichen
      dark: '#879481', // lichen hover
      contrastText: '#30271F', // bark -- verified 5.54:1 (AA); white fails at 2.64:1
    },

    background: {
      default: '#FAF5EA', // bone
      paper: '#EFE7D9', // parchment -- cards/sidebars. Dialogs/menus/popovers use the
      // lighter "paper" token (#FFFFFF) instead, see MuiDialog/MuiPopover/MuiMenu below.
    },

    text: {
      primary: '#30271F', // bark
      secondary: '#595148', // soil
    },

    divider: '#CBC7BF', // hypha

    // Mushroom-themed status colors -- see assets/colors/README.md for the
    // two corrections made before wiring these in (moss/success was too
    // close in hue to mycelium/lichen; laccaria/info's light value didn't
    // clear 4.5:1 with any label color).
    success: {
      // #1792: status colours double as *text* (outlined chips, "On track"
      // lines), so every `main` below must clear 4.5:1 on all three light
      // surfaces (parchment is the hardest). moss was #0D844C (3.87:1 on
      // parchment) -> 5.16:1; white labels on the fill 6.33:1.
      main: '#0A6E3F', // moss
      dark: '#046136', // moss hover
      contrastText: '#FFFFFF',
    },
    warning: {
      // #1792: chanterelle was #D3A563 -- 1.83:1 as text on parchment (the
      // "Degraded" chip). Darkened to an amber-brown (4.83:1 on parchment); it
      // is now dark enough that the fill takes a white label (5.93:1) instead
      // of bark. Dark mode keeps the bright chanterelle (7.41:1 as text).
      main: '#8A5A00', // chanterelle
      dark: '#784D00', // chanterelle hover
      contrastText: '#FFFFFF',
    },
    error: {
      // #187/#195: russula was #AD5349 (oklch 0.55) — as *text* on the
      // parchment cards it only cleared 4.16:1. Darkened to oklch 0.52
      // (#A34A40): 4.74:1 as text on parchment, white labels on fills 5.82:1.
      main: '#A34A40', // russula
      dark: '#923B33', // russula hover
      contrastText: '#FFFFFF',
    },
    info: {
      // #1792: was #7B6B98 (3.89:1 as text on parchment); now 4.99:1.
      main: '#6A5A87', // laccaria (corrected from the original oklch(0.60 0.070 300))
      dark: '#5B4D75', // laccaria hover
      contrastText: '#FFFFFF',
    },
  },

  shape: {
    borderRadius: 10,
  },

  typography: {
    fontFamily: `"IBM Plex Sans", "Fira Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Oxygen", "Ubuntu", "Cantarell", "Helvetica Neue", "Droid Sans", sans-serif`,

    subtitle1: {
      fontWeight: 500,
    },
    body2: {
      color: '#595148', // soil
    },
    // T63: h5 is used exclusively for page-level headings app-wide (every
    // page's own title, the contact-name heading) -- confirmed no live
    // collision with MuiDialogTitle (defaults to h6, not h5) or any error/
    // warning surface. Safe to theme globally, unlike h6/subtitle1/subtitle2/
    // body2, which are all reused too widely (including inside dialogs) for
    // a blanket override.
    h5: {
      fontFamily: '"EB Garamond", serif',
    },
    // T63: overline is used exclusively for section-subheading labels (e.g.
    // "Food & Drink Preferences", "Clothing Sizes") -- same reasoning, no
    // dialog/error collision.
    overline: {
      fontFamily: '"IBM Plex Mono", monospace',
    },
  },

  components: {
    // #186: MUI's default `.Mui-focusVisible` treatment is a flat 12% black
    // tint, which measures ~1.3:1 against every surface in this palette --
    // nowhere near the 3:1 floor for 1.4.11. A real outline on MuiButtonBase
    // covers Button/IconButton/ListItemButton/Tab/MenuItem/Chip in one place
    // since they all compose ButtonBase. Text inputs don't compose
    // ButtonBase and keep their own focused-border indicator instead (see
    // the AppBar search field note below) -- do not add `outline: 0`
    // removal here, MuiInputBase-input's own focus styling is untouched.
    MuiButtonBase: {
      styleOverrides: {
        root: {
          '&.Mui-focusVisible': {
            outline: '2px solid #3E543E', // mycelium -- 6.73:1 on parchment, 7.60:1 on bone
            outlineOffset: '2px',
          },
        },
      },
    },

    MuiCard: {
      styleOverrides: {
        root: {
          boxShadow: '0px 1px 2px rgba(48, 39, 31, 0.06), 0px 2px 8px rgba(48, 39, 31, 0.04)',
        },
      },
    },

    MuiChip: {
      styleOverrides: {
        root: {
          fontWeight: 500,
        },
      },
    },

    MuiAppBar: {
      styleOverrides: {
        root: {
          // No explicit color override -- AppBar defaults to color="primary"
          // (mycelium), matching its "brand primary, navigation" role.
          boxShadow: 'none',
        },
      },
    },

    // "paper" token (elevated dialogs/floating elements) is one step lighter
    // than background.paper (parchment) -- applied here rather than as the
    // base background.paper, to keep the 3-tier bone/parchment/paper
    // elevation the tokens define instead of collapsing it to MUI's default
    // 2-tier default/paper model.
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundColor: '#FFFFFF', // paper
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          backgroundColor: '#FFFFFF', // paper
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          backgroundColor: '#FFFFFF', // paper
        },
      },
    },

    // #194: honour prefers-reduced-motion (WCAG 2.3.3, AAA) globally, rather
    // than restructuring lightTheme/darkTheme into functions of the
    // preference -- this CSS override catches every MUI transition (dialogs,
    // drawers, menus, ripples, collapse) plus the plain-CSS opacity
    // transitions in ContactHeader.tsx for free. Duplicated in darkTheme
    // below since the two createTheme calls are independent (see the h5/
    // overline/MuiButtonBase comments in this file for the same pattern).
    // Pinned by src/themeAccessibility.test.ts (the media-query block is
    // present in both themes) and by the reduced-motion e2e case in
    // e2e/accessibility.spec.ts (the override actually collapses a real
    // transition/animation/scroll in the browser).
    MuiCssBaseline: {
      styleOverrides: `
        @media (prefers-reduced-motion: reduce) {
          *, *::before, *::after {
            animation-duration: 0.01ms !important;
            animation-iteration-count: 1 !important;
            transition-duration: 0.01ms !important;
            scroll-behavior: auto !important;
          }
        }
      `,
    },
  },
});

export const darkTheme = createTheme({
  palette: {
    mode: 'dark',

    primary: {
      main: '#9EB698', // mycelium (dark mode: bright, not dark -- see contrastText note)
      dark: '#8EA789', // mycelium hover
      // mycelium is bright in dark mode (L 0.75), so a light label fails
      // contrast (1.73:1) -- verified a dark label works instead (7.92:1, AAA).
      contrastText: '#1E1A13', // bone
    },

    secondary: {
      main: '#9BAA94', // lichen
      dark: '#8C9A85', // lichen hover
      contrastText: '#1E1A13', // bone -- verified 7.07:1 (AAA); bark fails at 1.94:1 here
    },

    background: {
      default: '#1E1A13', // bone
      paper: '#2B261B', // parchment
    },

    text: {
      primary: '#EAE4DA', // bark
      secondary: '#B5ADA2', // soil
    },

    divider: '#45423B', // hypha

    success: {
      // #1792: every dark `main` must clear 4.5:1 as text on all three dark
      // surfaces, including the elevated dialog paper (#393226). moss was
      // #349D62 (4.40:1 on parchment, 3.71:1 on dialogs) -> 5.38 / 4.53.
      main: '#3FAE72', // moss
      dark: '#3AA069', // moss hover
      contrastText: '#1E1A13', // bone -- moss is bright in dark mode, same reasoning as mycelium
    },
    warning: {
      main: '#DDAE6C', // chanterelle
      dark: '#CD9F5D', // chanterelle hover
      contrastText: '#1E1A13', // bone
    },
    error: {
      // #187/#195: russula was #C4675D (oklch 0.62) — as *text* on the dark
      // parchment cards it only cleared 3.90:1, and the filled-chip label
      // (#1E1A13 on it) 4.49:1. Brightened to oklch 0.67 (#D5776B): 4.79:1 as
      // text on dark surfaces, 5.51:1 for the dark label on fills — same
      // "status colors are bright in dark mode" rule as the others.
      // #1792: brightened again (#DE8579) so error text also clears 4.5:1 on
      // the elevated dialog paper (was 4.03:1 there).
      main: '#DE8579', // russula
      dark: '#C9786D', // russula hover
      contrastText: '#1E1A13', // bone
    },
    info: {
      main: '#A898C7', // laccaria (#1792: was #9F8FBE, 4.31:1 on dialog paper)
      dark: '#9889B5', // laccaria hover
      contrastText: '#1E1A13', // bone
    },
  },

  shape: {
    borderRadius: 10,
  },

  typography: {
    fontFamily: `"IBM Plex Sans", "Fira Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Oxygen", "Ubuntu", "Cantarell", "Helvetica Neue", "Droid Sans", sans-serif`,

    body2: {
      color: '#B5ADA2', // soil
    },
    // T63: see lightTheme's matching h5/overline comments -- both createTheme
    // calls are independent, so these need to be set in each block.
    h5: {
      fontFamily: '"EB Garamond", serif',
    },
    overline: {
      fontFamily: '"IBM Plex Mono", monospace',
    },
  },

  components: {
    // #186: see lightTheme's matching MuiButtonBase comment -- both
    // createTheme calls are independent, so this needs to be set in each
    // block, same as the h5/overline duplication below.
    MuiButtonBase: {
      styleOverrides: {
        root: {
          '&.Mui-focusVisible': {
            outline: '2px solid #9EB698', // mycelium (dark) -- 6.88:1 on parchment, 7.92:1 on bone
            outlineOffset: '2px',
          },
        },
      },
    },

    MuiCard: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          border: '1px solid #45423B', // hypha
        },
      },
    },

    MuiAppBar: {
      styleOverrides: {
        root: {
          // Deliberately not primary.main here (mycelium is bright in dark
          // mode -- a full-bleed bright light-green top bar would fight the
          // rest of the dark chrome, and tried-and-checked-in dark parchment
          // instead just blended into the cards below it with no brand
          // presence left). Uses light mode's mycelium value directly instead
          // -- a fixed dark-green brand anchor that doesn't flip between
          // modes, same idea as a fixed-color sidebar/header in apps like
          // Slack. Already verified AAA with white text (8.26:1) since it's
          // the exact value light mode's AppBar already uses.
          backgroundColor: '#3E543E', // mycelium (light-mode value, used as-is here)
          color: '#FFFFFF',
          boxShadow: 'none',
        },
      },
    },

    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
        },
      },
    },

    MuiChip: {
      // T62: this used to be an unconditional `styleOverrides.root` override,
      // which applied to every Chip regardless of `color` -- silently
      // flattening legitimately color="warning"/"error"/"success"/"info"
      // chips (e.g. an overdue reminder's date chip, audit log operation
      // badges) to plain hypha/bark grey in dark mode only. Scoped to
      // `color="default"` via `variants` so only undecorated/category-style
      // chips get the hypha/bark treatment; semantic-colored chips fall
      // through to MUI's normal palette-derived dark-mode colors, matching
      // light mode (which never had this override).
      variants: [
        {
          props: { color: 'default' },
          style: {
            backgroundColor: '#45423B', // hypha
            color: '#EAE4DA', // bark
          },
        },
      ],
    },

    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundColor: '#393226', // paper
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          backgroundColor: '#393226', // paper
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          backgroundColor: '#393226', // paper
        },
      },
    },

    // #194: see lightTheme's matching MuiCssBaseline comment -- both
    // createTheme calls are independent, so this needs to be set in each
    // block, same as the h5/overline/MuiButtonBase duplication above.
    MuiCssBaseline: {
      styleOverrides: `
        @media (prefers-reduced-motion: reduce) {
          *, *::before, *::after {
            animation-duration: 0.01ms !important;
            animation-iteration-count: 1 !important;
            transition-duration: 0.01ms !important;
            scroll-behavior: auto !important;
          }
        }
      `,
    },
  },
});
