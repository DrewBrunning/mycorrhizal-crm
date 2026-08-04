import { useEffect, useState, useMemo, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useContacts } from './hooks/useContacts';
import { useCircles } from './hooks/useCircles';
import { useTags } from './hooks/useTags';
import { useFieldDefinitions } from './hooks/useFieldDefinitions';
import { getCurrentUser } from './api/admin';
import { resolveEnabledFields, ContactFieldKey } from './contactFields';
import { BulkAction, runBulkOperation } from './api/bulkOperations';
import AddContactDialog from './components/AddContactDialog';
import ImportContactsDialog from './components/ImportContactsDialog';
import BulkActionsBar from './components/BulkActionsBar';
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
  Checkbox
} from '@mui/material';
import PersonAddIcon from '@mui/icons-material/PersonAdd';
import FileUploadIcon from '@mui/icons-material/FileUpload';
import { ContactListSkeleton } from './components/LoadingSkeletons';

export default function ContactsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const searchQuery = searchParams.get('search') || '';
  const [selectedCircle, setSelectedCircle] = useState('');
  const { circles, circleNamesByUid, refresh: refreshCircles } = useCircles();
  const { tags, refresh: refreshTags } = useTags();

  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [enabledFields, setEnabledFields] = useState<Set<ContactFieldKey>>(() => resolveEnabledFields(null));
  const [showArchived, setShowArchived] = useState(false);
  const pageSize = 10;

  // N5 multi-select: keyed by Contact.VCardUID so it survives pagination —
  // "load more" appends to the list but the selection Set is untouched.
  const [selectedUids, setSelectedUids] = useState<Set<string>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);

  // T17: cursor pagination — the list pages by (updated_at, id) DESC and the
  // "load more" button appends the next_cursor page. There is no page number
  // or exact total anymore.
  const contactParams = useMemo(() => ({
    limit: pageSize,
    search: searchQuery,
    circle: selectedCircle,
    order: 'desc' as const,
    includeArchived: showArchived,
  }), [searchQuery, selectedCircle, showArchived]);

  // Use custom hook for fetching contacts
  const { contacts, nextCursor, loading, refetch, loadMore } = useContacts(contactParams);

  // Custom field definitions (T7): the add-contact dialog needs the typed
  // definitions to render per-type value editors.
  const { definitions: fieldDefinitions } = useFieldDefinitions();

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
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems="center">
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
          variant="outlined"
          startIcon={<PersonAddIcon />}
          onClick={() => setAddDialogOpen(true)}
          sx={{ whiteSpace: 'nowrap' }}
        >
          {t('contacts.add.button')}
        </Button>
      </Stack>
      {(contacts.length > 0 || searchQuery || selectedCircle) && (
        <Box sx={{ mb: 2, p: 1.5, bgcolor: 'action.hover', borderRadius: 1, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          {searchQuery && (
            <Chip 
              label={`"${searchQuery}"`} 
              size="small" 
              onDelete={() => navigate('/contacts')} 
            />
          )}
          {selectedCircle && (
            <Chip 
              label={selectedCircle} 
              size="small" 
              onDelete={clearCircle} 
            />
          )}
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
          {nextCursor && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
              <Button variant="outlined" onClick={loadMore} disabled={loading}>
                {t('common.loadMore')}
              </Button>
            </Box>
          )}
        </>
      )}
      <AddContactDialog
        open={addDialogOpen}
        onClose={() => setAddDialogOpen(false)}
        onContactAdded={handleContactAdded}
        availableCircles={circles}
        availableTags={tags}
        fieldDefinitions={fieldDefinitions}
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
