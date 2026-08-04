import { useState, useEffect, Suspense, useMemo } from 'react';
import ContactsPage from './ContactsPage';
import ContactDetailPage from './ContactDetailPage';
import PrepViewPage from './PrepViewPage';
import ActivitiesPage from './ActivitiesPage';
import NotesPage from './NotesPage';
import DashboardPage from './DashboardPage';
import SettingsPage from './SettingsPage';
import NetworkPage from './NetworkPage';
import UsersPage from './UsersPage';
import CircleTagTriagePage from './CircleTagTriagePage';
import DataSettingsPage from './DataSettingsPage';
import HouseholdsPage from './HouseholdsPage';
import ContactSharesPage from './ContactSharesPage';
import SearchPage from './SearchPage';
import LoginPage from './LoginPage';
import RegisterPage from './RegisterPage';
import { getToken, logoutAndRedirect, isAdmin, fetchAndCacheUserInfo } from './auth';
import { BrowserRouter as Router, Routes, Route, Link, useLocation, useNavigate, Navigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  AppBar,
  Toolbar,
  IconButton,
  Typography,
  Drawer,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Box,
  Button,
  CircularProgress,
  useTheme,
  useMediaQuery,
  TextField,
  Autocomplete,
  InputAdornment,
  SvgIcon
} from '@mui/material';
import { mdiGraphOutline, mdiNotebookOutline } from '@mdi/js';
import BrandLogo from './components/BrandLogo';
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import { getContacts, Contact } from './api/contacts';
import MenuIcon from '@mui/icons-material/Menu';
import DashboardIcon from '@mui/icons-material/Dashboard';
import ContactsIcon from '@mui/icons-material/Contacts';
import EventNoteIcon from '@mui/icons-material/EventNote';
import SettingsIcon from '@mui/icons-material/Settings';
import PeopleIcon from '@mui/icons-material/People';
import HomeWorkIcon from '@mui/icons-material/HomeWork';
import ShareIcon from '@mui/icons-material/Share';
import StorageIcon from '@mui/icons-material/Storage';
import LogoutIcon from '@mui/icons-material/Logout';
import './App.css';

const drawerWidth = 180;

// Scroll to top on route change
function ScrollToTop() {
  const { pathname } = useLocation();

  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);

  return null;
}

