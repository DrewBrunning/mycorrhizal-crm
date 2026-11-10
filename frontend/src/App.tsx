// #211: the app-shell document-structure model. Every route inherits this;
// re-derive it per page and it will drift, which is exactly how the 17
// routes ended up with 17 different answers the first time. Do not
// re-litigate these six rules per page -- apply them.
//
// 1. Document identity. Every route sets document.title via
//    hooks/useDocumentTitle to "<page name> · Mycorrhizal CRM" (localised).
//    <html lang> always tracks the active i18next language (i18n/config.ts).
//
// 2. Landmark map -- exactly one of each per rendered page:
//      banner      the AppBar (implicit <header>, already correct)
//      navigation  the drawer's <List>, wrapped in <nav aria-label>
//      main        the content Box -- and the logged-out branch's Box too
//
// 3. Heading contract:
//      h1            the page's own title -- exactly one per route
//      h2            a section of that page
//      h3            a subsection inside a section, only where one genuinely
//                     exists
//      not a heading  titles of items in a list or timeline (use
//                     component="p"/"span" at the same variant -- do not
//                     renumber them to h3)
//    Visual size stays decoupled from level via `component`
//    (`<Typography variant="h5" component="h1">`) -- theme.ts documents h5 as
//    page-title-only, so this never collides with MuiDialogTitle (h6).
//
// 4. Location signalling. The active nav item carries aria-current="page"
//    and a visual indicator measuring >=3:1 against the adjacent unselected
//    background (a left border, not a background tint -- the old tint
//    measured 1.19:1).
//
// 5. Bypass. A skip link, first in the tab order, targeting #main-content.
//
// 6. Announcement channel. One polite live region, owned by
//    context/AnnouncerContext, reachable via useAnnouncer() by any component
//    that needs to announce a status change.

