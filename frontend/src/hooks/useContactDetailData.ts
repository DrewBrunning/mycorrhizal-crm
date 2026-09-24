import { useCallback, useEffect, useRef, useState } from 'react';
import { type Activity, getContactActivities } from '../api/activities';
import { getCurrentUser } from '../api/admin';
import {
  type ContactRecordResponse,
  getContactProfilePicture,
  getContactRecord,
} from '../api/contacts';
import { getContactNotes, type Note } from '../api/notes';
import { getCompletionsForContact, type ReminderCompletion } from '../api/reminders';
import { getCachedSelfContactVCardUID } from '../auth';
import { type ContactFieldKey, resolveEnabledFields } from '../contactFields';
import type { User } from '../types';
import { handleFetchError } from '../utils/errorHandler';

// Data-loading orchestration for ContactDetailPage, split in two because of
// hook-call ordering: useContactDetailData owns the page-level state (the
// record every other per-contact hook keys off), and useContactDetailLoader
// runs the mount/id-change fetch once the page has instantiated those other
// hooks and can hand over their refresh functions as `loadDependents`.

export interface ContactDetailCore {
  record: ContactRecordResponse;
  notes: Note[];
  activities: Activity[];
  completions: ReminderCompletion[];
  // null when /users/me failed -- the page falls back to defaults.
  user: User | null;
  // True when notes, activities, or reminder completions failed and fell back
  // to an empty list.
  auxFetchFailed: boolean;
}

// First batch: parallel fetch of core data. Only getContactRecord is allowed
// to reject (and so gate the page's not-found branch) -- notes, activities,
// and completions are auxiliary timeline data, and one of them 500ing must
// not make an existing contact look deleted (issue #958). Each is isolated
// with its own .catch() so a single failure falls back to an empty list and
// is reported via auxFetchFailed instead of rejecting the whole Promise.all.
export async function fetchContactDetailCore(id: string): Promise<ContactDetailCore> {
  let auxFetchFailed = false;
  const [record, notesData, activitiesData, completionsData, user] = await Promise.all([
    getContactRecord(id),
    getContactNotes(id).catch((err: unknown) => {
      console.error('Error fetching contact notes:', err);
      auxFetchFailed = true;
      return { notes: [] as Note[] };
    }),
    getContactActivities(id).catch((err: unknown) => {
      console.error('Error fetching contact activities:', err);
      auxFetchFailed = true;
      return { activities: [] as Activity[] };
    }),
    getCompletionsForContact(parseInt(id, 10)).catch((err: unknown) => {
      console.error('Error fetching reminder completions:', err);
      auxFetchFailed = true;
      return [] as ReminderCompletion[];
    }),
    getCurrentUser().catch((err: unknown) => {
      console.error('Error fetching current user preferences:', err);
      return null;
    }),
  ]);
  return {
    record,
    notes: notesData.notes || [],
    activities: activitiesData.activities || [],
    completions: completionsData || [],
    user,
    auxFetchFailed,
  };
}

// Resolves the profile picture to show: an object URL for the fetched blob,
// '' when there is none, or undefined when the fetch failed (leave whatever
// is showing). Only fetches when the record says it has a photo, avoiding an
// unnecessary 404.
export async function fetchProfilePictureUrl(
  id: string,
  hasPhoto: boolean,
): Promise<string | undefined> {
  if (!hasPhoto) return '';
  try {
    const blob = await getContactProfilePicture(id);
    return blob ? URL.createObjectURL(blob) : '';
  } catch (err) {
    console.error('Error fetching profile picture:', err);
    return undefined;
  }
}