// Inner component that can use useLocation (must be inside Router)
function AppContent({ token, setToken }: { token: string | null; setToken: (token: string | null) => void }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const location = useLocation();
  const navigate = useNavigate();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<Contact[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  
  const handleDrawerToggle = () => {
    setMobileDrawerOpen(!mobileDrawerOpen);
  };

  // Debounced search for contacts
  useEffect(() => {
    if (!token || searchQuery.length < 2) {
      setSearchResults([]);
      return;
    }

    const timer = setTimeout(async () => {
      setSearchLoading(true);
      try {
        const result = await getContacts({ search: searchQuery, limit: 10 });
        setSearchResults(result.contacts || []);
      } catch (err) {
        console.error('Search error:', err);
        setSearchResults([]);
      } finally {
        setSearchLoading(false);
      }
    }, 300);

    return () => clearTimeout(timer);
  }, [searchQuery, token]);

  const handleLogout = async () => {
    await logoutAndRedirect();
  };

  const handleSearchSubmit = () => {
    if (searchQuery.trim()) {
      navigate(`/search?q=${encodeURIComponent(searchQuery.trim())}`);
      setSearchQuery('');
      setSearchResults([]);
    }
  };

  // eslint-disable-next-line react-hooks/exhaustive-deps -- token changes trigger admin status recalculation
  const userIsAdmin = useMemo(() => isAdmin(), [token]);

  const mainNavItems = useMemo(() => {
    const items = [
      { text: t('nav.dashboard'), icon: <DashboardIcon />, path: '/' },
      { text: t('nav.contacts'), icon: <ContactsIcon />, path: '/contacts' },
      { text: t('nav.search'), icon: <SearchIcon />, path: '/search' },
      { text: t('nav.activities'), icon: <EventNoteIcon />, path: '/activities' },
      { text: t('nav.notes'), icon: <SvgIcon><path d={mdiNotebookOutline} /></SvgIcon>, path: '/notes' },
      { text: t('nav.network'), icon: <SvgIcon><path d={mdiGraphOutline} /></SvgIcon>, path: '/network' },
      { text: t('nav.households'), icon: <HomeWorkIcon />, path: '/households' },
      { text: t('nav.shares'), icon: <ShareIcon />, path: '/shares' },
      { text: t('nav.data'), icon: <StorageIcon />, path: '/settings/data' },
      { text: t('nav.profile'), icon: <SettingsIcon />, path: '/settings' },
    ];
    if (userIsAdmin) {
      items.push({ text: t('nav.users'), icon: <PeopleIcon />, path: '/users' });
    }
    return items;
  }, [t, userIsAdmin]);

  const drawerContent = (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Toolbar />
      <List>
        {mainNavItems.map((item) => (
          <ListItem key={item.text} disablePadding>
            <ListItemButton
              component={Link}
              to={item.path}
              onClick={isMobile ? handleDrawerToggle : undefined}
              selected={item.path === '/' ? location.pathname === '/' : location.pathname.startsWith(item.path)}
              sx={{
                '&.Mui-selected': {
                  backgroundColor: 'action.selected',
                },
                '&.Mui-selected:hover': {
                  backgroundColor: 'action.selected',
                },
              }}
            >
              <ListItemIcon>{item.icon}</ListItemIcon>
              <ListItemText primary={item.text} />
            </ListItemButton>
          </ListItem>
        ))}
      </List>
      <Box sx={{ mt: 'auto', py: 2, px: 2 }}>
        <BrandLogo height="auto" width="100%" />
      </Box>
    </Box>
  );

  if (!token) {
    return (
      <Box sx={{ p: 2, width: '100%' }}>
        <Routes>
          <Route path="/register" element={<RegisterPage />} />
          <Route path="*" element={<LoginPage setToken={setToken} />} />
        </Routes>
      </Box>
    );
  }

  return (
    <>
      <AppBar 
        position="fixed" 
        sx={{ 
          zIndex: (theme) => theme.zIndex.drawer + 1
        }}
      >
        <Toolbar>
          {isMobile && (
            <IconButton 
              edge="start" 
              color="inherit" 
              aria-label="menu" 
              onClick={handleDrawerToggle} 
              sx={{ mr: 2 }}
            >
              <MenuIcon />
            </IconButton>
          )}
          <Typography 
            variant="h6" 
            component={Link} 
            to="/" 
            onClick={() => {
              setSearchQuery('');
              setSearchResults([]);
            }}
            sx={{
              flexGrow: 1,
              textDecoration: 'none',
              color: 'inherit',
              fontFamily: '"EB Garamond", serif',
              fontWeight: 600,
              '&:hover': { opacity: 0.8 }
            }}
          >
            {t('app.title')}
          </Typography>
          <Autocomplete
            freeSolo
            size="small"
            options={searchResults}
            getOptionLabel={(option) => 
              typeof option === 'string' 
                ? option 
                : `${option.firstname} ${option.lastname}`
            }
            loading={searchLoading}
            onInputChange={(_, value) => setSearchQuery(value)}
            onChange={(_, value) => {
              if (value && typeof value !== 'string') {
                navigate(`/contacts/${value.ID}`);
                setSearchQuery('');
                setSearchResults([]);
              }
            }}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                handleSearchSubmit();
              }
            }}
            inputValue={searchQuery}
            renderInput={(params) => (
              <TextField
                {...params}
                placeholder={t('contacts.search')}
                variant="outlined"
                sx={{
                  width: { xs: 150, sm: 200, md: 250 },
                  mr: 2,
                  '& .MuiOutlinedInput-root': {
                    backgroundColor: 'rgba(255, 255, 255, 0.15)',
                    '&:hover': {
                      backgroundColor: 'rgba(255, 255, 255, 0.25)',
                    },
                    '& fieldset': {
                      borderColor: 'rgba(255, 255, 255, 0.3)',
                    },
                    '&:hover fieldset': {
                      borderColor: 'rgba(255, 255, 255, 0.5)',
                    },
                    '&.Mui-focused fieldset': {
                      borderColor: 'rgba(255, 255, 255, 0.7)',
                    },
                  },
                  '& .MuiInputBase-input': {
                    color: 'white',
                  },
                  '& .MuiInputBase-input::placeholder': {
                    color: 'rgba(255, 255, 255, 0.7)',
                    opacity: 1,
                  },
                }}
                InputProps={{
                  ...params.InputProps,
                  startAdornment: (
                    <InputAdornment position="start">
                      <IconButton
                        size="small"
                        onClick={handleSearchSubmit}
                        sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.5 }}
                      >
                        <SearchIcon />
                      </IconButton>
                    </InputAdornment>
                  ),
                  endAdornment: searchQuery ? (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        onClick={() => {
                          setSearchQuery('');
                          setSearchResults([]);
                          // Clear search filter if on contacts page
                          if (location.pathname.startsWith('/contacts')) {
                            navigate('/contacts');
                          }
                        }}
                        sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.5 }}
                      >
                        <ClearIcon fontSize="small" />
                      </IconButton>
                    </InputAdornment>
                  ) : null,
                }}
              />
            )}
            renderOption={(props, option) => (
              <li {...props} key={option.ID}>
                <Box>
                  <Typography variant="body1">
                    {option.firstname} {option.lastname}
                  </Typography>
                  {option.email && (
                    <Typography variant="caption" color="text.secondary">
                      {option.email}
                    </Typography>
                  )}
                </Box>
              </li>
            )}
          />
          <Button color="inherit" startIcon={<LogoutIcon />} onClick={handleLogout}>
            {t('app.logout')}
          </Button>
        </Toolbar>
      </AppBar>

      {/* Mobile drawer */}
      <Drawer 
        variant="temporary"
        open={mobileDrawerOpen} 
        onClose={handleDrawerToggle}
        sx={{
          display: { xs: 'block', md: 'none' },
          '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth }
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
          '& .MuiDrawer-paper': { 
            width: drawerWidth, 
            boxSizing: 'border-box' 
          }
        }}
      >
        {drawerContent}
      </Drawer>

      <Box 
        component="main" 
        sx={{ 
          flexGrow: 1, 
          p: 2,
          width: { md: `calc(100% - ${drawerWidth}px)` },
          mt: 7
        }}
      >
        <Routes>
          <Route path="/contacts" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><ContactsPage /></Suspense>} />
          <Route path="/contacts/:id" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><ContactDetailPage /></Suspense>} />
          <Route path="/contacts/:id/prep" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><PrepViewPage /></Suspense>} />
          <Route path="/notes" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><NotesPage /></Suspense>} />
          <Route path="/activities" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><ActivitiesPage /></Suspense>} />
          <Route path="/settings" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><SettingsPage /></Suspense>} />
          <Route path="/settings/data" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><DataSettingsPage /></Suspense>} />
          <Route path="/network" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><NetworkPage /></Suspense>} />
          <Route path="/households" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><HouseholdsPage /></Suspense>} />
          <Route path="/shares" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><ContactSharesPage /></Suspense>} />
          <Route path="/search" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><SearchPage /></Suspense>} />
          <Route path="/users" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><UsersPage /></Suspense>} />
          <Route path="/api-tokens" element={<Navigate to="/settings" replace />} />
          <Route path="/circle-tag-triage" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><CircleTagTriagePage /></Suspense>} />
          <Route path="/reminders" element={<div>{t('pages.reminders')}</div>} />
          <Route path="/" element={<Suspense fallback={<Box display="flex" justifyContent="center" mt={4}><CircularProgress /></Box>}><DashboardPage /></Suspense>} />
          <Route path="/login" element={<Navigate to="/" replace />} />
          <Route path="/register" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Box>
    </>
  );
}

function App() {
  const [token, setToken] = useState(getToken());

  useEffect(() => {
    // Restore session after OIDC redirect: the server sets the auth cookie but
    // localStorage is empty, so we fetch user info once to populate it.
    if (!getToken()) {
      fetchAndCacheUserInfo().then(info => {
        if (info) setToken(getToken());
      });
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

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
    <Router>
      <ScrollToTop />
      <Box sx={{ display: 'flex' }}>
        <AppContent token={token} setToken={setToken} />
      </Box>
    </Router>
  );
}

export default App;