import { mdiGraphOutline, mdiNotebookOutline } from '@mdi/js';
import AccountCircleIcon from '@mui/icons-material/AccountCircle';
import CallMadeIcon from '@mui/icons-material/CallMade';
import CallReceivedIcon from '@mui/icons-material/CallReceived';
import CelebrationIcon from '@mui/icons-material/Celebration';
import ContactsIcon from '@mui/icons-material/Contacts';
import DashboardIcon from '@mui/icons-material/Dashboard';
import DeleteIcon from '@mui/icons-material/Delete';
import DomainIcon from '@mui/icons-material/Domain';
import EventNoteIcon from '@mui/icons-material/EventNote';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import GroupIcon from '@mui/icons-material/Group';
import HealthAndSafetyIcon from '@mui/icons-material/HealthAndSafety';
import HistoryIcon from '@mui/icons-material/History';
import HomeWorkIcon from '@mui/icons-material/HomeWork';
import LogoutIcon from '@mui/icons-material/Logout';
import MapIcon from '@mui/icons-material/Map';
import MenuIcon from '@mui/icons-material/Menu';
import MonitorHeartIcon from '@mui/icons-material/MonitorHeart';
import PeopleIcon from '@mui/icons-material/People';
import SettingsIcon from '@mui/icons-material/Settings';
import ShareIcon from '@mui/icons-material/Share';
import StorageIcon from '@mui/icons-material/Storage';
import {
  AppBar,
  Badge,
  Box,
  Button,
  Collapse,
  Divider,
  Drawer,
  IconButton,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  SvgIcon,
  Toolbar,
  Typography,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import { Fragment, lazy, type ReactElement, Suspense, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  createBrowserRouter,
  Link,
  Navigate,
  Route,
  RouterProvider,
  Routes,
  useLocation,
} from 'react-router';
import ActivitiesPage from './ActivitiesPage';
import AuditPage from './AuditPage';
import { getShareGrantNotifications } from './api/shareGrants';
import { fetchAndCacheUserInfo, getToken, isAdmin, logoutAndRedirect } from './auth';
import CirclesTagsPage from './CirclesTagsPage';
import CircleTagTriagePage from './CircleTagTriagePage';
import ContactDetailPage from './ContactDetailPage';
import ContactSharesPage from './ContactSharesPage';
import ContactsPage from './ContactsPage';
import BrandLogo from './components/BrandLogo';
import ErrorBoundary from './components/ErrorBoundary';
import GlobalSearch from './components/GlobalSearch';
import OfflineBanner from './components/OfflineBanner';
import RouteAnnouncer from './components/RouteAnnouncer';
import RouteLoadingFallback from './components/RouteLoadingFallback';
import SessionExpiredNotice from './components/SessionExpiredNotice';
import DashboardPage from './DashboardPage';
import DataSettingsPage from './DataSettingsPage';
import EntitiesPage from './EntitiesPage';
import EntityDetailPage from './EntityDetailPage';
import HouseholdsPage from './HouseholdsPage';
import LoginPage from './LoginPage';
import NetworkPage from './NetworkPage';
import NotesPage from './NotesPage';
import NotFoundPage from './NotFoundPage';
import OccasionsPage from './OccasionsPage';
import PrepViewPage from './PrepViewPage';
import RegisterPage from './RegisterPage';
import SettingsPage from './SettingsPage';
// Imported statically like ContactSharesPage: lazy-loading these two reshuffled the
// shared chunk split (a 273 KiB "errorHandler" chunk tripped the new-chunk gate).
import SharedByMePage from './SharedByMePage';
import SharedWithMePage from './SharedWithMePage';
import SystemEventsPage from './SystemEventsPage';
import SystemStatusPage from './SystemStatusPage';
import UsersPage from './UsersPage';
import './App.css';
import { AuthContext, useAuth } from './context/AuthContext';

// T98: 180 -> 256. The old 180 was tight enough that four of the five locales
// wrapped their longest nav label even at the pre-T98 16px (de/es/fr/it, at
// 99-128px against a 91px text slot), and raising the label to 1.0625rem made
// English wrap too.
//
// A ListItemButton spends 48px of its width on padding plus a 56px icon slot,
// so the usable text slot is drawerWidth - 104. The widest label across all
// five locales at 1.0625rem is Spanish's "Registro de auditoría" at 136.2px,
// which needs 240.2 -- i.e. 240 misses by a sixth of a pixel, and Italian
// clears it by only 1.5px. 256 gives a 152px slot: every locale fits with real
// headroom rather than sitting on the rounding boundary, where a font-rendering
// difference between platforms would flip it back to wrapping.
// The first React.lazy route in the app (ADR 0031): MapLibre GL JS is heavy, so the
// map page (and the `map-vendor` chunk it pulls in) loads only when /map is opened.
const MapPage = lazy(() => import('./MapPage'));
const ShareTargetPage = lazy(() => import('./ShareTargetPage'));
const TrashPage = lazy(() => import('./TrashPage'));
const SearchResultsPage = lazy(() => import('./SearchResultsPage'));

const drawerWidth = 256;

interface NavItem {
  text: string;
  icon: ReactElement;
  path: string;
  /** Sub-items rendered under this one (a collapsible group). */
  children?: NavItem[];
}

// T33 nav classification: which destinations are "primary" (kept as icons in
// the phone AppBar) and which are "account-level" (collapsed into the account
// menu). Everything else is secondary and lives in the hamburger drawer.
const PRIMARY_NAV_PATHS = ['/contacts', '/notes'];
const ACCOUNT_NAV_PATHS = ['/settings', '/settings/data', '/users'];

// Scroll to top on route change
function ScrollToTop() {
  const { pathname } = useLocation();

  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);

  return null;
}

