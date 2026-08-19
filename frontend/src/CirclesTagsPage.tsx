import { useMemo, SyntheticEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { Box, Typography, Tabs, Tab, Alert, LinearProgress } from '@mui/material';
import CircleTagEntityList from './components/CircleTagEntityList';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useCircles } from './hooks/useCircles';
import { useTags } from './hooks/useTags';
import { useSnackbar } from './context/SnackbarContext';

// T65: the backend, API client and hooks (useCircles/useTags) already had
// full rename/delete support — no page called it. This is the missing
// entry point, mirroring Android's standalone CirclesScreen/TagsScreen
// rather than bolting rename/delete onto the existing side-effect-only
// surfaces (ContactHeader's per-contact add rows, the Contacts filter
// dropdown) — those stay create-only, exactly as they are today.
type TabKey = 'circles' | 'tags';

export default function CirclesTagsPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.circlesTags'));
  const { showError, showSuccess } = useSnackbar();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: TabKey = searchParams.get('tab') === 'tags' ? 'tags' : 'circles';

  const {
    circles,
    members: circleMembers,
    loading: circlesLoading,
    error: circlesError,
    refresh: refreshCircles,
    handleCreate: handleCreateCircle,
    handleUpdate: handleUpdateCircle,
    handleDelete: handleDeleteCircle,
  } = useCircles({ showError });

  const {
    tags,
    contacts: tagContacts,
    loading: tagsLoading,
    error: tagsError,
    refresh: refreshTags,
    handleCreate: handleCreateTag,
    handleUpdate: handleUpdateTag,
    handleDelete: handleDeleteTag,
  } = useTags({ showError });

  const circleItems = useMemo(() => {
    const counts = new Map<string, number>();
    for (const m of circleMembers) counts.set(m.circle_id, (counts.get(m.circle_id) || 0) + 1);
    return circles.map((c) => ({ id: c.id, name: c.name, memberCount: counts.get(c.id) || 0 }));
  }, [circles, circleMembers]);

  const tagItems = useMemo(() => {
    const counts = new Map<string, number>();
    for (const ct of tagContacts) counts.set(ct.tag_id, (counts.get(ct.tag_id) || 0) + 1);
    return tags.map((tg) => ({ id: tg.id, name: tg.name, memberCount: counts.get(tg.id) || 0 }));
  }, [tags, tagContacts]);

  const handleTabChange = (_event: SyntheticEvent, value: TabKey) => {
    setSearchParams(value === 'circles' ? {} : { tab: value }, { replace: true });
  };

  return (
    <Box sx={{ maxWidth: 720, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" component="h1" gutterBottom>
        {t('circlesTags.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        {t('circlesTags.description')}
      </Typography>

      <Tabs value={tab} onChange={handleTabChange} sx={{ mb: 2 }}>
        <Tab label={t('circlesTags.tabCircles')} value="circles" />
        <Tab label={t('circlesTags.tabTags')} value="tags" />
      </Tabs>

      {tab === 'circles' ? (
        <>
          {circlesError && <Alert severity="error" sx={{ mb: 2 }}>{circlesError}</Alert>}
          {circlesLoading && <LinearProgress sx={{ mb: 2 }} />}
          <CircleTagEntityList
            items={circleItems}
            loading={circlesLoading}
            newPlaceholder={t('circlesTags.newCirclePlaceholder')}
            addLabel={t('circlesTags.add')}
            emptyLabel={t('circlesTags.noCircles')}
            memberCountLabel={(count) => t('circlesTags.memberCount', { count })}
            deleteConfirmLabel={(name) => t('circlesTags.deleteCircleConfirm', { name })}
            onCreate={async (name) => {
              // handleCreateCircle doesn't refresh internally (ContactDetailPage's
              // handleCircleAdd orchestrates that itself the same way) — do it here.
              await handleCreateCircle(name);
              await refreshCircles();
              showSuccess(t('circlesTags.circleCreated'));
            }}
            onRename={async (id, name) => {
              await handleUpdateCircle(id, name);
              showSuccess(t('circlesTags.circleRenamed'));
            }}
            onDelete={async (id) => {
              await handleDeleteCircle(id);
              showSuccess(t('circlesTags.circleDeleted'));
            }}
          />
        </>
      ) : (
        <>
          {tagsError && <Alert severity="error" sx={{ mb: 2 }}>{tagsError}</Alert>}
          {tagsLoading && <LinearProgress sx={{ mb: 2 }} />}
          <CircleTagEntityList
            items={tagItems}
            loading={tagsLoading}
            newPlaceholder={t('circlesTags.newTagPlaceholder')}
            addLabel={t('circlesTags.add')}
            emptyLabel={t('circlesTags.noTags')}
            memberCountLabel={(count) => t('circlesTags.memberCount', { count })}
            deleteConfirmLabel={(name) => t('circlesTags.deleteTagConfirm', { name })}
            onCreate={async (name) => {
              await handleCreateTag(name);
              await refreshTags();
              showSuccess(t('circlesTags.tagCreated'));
            }}
            onRename={async (id, name) => {
              await handleUpdateTag(id, name);
              showSuccess(t('circlesTags.tagRenamed'));
            }}
            onDelete={async (id) => {
              await handleDeleteTag(id);
              showSuccess(t('circlesTags.tagDeleted'));
            }}
          />
        </>
      )}
    </Box>
  );
}