export function useContactDetailData(id: string | undefined) {
  // record is the single source of truth, fetched/written directly against
  // the nested Card/CRM wire shape.
  const [record, setRecord] = useState<ContactRecordResponse | null>(null);
  // T90: VCardUID of the caller's "Me" contact. Seeded from the localStorage
  // cache (written at login / after a Settings picker change) so a fresh page
  // shows the badge immediately, then corrected from this page's own
  // /users/me fetch when it lands.
  const [selfContactUid, setSelfContactUid] = useState<string | null>(() =>
    getCachedSelfContactVCardUID(),
  );
  const [profilePic, setProfilePic] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [notes, setNotes] = useState<Note[]>([]);
  const [activities, setActivities] = useState<Activity[]>([]);
  const [completions, setCompletions] = useState<ReminderCompletion[]>([]);
  // Enabled extended contact fields (UI visibility)
  const [enabledFields, setEnabledFields] = useState<Set<ContactFieldKey>>(() =>
    resolveEnabledFields(null),
  );
  // T78: a revision counter bumped whenever the page's timeline data changes
  // so the timeline explorer's own paginated fetch (which the page-level edit
  // dialogs can't touch) refreshes to match.
  const [timelineRevision, setTimelineRevision] = useState(0);

  const applyCore = useCallback((core: ContactDetailCore) => {
    setRecord(core.record);
    setNotes(core.notes);
    setActivities(core.activities);
    setCompletions(core.completions);
    setEnabledFields(resolveEnabledFields(core.user?.enabled_contact_fields ?? null));
    // T90: this page's own /users/me fetch is fresher than the localStorage
    // cache (e.g. right after a Settings picker change); take its value. Only
    // when the fetch actually succeeded -- a transient failure shouldn't hide
    // a badge the cache had.
    if (core.user) {
      setSelfContactUid(core.user.self_contact_vcard_uid ?? null);
    }
  }, []);

  // Unified refresh for notes, activities, and completions.
  const refreshNotesAndActivities = useCallback(async () => {
    if (!id) return;
    try {
      const [notesData, activitiesData, completionsData] = await Promise.all([
        getContactNotes(id),
        getContactActivities(id),
        getCompletionsForContact(parseInt(id, 10)),
      ]);
      setNotes(notesData.notes || []);
      setActivities(activitiesData.activities || []);
      setCompletions(completionsData || []);
      // Bump the explorer's revision: a save/delete went through the
      // page-level dialogs, so the explorer's own fetch needs to catch up.
      setTimelineRevision((r) => r + 1);
    } catch (err) {
      handleFetchError(err, 'refreshing notes and activities');
    }
  }, [id]);

  // Refetches the full contact record after a server-side change that touches
  // the card (e.g. a married LifeEvent mirrored onto the wedding anniversary).
  const reloadRecord = useCallback(async () => {
    if (!id) return;
    try {
      setRecord(await getContactRecord(id));
    } catch {
      // leave the current record as-is
    }
  }, [id]);

  return {
    record,
    setRecord,
    selfContactUid,
    setSelfContactUid,
    profilePic,
    setProfilePic,
    loading,
    setLoading,
    notes,
    activities,
    completions,
    enabledFields,
    timelineRevision,
    applyCore,
    refreshNotesAndActivities,
    reloadRecord,
  };
}

export interface ContactDetailLoaderOptions {
  applyCore: (core: ContactDetailCore) => void;
  setProfilePic: (url: string) => void;
  setLoading: (loading: boolean) => void;
  // Second batch, run once the record is known: every per-contact hook's
  // refresh, passed the freshly-fetched record directly rather than relying
  // on the record state var -- that state hasn't re-rendered yet at this
  // point, so relying on it would silently fetch nothing on a fresh load.
  // Its identity is the effect's re-run trigger, so memoize it.
  loadDependents: (record: ContactRecordResponse) => Promise<unknown>;
  // Called when an auxiliary timeline fetch fell back to empty. Read through
  // a ref, not a dependency: a notifier change must not refetch the page.
  onAuxFetchFailed: () => void;
}

export function useContactDetailLoader(
  id: string | undefined,
  {
    applyCore,
    setProfilePic,
    setLoading,
    loadDependents,
    onAuxFetchFailed,
  }: ContactDetailLoaderOptions,
) {
  const onAuxFetchFailedRef = useRef(onAuxFetchFailed);
  onAuxFetchFailedRef.current = onAuxFetchFailed;

  useEffect(() => {
    if (!id) return;

    let currentBlobUrl: string | null = null;

    const fetchData = async () => {
      try {
        const core = await fetchContactDetailCore(id);
        applyCore(core);
        if (core.auxFetchFailed) {
          onAuxFetchFailedRef.current();
        }

        await loadDependents(core.record);

        const pic = await fetchProfilePictureUrl(id, !!core.record.photo);
        if (pic !== undefined) {
          if (pic) currentBlobUrl = pic;
          setProfilePic(pic);
        }

        setLoading(false);
      } catch (err) {
        console.error('Error fetching data:', err);
        setLoading(false);
      }
    };

    void fetchData();

    return () => {
      if (currentBlobUrl) {
        URL.revokeObjectURL(currentBlobUrl);
      }
    };
  }, [id, applyCore, setProfilePic, setLoading, loadDependents]);
}