// Inner component that can use useLocation (must be inside Router)
function AppContent({
  token,
  setToken,
}: {
  token: string | null;
  setToken: (token: string | null) => void;
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  const location = useLocation();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  // Below the `sm` breakpoint the AppBar itself is the crowded surface: even
  // title + live search + logout overflow ~360-414px. Swap to icon-only
  // primary destinations + an account menu (T33) once space is tight.
  const compactNav = useMediaQuery(theme.breakpoints.down('sm'));
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
  // Standing shares (#2292): grants awaiting this user's answer, for the nav badge.
  // Re-read on navigation so answering one on the page clears it. A failed read
  // just shows no badge.
  const [pendingShares, setPendingShares] = useState(0);
  const { pathname: currentPath } = location;
  // biome-ignore lint/correctness/useExhaustiveDependencies: currentPath re-reads the count on navigation
  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    getShareGrantNotifications()
      .then((n) => {
        if (!cancelled) setPendingShares(n.pending_count);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [token, currentPath]);
  const [accountMenuAnchor, setAccountMenuAnchor] = useState<HTMLElement | null>(null);
  const accountMenuOpen = Boolean(accountMenuAnchor);

  const handleDrawerToggle = () => {
    setMobileDrawerOpen(!mobileDrawerOpen);
  };

  const handleLogout = async () => {
    await logoutAndRedirect();
  };

  // eslint-disable-next-line react-hooks/exhaustive-deps -- token changes trigger admin status recalculation
  const userIsAdmin = useMemo(() => isAdmin(), [token]);

  // #2175: the drawer is a two-level tree. Daily destinations stay at the top
  // level; Shares, Audit log, Data and the admin pages are sub-items of
  // Settings (their URLs are unchanged). To add a page to the Settings group
  // -- the upcoming Shares (#2292) and Entities (#2301) pages -- push a NavItem
  // onto `settingsChildren` below; a child shows as current when the URL is
  // under its path, and the group highlights and opens with it. Gate
  // admin-only pages behind `userIsAdmin` like Users/System events/System
  // status. Routes live in the <Routes> block; this list only places them.
  const navTree = useMemo<NavItem[]>(() => {
    const settingsChildren: NavItem[] = [
      // Shares: the one-time contact-sharing page, then the standing-share pages
      // (#2292). "Shared with me" carries the pending-share badge.
      { text: t('nav.shares'), icon: <ShareIcon />, path: '/shares' },
      {
        text: t('nav.sharedWithMe'),
        icon:
          pendingShares > 0 ? (
            <Badge
              badgeContent={pendingShares}
              color="primary"
              role="img"
              aria-label={t('nav.sharedPendingBadge', { count: pendingShares })}
            >
              <CallReceivedIcon />
            </Badge>
          ) : (
            <CallReceivedIcon />
          ),
        path: '/shared-with-me',
      },
      { text: t('nav.sharedByMe'), icon: <CallMadeIcon />, path: '/shared-by-me' },
      { text: t('nav.entities'), icon: <DomainIcon />, path: '/entities' },
      { text: t('nav.audit'), icon: <HistoryIcon />, path: '/audit' },
      { text: t('nav.data'), icon: <StorageIcon />, path: '/settings/data' },
    ];
    if (userIsAdmin) {
      settingsChildren.push(
        { text: t('nav.users'), icon: <PeopleIcon />, path: '/users' },
        { text: t('nav.systemEvents'), icon: <MonitorHeartIcon />, path: '/system-events' },
        { text: t('nav.systemStatus'), icon: <HealthAndSafetyIcon />, path: '/system-status' },
      );
    }
    return [
      { text: t('nav.dashboard'), icon: <DashboardIcon />, path: '/' },
      { text: t('nav.contacts'), icon: <ContactsIcon />, path: '/contacts' },
      { text: t('nav.activities'), icon: <EventNoteIcon />, path: '/activities' },
      {
        text: t('nav.notes'),
        icon: (
          <SvgIcon>
            <path d={mdiNotebookOutline} />
          </SvgIcon>
        ),
        path: '/notes',
      },
      {
        text: t('nav.network'),
        icon: (
          <SvgIcon>
            <path d={mdiGraphOutline} />
          </SvgIcon>
        ),
        path: '/network',
      },
      { text: t('nav.map'), icon: <MapIcon />, path: '/map' },
      { text: t('nav.occasions'), icon: <CelebrationIcon />, path: '/occasions' },
      { text: t('nav.households'), icon: <HomeWorkIcon />, path: '/households' },
      { text: t('nav.circlesTags'), icon: <GroupIcon />, path: '/circles' },
      { text: t('nav.trash'), icon: <DeleteIcon />, path: '/trash' },
      {
        text: t('nav.profile'),
        icon: <SettingsIcon />,
        path: '/settings',
        children: settingsChildren,
      },
    ];
  }, [t, userIsAdmin, pendingShares]);

  // Flat view of the tree (parents then their children) for path matching and
  // the phone AppBar / account menu, which do not nest.
  const mainNavItems = useMemo(
    () => navTree.flatMap((item) => [item, ...(item.children ?? [])]),
    [navTree],
  );

  // T33 information-architecture pass: the nav bar had grown to ten
  // destinations. Each is classified so the mobile AppBar knows where it
  // lives:
  //   primary — used constantly, kept as visible icons in the AppBar below sm
  //              (Contacts, Notes)
  //   account — Settings, Data settings, User Management; collapsed into the
  //              account menu instead of the main nav row
  //   everything else is secondary and lives in the hamburger drawer.
  const primaryNavItems = useMemo(
    () => mainNavItems.filter((item) => PRIMARY_NAV_PATHS.includes(item.path)),
    [mainNavItems],
  );
  const accountNavItems = useMemo(
    () => mainNavItems.filter((item) => ACCOUNT_NAV_PATHS.includes(item.path)),
    [mainNavItems],
  );

  // #2154: select exactly one destination -- the longest nav path matching on
  // a path-segment boundary -- so /settings/data does not also light up
  // Settings (/settings), and /contacts-archive does not match /contacts.
  const currentNavPath = useMemo(() => {
    let best: string | null = null;
    for (const { path } of mainNavItems) {
      const matches =
        path === '/'
          ? location.pathname === '/'
          : location.pathname === path || location.pathname.startsWith(`${path}/`);
      if (matches && (best === null || path.length > best.length)) best = path;
    }
    return best;
  }, [mainNavItems, location.pathname]);

  // #2175: a group (Settings) is open when the current page is inside it,
  // unless the user toggled it on this very page. The override is keyed to the
  // pathname so navigating re-derives the default (open on a child route).
  const [groupOverride, setGroupOverride] = useState<{ at: string; open: boolean } | null>(null);

  const navLinkSx = {
    // #211: a left border, not a background tint -- the tint measured 1.19:1,
    // this measures 6.73:1 light / 6.88:1 dark (see the model comment block at
    // the top of this file).
    borderLeft: '3px solid transparent',
    '&.Mui-selected': {
      backgroundColor: 'action.selected',
      borderLeftColor: 'primary.main',
    },
    '&.Mui-selected:hover': {
      backgroundColor: 'action.selected',
    },
  };
  // T98: Garamond has a smaller x-height than IBM Plex Sans, so ListItemText's
  // 1rem body1 default reads shrunken here rather than merely different. T63
  // set the family and no size.
  const navLabelSlotProps = {
    primary: { sx: { fontFamily: '"EB Garamond", serif', fontSize: '1.0625rem' } },
  };

  const renderNavItem = (item: NavItem, nested = false): ReactElement => {
    const children = item.children;
    const isCurrent = item.path === currentNavPath;
    const childCurrent = !!children?.some((c) => c.path === currentNavPath);
    // The group parent stays highlighted while one of its children is the
    // current page; only the item the URL actually matches carries
    // aria-current="page" (the parent says "true": the current page is inside).
    const isSelected = isCurrent || childCurrent;
    const open = groupOverride?.at === location.pathname ? groupOverride.open : childCurrent;
    const groupId = `nav-group-${item.path.replace(/\W+/g, '-')}`;
    return (
      <Fragment key={item.path}>
        <ListItem
          disablePadding
          secondaryAction={
            children ? (
              <IconButton
                edge="end"
                size="small"
                aria-label={t('nav.settingsSubpages')}
                aria-expanded={open}
                aria-controls={groupId}
                onClick={() => setGroupOverride({ at: location.pathname, open: !open })}
              >
                {open ? <ExpandLessIcon /> : <ExpandMoreIcon />}
              </IconButton>
            ) : undefined
          }
        >
          <ListItemButton
            component={Link}
            to={item.path}
            onClick={isMobile ? handleDrawerToggle : undefined}
            aria-current={isCurrent ? 'page' : childCurrent ? 'true' : undefined}
            selected={isSelected}
            sx={{ ...navLinkSx, ...(nested ? { pl: 3 } : {}) }}
          >
            <ListItemIcon>{item.icon}</ListItemIcon>
            <ListItemText primary={item.text} slotProps={navLabelSlotProps} />
          </ListItemButton>
        </ListItem>
        {children ? (
          <Collapse component="li" in={open} timeout="auto" id={groupId}>
            <List disablePadding>{children.map((c) => renderNavItem(c, true))}</List>
          </Collapse>
        ) : null}
      </Fragment>
    );
  };

  const drawerContent = (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Toolbar />
      {/* #1792: the brand logo lives inside the navigation landmark (it used
          to sit after </nav>, so axe's `region` rule flagged it as content
          outside every landmark). The nav fills the drawer so the logo still
          pins to the bottom. */}
      <Box
        component="nav"
        aria-label={t('nav.mainLabel')}
        sx={{ display: 'flex', flexDirection: 'column', flexGrow: 1 }}
      >
        <List>{navTree.map((item) => renderNavItem(item))}</List>
        <Box sx={{ mt: 'auto', py: 2, px: 2 }}>
          <BrandLogo height="auto" width="100%" />
        </Box>
      </Box>
    </Box>
  );

  if (!token) {
    return (
      <Box component="main" id="main-content" tabIndex={-1} sx={{ p: 2, width: '100%' }}>
        <OfflineBanner />
        <RouteAnnouncer />
        <Routes>
          <Route path="/register" element={<RegisterPage setToken={setToken} />} />
          <Route path="*" element={<LoginPage setToken={setToken} />} />
        </Routes>
      </Box>
    );
  }

  return (
    <>
      <Box
        component="a"
        href="#main-content"
        sx={{
          position: 'absolute',
          left: -9999,
          top: 0,
          zIndex: (theme) => theme.zIndex.drawer + 2,
          p: 2,
          bgcolor: 'background.paper',
          color: 'text.primary',
          '&:focus': { left: 0 },
        }}
      >
        {t('nav.skipToContent')}
      </Box>
      <AppBar
        position="fixed"
        sx={{
          zIndex: (theme) => theme.zIndex.drawer + 1,
        }}
      >
        <Toolbar>
          {isMobile && (
            <IconButton
              edge="start"
              color="inherit"
              aria-label="menu"
              onClick={handleDrawerToggle}
              sx={{ mr: 0.5 }}
            >
              <MenuIcon />
            </IconButton>
          )}
          <Typography
            variant="h6"
            component={Link}
            to="/"
            onClick={() => {}}
            sx={{
              flexGrow: 1,
              display: { xs: 'none', sm: 'block' },
              textDecoration: 'none',
              color: 'inherit',
              fontFamily: '"EB Garamond", serif',
              fontWeight: 600,
              '&:hover': { opacity: 0.8 },
            }}
          >
            {t('app.title')}
          </Typography>

          {compactNav ? (
            // Phone-width AppBar (T33): primary destinations stay directly
            // visible as icon-only buttons (each carries an aria-label — no
            // accessible name is dropped when the label is), secondary items
            // live in the hamburger drawer, and account-level items collapse
            // into the account menu.
            <>
              {primaryNavItems.map((item) => (
                <IconButton
                  key={item.path}
                  component={Link}
                  to={item.path}
                  color="inherit"
                  aria-label={item.text}
                  sx={{ mr: 0.5 }}
                >
                  {item.icon}
                </IconButton>
              ))}
              <Box sx={{ flexGrow: 1 }} />
              <IconButton
                color="inherit"
                aria-label={t('app.accountMenu')}
                onClick={(e) => setAccountMenuAnchor(e.currentTarget)}
              >
                <AccountCircleIcon />
              </IconButton>
              <Menu
                anchorEl={accountMenuAnchor}
                open={accountMenuOpen}
                onClose={() => setAccountMenuAnchor(null)}
              >
                {accountNavItems.map((item) => (
                  <MenuItem
                    key={item.path}
                    component={Link}
                    to={item.path}
                    onClick={() => setAccountMenuAnchor(null)}
                  >
                    <ListItemIcon>{item.icon}</ListItemIcon>
                    <ListItemText>{item.text}</ListItemText>
                  </MenuItem>
                ))}
                <Divider sx={{ my: 0.5 }} />
                <MenuItem onClick={() => void handleLogout()}>
                  <ListItemIcon>
                    <LogoutIcon />
                  </ListItemIcon>
                  <ListItemText>{t('app.logout')}</ListItemText>
                </MenuItem>
              </Menu>
            </>
          ) : (
            <>
              <GlobalSearch />
              <Button
                color="inherit"
                startIcon={<LogoutIcon />}
                onClick={() => void handleLogout()}
              >
                {t('app.logout')}
              </Button>
            </>
          )}
        </Toolbar>
      </AppBar>

      {/* Mobile drawer */}
      <Drawer
        variant="temporary"
        open={mobileDrawerOpen}
        onClose={handleDrawerToggle}
        sx={{
          display: { xs: 'block', md: 'none' },
          '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
        }}
      >
        {drawerContent}
      </Drawer>

      {/* Desktop drawer */}
      <Drawer
        variant="permanent"
        sx={{
          display: { xs: 'none', md: 'block' },
          width: drawerWidth,
          flexShrink: 0,
          // #1788: the paper below is `position: fixed` (viewport tall), so on a
          // page taller than the viewport -- or in a full-page capture -- the
          // sidebar surface used to stop after one screen and leave a visible
          // background step. This in-flow column stretches the full height of
          // the app, so paint the same surface (and the divider edge) on it too.
          backgroundColor: 'background.paper',
          boxShadow: (theme) => `inset -1px 0 0 ${theme.palette.divider}`,
          '& .MuiDrawer-paper': {
            width: drawerWidth,
            boxSizing: 'border-box',
          },
        }}
      >
        {drawerContent}
      </Drawer>

      <Box
        component="main"
        id="main-content"
        tabIndex={-1}
        sx={{
          flexGrow: 1,
          p: 2,
          // `min-width: auto` (the flex default) stops this item shrinking below
          // its content's intrinsic width, so a wide child (the contact
          // timeline, the network controls, the settings tables) would force
          // the page to scroll horizontally at phone widths instead of the
          // child scrolling/containing itself. minWidth: 0 lets the page
          // column take the viewport and the children handle their own
          // overflow (T32/T33).
          minWidth: 0,
          width: { md: `calc(100% - ${drawerWidth}px)` },
          mt: 7,
        }}
      >
        {/* Issue #557: scoped to the routed page, not the whole app shell,
            so a crash here leaves the AppBar/drawer/search (and anything
            <SessionExpiredGate> is showing) intact -- "Try Again" retries
            just this subtree instead of forcing a full reload. Keyed by
            pathname so navigating to a different route recovers on its own;
            an ErrorBoundary's caught-error state does not otherwise clear
            just because its children changed. */}
        <SessionExpiredNotice />
        <OfflineBanner />
        <RouteAnnouncer />
        <ErrorBoundary key={location.pathname} name="Page">
          <Routes>
            <Route
              path="/contacts"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ContactsPage />
                </Suspense>
              }
            />
            <Route
              path="/contacts/:id"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ContactDetailPage />
                </Suspense>
              }
            />
            <Route
              path="/contacts/:id/prep"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <PrepViewPage />
                </Suspense>
              }
            />
            <Route
              path="/notes"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <NotesPage />
                </Suspense>
              }
            />
            <Route
              path="/activities"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ActivitiesPage />
                </Suspense>
              }
            />
            <Route
              path="/occasions"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <OccasionsPage />
                </Suspense>
              }
            />
            <Route
              path="/settings"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <SettingsPage />
                </Suspense>
              }
            />
            <Route
              path="/settings/data"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <DataSettingsPage />
                </Suspense>
              }
            />
            <Route
              path="/network"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <NetworkPage />
                </Suspense>
              }
            />
            <Route
              path="/map"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <MapPage />
                </Suspense>
              }
            />
            <Route
              path="/households"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <HouseholdsPage />
                </Suspense>
              }
            />
            <Route
              path="/circles"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <CirclesTagsPage />
                </Suspense>
              }
            />
            <Route path="/tags" element={<Navigate to="/circles?tab=tags" replace />} />
            <Route
              path="/shares"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ContactSharesPage />
                </Suspense>
              }
            />
            <Route
              path="/shared-with-me"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <SharedWithMePage />
                </Suspense>
              }
            />
            <Route
              path="/shared-by-me"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <SharedByMePage />
                </Suspense>
              }
            />
            <Route
              path="/trash"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <TrashPage />
                </Suspense>
              }
            />
            <Route
              path="/search"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <SearchResultsPage />
                </Suspense>
              }
            />
            <Route
              path="/share"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <ShareTargetPage />
                </Suspense>
              }
            />
            <Route
              path="/entities"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <EntitiesPage />
                </Suspense>
              }
            />
            <Route
              path="/entities/:id"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <EntityDetailPage />
                </Suspense>
              }
            />
            <Route
              path="/audit"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <AuditPage />
                </Suspense>
              }
            />
            <Route
              path="/users"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <UsersPage />
                </Suspense>
              }
            />
            <Route
              path="/system-events"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <SystemEventsPage />
                </Suspense>
              }
            />
            <Route
              path="/system-status"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <SystemStatusPage />
                </Suspense>
              }
            />
            <Route path="/api-tokens" element={<Navigate to="/settings" replace />} />
            <Route
              path="/circle-tag-triage"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <CircleTagTriagePage />
                </Suspense>
              }
            />
            <Route
              path="/"
              element={
                <Suspense fallback={<RouteLoadingFallback />}>
                  <DashboardPage />
                </Suspense>
              }
            />
            {/* Issue #1793: the logged-in shell used to redirect /login away
                even when the cached user_info outlived the session cookie,
                making an explicit visit a dead end. Let the form render so a
                stale-cache visitor can sign in again. */}
            <Route path="/login" element={<LoginPage setToken={setToken} />} />
            <Route path="/register" element={<Navigate to="/" replace />} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </ErrorBoundary>
      </Box>
    </>
  );
}

