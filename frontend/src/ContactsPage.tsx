import { useEffect, useState, useMemo, useCallback, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useContacts } from './hooks/useContacts';
import { useCircles } from './hooks/useCircles';
import { useTags } from './hooks/useTags';
import { useSearch } from './hooks/useSearch';
import { getCurrentUser } from './api/admin';
import { resolveEnabledFields, ContactFieldKey } from './contactFields';
import { BulkAction, runBulkOperation } from './api/bulkOperations';
import AddContactDialog from './components/AddContactDialog';
import ImportContactsDialog from './components/ImportContactsDialog';
import BulkActionsBar from './components/BulkActionsBar';
import SearchNotesActivities from './components/SearchNotesActivities';
import {
  Box,
  Card,
  Avatar,
  Typography,
  Chip,
  Stack,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Button,
  FormControlLabel,
  Switch,
  Checkbox,
  TextField,
  InputAdornment,
  IconButton
} from '@mui/material';
import PersonAddIcon from '@mui/icons-material/PersonAdd';
import FileUploadIcon from '@mui/icons-material/FileUpload';
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import { ContactListSkeleton } from './components/LoadingSkeletons';

export default function ContactsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  // The committed (debounced) search term — the URL's `?search=` param is the
  // source of truth and drives the list query; the text field below owns it.
  const searchQuery = searchParams.get('search') || '';
  const [selectedCircle, setSelectedCircle] = useState('');
  const { circles, circleNamesByUid, refresh: refreshCircles } = useCircles();
  const { tags, refresh: refreshTags } = useTags();
  const { result: searchResult, runSearch } = useSearch();

  // The search field is a draft: as-you-type input, debounced into the URL
  // (300ms, two-character minimum — matching the backend's own two-rune gate).
  // `searchInput` mirrors the field; `searchQuery` (above) is the committed
  // value the list actually queries. `ownWriteRef` tracks the value this
  // component itself last wrote, so the resync effect can tell "the URL
  // changed because we wrote it" (ignore) apart from "something else
  // navigated here" (seed the field) — the exact bug SearchPage's comment
  // documented; ported rather than rediscovered.
  const [searchInput, setSearchInput] = useState(searchQuery);
  const ownWriteRef = useRef(searchQuery);

  useEffect(() => {
    if (searchQuery !== ownWriteRef.current) {
      setSearchInput(searchQuery);
    }
  }, [searchQuery]);

  useEffect(() => {
    const timer = setTimeout(() => {
      const next = searchInput.trim().length >= 2 ? searchInput.trim() : '';
      ownWriteRef.current = next;
      if (next === searchQuery) return;
      setSearchParams((prev) => {
        const nextParams = new URLSearchParams(prev);
        if (next) nextParams.set('search', next);
        else nextParams.delete('search');
        return nextParams;
      }, { replace: true });
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput, searchQuery, setSearchParams]);

  // Cross-entity search (notes/activities) fires in parallel with the list and
  // is only ever additive — the contact cards never wait on it (trap #3).
  useEffect(() => {
    runSearch(searchQuery);
  }, [searchQuery, runSearch]);

  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [enabledFields, setEnabledFields] = useState<Set<ContactFieldKey>>(() => resolveEnabledFields(null));
  const [showArchived, setShowArchived] = useState(false);
  const pageSize = 10;

  // N5 multi-select: keyed by Contact.VCardUID so it survives pagination —
  // "load more" appends to the list but the selection Set is untouched.
  const [selectedUids, setSelectedUids] = useState<Set<string>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);

  // Selection is meaningless once the visible set changes underneath it — a
  // filter/search/archived change swaps `contacts` for an unrelated page,
  // so a stale selection would let a bulk action (including delete) run
  // against contacts the user can no longer see. Clear it here rather than
  // in useContacts, which has no concept of selection. Sort is deliberately
  // NOT a dependency (T77): changing the order changes which page a contact
  // is on, not which contacts are selected — clearing on a sort would throw
  // away a valid selection for no reason.
  useEffect(() => {
    setSelectedUids(new Set());
  }, [searchQuery, selectedCircle, showArchived]);

  // T77 sort control: the URL's ?sort= and ?order= params own the list order
  // (same persistence mechanism as T86's ?search=). The web client opts into
  // name (alphabetical) as its default; the server keeps updated_at for
  // existing API consumers. The Select below encodes both dimensions in one
  // "sort:order" value.
  const sort = searchParams.get('sort') === 'updated_at' ? 'updated_at' : 'name';
  // Direction default follows the sort key's natural expectation: name opens
  // A→Z, recently-edited opens newest-first (matching the server's desc
  // default) — only relevant for a hand-crafted URL, since the Select below
  // always writes both params together.
  const orderParam = searchParams.get('order');
  const order = orderParam === 'desc' || orderParam === 'asc'
    ? orderParam
    : (sort === 'name' ? 'asc' : 'desc');

  const setSortSelection = (value: string) => {
    const [nextSort, nextOrder] = value.split(':');
    setSearchParams((prev) => {
      const nextParams = new URLSearchParams(prev);
      nextParams.set('sort', nextSort);
      nextParams.set('order', nextOrder);
      return nextParams;
    }, { replace: true });
  };

  // T17/T73: cursor pagination — the list pages by the chosen sort's cursor
  // key ((updated_at, id) DESC by default, or (sort_name, id) under
  // ?sort=name) and the "load more" button appends the next_cursor page.
  // There is no page number or exact total anymore.
  const contactParams = useMemo(() => ({
    limit: pageSize,
    search: searchQuery,
    circle: selectedCircle,
    sort: sort as 'updated_at' | 'name',
    order: order as 'asc' | 'desc',
    includeArchived: showArchived,
  }), [searchQuery, selectedCircle, showArchived, sort, order]);

  // Use custom hook for fetching contacts
  const { contacts, nextCursor, loading, refetch, loadMore } = useContacts(contactParams);

  // Derived flags for the merged search surfaces (T86).
  const searchActive = searchQuery.trim().length >= 2;
  // Discard a stale /search result: useSearch keeps the previous query's
  // result until the new one lands, so without this the notes/activities
  // section would render the old query's hits against the new query's cards.
  const crossResultMatches = searchResult !== null && searchResult.query === searchQuery;
  const crossEmpty = crossResultMatches
    && (searchResult.notes || []).length === 0
    && (searchResult.activities || []).length === 0;
  const showNoResults = searchActive && !loading && contacts.length === 0 && crossEmpty;

  // Fetch enabled contact fields
  useEffect(() => {
    const fetchData = async () => {
      try {
        const user = await getCurrentUser();
        setEnabledFields(resolveEnabledFields(user.enabled_contact_fields));
      } catch (err) {
        console.error('Error fetching enabled contact fields:', err);
      }
    };
    fetchData();
  }, []);

  // Clear the circle filter chip → list refetches automatically via contactParams.
  const clearCircle = useCallback(() => setSelectedCircle(''), []);

  const handleContactAdded = (contactId: number) => {
    navigate(`/contacts/${contactId}`);
  };

  const handleImportComplete = async () => {
    await refetch();
    await refreshCircles();
  };

  // --- N5 selection --------------------------------------------------------

  const isSelected = useCallback((uid: string | undefined) => !!uid && selectedUids.has(uid), [selectedUids]);
  const allSelected = contacts.length > 0 && contacts.every((c) => isSelected(c.uid));

  const toggleSelect = (uid: string | undefined) => {
    if (!uid) return;
    setSelectedUids((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid);
      else next.add(uid);
      return next;
    });
  };

  const toggleSelectAll = () => {
    const loaded = contacts.map((c) => c.uid).filter(Boolean) as string[];
    if (allSelected) {
      setSelectedUids(new Set());
    } else {
      setSelectedUids((prev) => new Set([...prev, ...loaded]));
    }
  };

  const clearSelection = () => setSelectedUids(new Set());

  // --- N5 bulk actions -----------------------------------------------------

  const handleBulk = async (action: BulkAction, circleId?: string, tagId?: string) => {
    if (selectedUids.size === 0) return;
    setBulkBusy(true);
    try {
      const result = await runBulkOperation({
        action,
        vcard_uids: Array.from(selectedUids),
        circle_id: circleId,
        tag_id: tagId,
      });
      if (result.failed > 0) {
        // Partial success: surface the counts so nothing failed silently.
        window.alert(t('bulk.resultFailure', { total: result.total, succeeded: result.succeeded, failed: result.failed }));
      }
      setSelectedUids(new Set());
      await Promise.all([refetch(), refreshCircles(), refreshTags()]);
    } catch {
      window.alert(t('bulk.resultError'));
    } finally {
      setBulkBusy(false);
    }
  };

  const handleBulkDelete = () => {
    if (selectedUids.size === 0) return Promise.resolve();
    // Bulk delete is the most destructive action in the app — require a real
    // confirmation naming the count before anything happens.
    if (!window.confirm(t('bulk.deleteConfirm', { count: selectedUids.size }))) return Promise.resolve();
    return handleBulk('delete');
  };

  const handleAddCircle = (circleId: string) => handleBulk('add_circle', circleId);
  const handleRemoveCircle = (circleId: string) => handleBulk('remove_circle', circleId);
  const handleAddTag = (tagId: string) => handleBulk('add_tag', undefined, tagId);
  const handleRemoveTag = (tagId: string) => handleBulk('remove_tag', undefined, tagId);
  const handleArchive = () => handleBulk('archive');
  const handleUnarchive = () => handleBulk('unarchive');

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" gutterBottom sx={{ mb: 2 }}>
        {t('contacts.title')}
      </Typography>
      <TextField
        label={t('contacts.searchPlaceholder')}
        value={searchInput}
        onChange={(e) => setSearchInput(e.target.value)}
        fullWidth
        size="small"
        sx={{ mb: 2 }}
        helperText={searchInput.trim().length === 1 ? t('contacts.searchMinLengthHint') : undefined}
        slotProps={{
          input: {
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon />
              </InputAdornment>
            ),
            endAdornment: searchInput ? (
              <InputAdornment position="end">
                <IconButton size="small" onClick={() => setSearchInput('')} aria-label={t('contacts.searchClear')}>
                  <ClearIcon fontSize="small" />
                </IconButton>
              </InputAdornment>
            ) : undefined,
          },
        }}
      />
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems="center" flexWrap="wrap">
        <FormControl sx={{ minWidth: 180 }} size="small">
          <InputLabel id="circle-select-label">{t('contacts.filterByCircle')}</InputLabel>
          <Select
            labelId="circle-select-label"
            value={selectedCircle}
            label={t('contacts.filterByCircle')}
            onChange={e => setSelectedCircle(e.target.value)}
          >
            <MenuItem value="">{t('contacts.allCircles')}</MenuItem>
            {circles.map(c => (
              <MenuItem key={c.id} value={c.name}>{c.name}</MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl sx={{ minWidth: 220 }} size="small">
          <InputLabel id="sort-select-label">{t('contacts.sortBy')}</InputLabel>
          <Select
            labelId="sort-select-label"
            value={`${sort}:${order}`}
            label={t('contacts.sortBy')}
            onChange={e => setSortSelection(e.target.value)}
          >
            <MenuItem value="name:asc">{t('contacts.sortNameAsc')}</MenuItem>
            <MenuItem value="name:desc">{t('contacts.sortNameDesc')}</MenuItem>
            <MenuItem value="updated_at:desc">{t('contacts.sortUpdatedDesc')}</MenuItem>
            <MenuItem value="updated_at:asc">{t('contacts.sortUpdatedAsc')}</MenuItem>
          </Select>
        </FormControl>
        <FormControlLabel
          control={
            <Switch
              checked={showArchived}
              onChange={(e) => setShowArchived(e.target.checked)}
              size="small"
            />
          }
          label={t('contacts.showArchived')}
          sx={{ ml: 0.5, whiteSpace: 'nowrap' }}
        />
        <Button
          variant="outlined"
          startIcon={<FileUploadIcon />}
          onClick={() => setImportDialogOpen(true)}
          sx={{ whiteSpace: 'nowrap' }}
        >
          {t('contacts.import.button', 'Import')}
        </Button>
        <Button
          variant="contained"
          color="primary"
          startIcon={<PersonAddIcon />}
          onClick={() => setAddDialogOpen(true)}
          sx={{ whiteSpace: 'nowrap' }}
        >
          {t('contacts.add.button')}
        </Button>
      </Stack>
      {selectedCircle && (
        <Box sx={{ mb: 2, p: 1.5, bgcolor: 'action.hover', borderRadius: 1, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          <Chip 
            label={selectedCircle} 
            size="small" 
            onDelete={clearCircle} 
          />
        </Box>
      )}
      <BulkActionsBar
        selectedCount={selectedUids.size}
        loadedCount={contacts.length}
        allSelected={allSelected}
        circles={circles}
        tags={tags}
        busy={bulkBusy}
        onSelectAll={toggleSelectAll}
        onClear={clearSelection}
        onAddCircle={handleAddCircle}
        onRemoveCircle={handleRemoveCircle}
        onAddTag={handleAddTag}
        onRemoveTag={handleRemoveTag}
        onArchive={handleArchive}
        onUnarchive={handleUnarchive}
        onDelete={handleBulkDelete}
      />
      {loading && contacts.length === 0 ? (
        <ContactListSkeleton count={10} />
      ) : (
        <>
          <Stack spacing={2}>
            {contacts.map(contact => (
              <Card
                key={contact.ID}
                onClick={() => navigate(`/contacts/${contact.ID}`)}
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  p: 1.5,
                  cursor: 'pointer',
                  textDecoration: 'none',
                  color: 'inherit',
                  bgcolor: contact.archived ? 'action.disabledBackground' : undefined,
                  '&:hover': {
                    bgcolor: 'action.hover'
                  }
                }}
              >
                <Checkbox
                  checked={isSelected(contact.uid)}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) => { e.stopPropagation(); toggleSelect(contact.uid); }}
                  disabled={bulkBusy}
                  inputProps={{ 'aria-label': t('bulk.selectContact', { name: `${contact.firstname} ${contact.lastname}`.trim() }) }}
                />
                <Avatar src={contact.photo_thumbnail || undefined} sx={{ width: 48, height: 48, mr: 1.5, bgcolor: 'primary.main' }}>
                  {contact.firstname.charAt(0)}
                </Avatar>
                <Box sx={{ flex: 1 }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="body1" sx={{ fontWeight: 500 }}>
                      {[contact.firstname, contact.nickname && `"${contact.nickname}"`, contact.lastname].filter(Boolean).join(' ')}
                    </Typography>
                    {contact.archived && (
                      <Chip
                        label={t('contacts.archived')}
                        size="small"
                        color="default"
                        sx={{ height: 20, fontSize: '0.7rem' }}
                      />
                    )}
                  </Box>
                  <Stack direction="row" spacing={0.5} mt={0.5} flexWrap="wrap" gap={0.5}>
                    {(circleNamesByUid.get(contact.uid || '') || []).map((name: string) => (
                      <Chip
                        key={`${contact.ID}-${name}`}
                        label={name}
                        size="small"
                        variant="outlined"
                        clickable
                        onClick={(e) => { e.preventDefault(); e.stopPropagation(); setSelectedCircle(name); }}
                        sx={{ height: 20, fontSize: '0.75rem' }}
                      />
                    ))}
                  </Stack>
                </Box>
              </Card>
            ))}
          </Stack>
          {showNoResults && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
              {t('contacts.searchNoResults', { query: searchQuery })}
            </Typography>
          )}
          {nextCursor && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
              <Button variant="outlined" onClick={loadMore} disabled={loading}>
                {t('common.loadMore')}
              </Button>
            </Box>
          )}
        </>
      )}
      {searchActive && (
        <SearchNotesActivities
          query={searchQuery}
          result={crossResultMatches ? searchResult : null}
          onOpenContact={(id) => navigate(`/contacts/${id}`)}
        />
      )}
      <AddContactDialog
        open={addDialogOpen}
        onClose={() => setAddDialogOpen(false)}
        onContactAdded={handleContactAdded}
        availableCircles={circles}
        availableTags={tags}
        enabledFields={enabledFields}
      />
      <ImportContactsDialog
        open={importDialogOpen}
        onClose={() => setImportDialogOpen(false)}
        onImportComplete={handleImportComplete}
      />
    </Box>
  );
}
