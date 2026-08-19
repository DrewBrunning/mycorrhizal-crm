// theme.ts
//
// Palette values come from assets/colors/tokens.json (OKLCH-derived; see
// assets/colors/README.md for the conversion math and the hover/active/
// contrastText derivation rules). `light`/`dark` on primary/secondary are
// deliberately omitted where we don't have a bespoke tint -- MUI's own
// augmentColor() fills them in from `main`.
import { createTheme } from "@mui/material/styles";

export const lightTheme = createTheme({
  palette: {
    mode: "light",

    primary: {
      main: "#3E543E", // mycelium
      dark: "#314631", // mycelium hover (also used by MUI for contained-button hover/emphasis)
      contrastText: "#FFFFFF",
    },

    secondary: {
      main: "#97A390", // lichen
      dark: "#879481", // lichen hover
      contrastText: "#30271F", // bark -- verified 5.54:1 (AA); white fails at 2.64:1
    },

    background: {
      default: "#FAF5EA", // bone
      paper: "#EFE7D9", // parchment -- cards/sidebars. Dialogs/menus/popovers use the
      // lighter "paper" token (#FFFFFF) instead, see MuiDialog/MuiPopover/MuiMenu below.
    },

    text: {
      primary: "#30271F", // bark
      secondary: "#595148", // soil
    },

    divider: "#CBC7BF", // hypha

    // Mushroom-themed status colors -- see assets/colors/README.md for the
    // two corrections made before wiring these in (moss/success was too
    // close in hue to mycelium/lichen; laccaria/info's light value didn't
    // clear 4.5:1 with any label color).
    success: {
      main: "#0D844C", // moss
      dark: "#0B7643", // moss hover
      contrastText: "#FFFFFF",
    },
    warning: {
      main: "#D3A563", // chanterelle
      dark: "#C39553", // chanterelle hover
      contrastText: "#30271F", // bark -- chanterelle is light in both modes, needs a dark label in both
    },
    error: {
      // #187/#195: russula was #AD5349 (oklch 0.55) — as *text* on the
      // parchment cards it only cleared 4.16:1. Darkened to oklch 0.52
      // (#A34A40): 4.74:1 as text on parchment, white labels on fills 5.82:1.
      main: "#A34A40", // russula
      dark: "#923B33", // russula hover
      contrastText: "#FFFFFF",
    },
    info: {
      main: "#7B6B98", // laccaria (corrected from the original oklch(0.60 0.070 300))
      dark: "#6C5D89", // laccaria hover
      contrastText: "#FFFFFF",
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
      color: "#595148", // soil
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
          "&.Mui-focusVisible": {
            outline: "2px solid #3E543E", // mycelium -- 6.73:1 on parchment, 7.60:1 on bone
            outlineOffset: "2px",
          },
        },
      },
    },

    MuiCard: {
      styleOverrides: {
        root: {
          boxShadow:
            "0px 1px 2px rgba(48, 39, 31, 0.06), 0px 2px 8px rgba(48, 39, 31, 0.04)",
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
          boxShadow: "none",
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
          backgroundColor: "#FFFFFF", // paper
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          backgroundColor: "#FFFFFF", // paper
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          backgroundColor: "#FFFFFF", // paper
        },
      },
    },
  },
});

export const darkTheme = createTheme({
  palette: {
    mode: "dark",

    primary: {
      main: "#9EB698", // mycelium (dark mode: bright, not dark -- see contrastText note)
      dark: "#8EA789", // mycelium hover
      // mycelium is bright in dark mode (L 0.75), so a light label fails
      // contrast (1.73:1) -- verified a dark label works instead (7.92:1, AAA).
      contrastText: "#1E1A13", // bone
    },

    secondary: {
      main: "#9BAA94", // lichen
      dark: "#8C9A85", // lichen hover
      contrastText: "#1E1A13", // bone -- verified 7.07:1 (AAA); bark fails at 1.94:1 here
    },

    background: {
      default: "#1E1A13", // bone
      paper: "#2B261B", // parchment
    },

    text: {
      primary: "#EAE4DA", // bark
      secondary: "#B5ADA2", // soil
    },

    divider: "#45423B", // hypha

    success: {
      main: "#349D62", // moss
      dark: "#318F5A", // moss hover
      contrastText: "#1E1A13", // bone -- moss is bright in dark mode, same reasoning as mycelium
    },
    warning: {
      main: "#DDAE6C", // chanterelle
      dark: "#CD9F5D", // chanterelle hover
      contrastText: "#1E1A13", // bone
    },
    error: {
      // #187/#195: russula was #C4675D (oklch 0.62) — as *text* on the dark
      // parchment cards it only cleared 3.90:1, and the filled-chip label
      // (#1E1A13 on it) 4.49:1. Brightened to oklch 0.67 (#D5776B): 4.79:1 as
      // text on dark surfaces, 5.51:1 for the dark label on fills — same
      // "status colors are bright in dark mode" rule as the others.
      main: "#D5776B", // russula
      dark: "#C4675D", // russula hover
      contrastText: "#1E1A13", // bone
    },
    info: {
      main: "#9F8FBE", // laccaria
      dark: "#8F80AE", // laccaria hover
      contrastText: "#1E1A13", // bone
    },
  },

  shape: {
    borderRadius: 10,
  },

  typography: {
    fontFamily: `"IBM Plex Sans", "Fira Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Oxygen", "Ubuntu", "Cantarell", "Helvetica Neue", "Droid Sans", sans-serif`,

    body2: {
      color: "#B5ADA2", // soil
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
          "&.Mui-focusVisible": {
            outline: "2px solid #9EB698", // mycelium (dark) -- 6.88:1 on parchment, 7.92:1 on bone
            outlineOffset: "2px",
          },
        },
      },
    },

    MuiCard: {
      styleOverrides: {
        root: {
          backgroundImage: "none",
          border: "1px solid #45423B", // hypha
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
          backgroundColor: "#3E543E", // mycelium (light-mode value, used as-is here)
          color: "#FFFFFF",
          boxShadow: "none",
        },
      },
    },

    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: "none",
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
          props: { color: "default" },
          style: {
            backgroundColor: "#45423B", // hypha
            color: "#EAE4DA", // bark
          },
        },
      ],
    },

    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundColor: "#393226", // paper
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          backgroundColor: "#393226", // paper
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          backgroundColor: "#393226", // paper
        },
      },
    },
  },
});