// Issue #805: react-router's useBlocker (the discard guards' in-app route
// navigation protection) only works under a *data* router -- the plain
// <BrowserRouter>+<Routes> this file used never installed the DataRouterContext
// useBlocker reads. The route tree is therefore moved onto a data router,
// created once at module scope (RouterProvider refuses to own a router that is
// recreated on render). AppContent still renders the declarative <Routes>
// tree inside it, so the ~800-line logged-in/logged-out shell and its two
// per-auth route branches move over without a behavioural rewrite -- the
// top-level match is deliberately a single catch-all. What changes is the
// context RouterProvider now installs: useDiscardGuard's navigation blocker
// is live, so a dirty dialog intercepts a drawer link, a programmatic
// navigate, or a browser Back/Forward instead of silently unmounting.
function AppRoot() {
  const { token, setToken } = useAuth();
  return (
    <>
      <ScrollToTop />
      <Box sx={{ display: 'flex' }}>
        <AppContent token={token} setToken={setToken} />
      </Box>
    </>
  );
}

const router = createBrowserRouter([{ path: '*', element: <AppRoot /> }]);

function App() {
  const [token, setToken] = useState(getToken());

  useEffect(() => {
    // Restore session after OIDC redirect: the server sets the auth cookie but
    // localStorage is empty, so we fetch user info once to populate it.
    if (!getToken()) {
      // fetchAndCacheUserInfo catches its own errors and resolves to null.
      void fetchAndCacheUserInfo().then((info) => {
        if (info) setToken(getToken());
      });
    }
  }, []);

  useEffect(() => {
    // Listen for storage changes (e.g., logout in another tab)
    const onStorage = (e: StorageEvent) => {
      if (e.key === 'user_info') {
        setToken(getToken());
      }
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);

  return (
    <AuthContext.Provider value={{ token, setToken }}>
      <RouterProvider router={router} />
    </AuthContext.Provider>
  );
}

export default App;
