import { useMemo } from 'react';
import { addCircleMember, type Circle, removeCircleMember } from '../api/circles';
import { addContactTag, removeContactTag, type Tag } from '../api/tags';
import type { ErrorNotifier } from '../utils/errorHandler';
import { useCircles } from './useCircles';
import { useTags } from './useTags';

// Circle and tag membership for one contact (T4 -- real entities instead of
// flat strings): the user-wide circle/tag lists, the subset this contact is
// in, and add/remove handlers. Adding an unsaved circle/tag (no id yet --
// typed free-solo in the header's autocomplete) creates it first.
//
// Every handler refreshes afterwards whether or not the mutation succeeded:
// the create/add error is already reported by useCircles/useTags' own
// notifier (or is a membership call the refresh reconciles), so the refresh
// is what brings the UI back in line with the server.
export function useContactMemberships(contactUid: string | undefined, notifier?: ErrorNotifier) {
  const {
    circles: allCircles,
    circleNamesByUid,
    refresh: refreshCircles,
    handleCreate: handleCreateCircle,
  } = useCircles(notifier);

  const {
    tags: allTags,
    tagNamesByUid,
    refresh: refreshTags,
    handleCreate: handleCreateTag,
  } = useTags(notifier);

  const contactCircles = useMemo(() => {
    if (!contactUid) return [];
    const names = circleNamesByUid.get(contactUid) || [];
    return allCircles.filter((c) => names.includes(c.name));
  }, [contactUid, circleNamesByUid, allCircles]);

  const contactTags = useMemo(() => {
    if (!contactUid) return [];
    const names = tagNamesByUid.get(contactUid) || [];
    return allTags.filter((t) => names.includes(t.name));
  }, [contactUid, tagNamesByUid, allTags]);

  const handleCircleAdd = async (circle: Circle) => {
    if (!contactUid) return;
    try {
      if (circle.id) {
        await addCircleMember(circle.id, contactUid);
      } else {
        const created = await handleCreateCircle(circle.name);
        if (created?.id) await addCircleMember(created.id, contactUid);
      }
      await refreshCircles();
    } catch {
      await refreshCircles();
    }
  };

  const handleCircleRemove = async (circle: Circle) => {
    if (!contactUid) return;
    try {
      await removeCircleMember(circle.id, contactUid);
      await refreshCircles();
    } catch {
      await refreshCircles();
    }
  };

  const handleTagAdd = async (tag: Tag) => {
    if (!contactUid) return;
    try {
      if (tag.id) {
        await addContactTag(tag.id, contactUid);
      } else {
        const created = await handleCreateTag(tag.name);
        if (created?.id) await addContactTag(created.id, contactUid);
      }
      await refreshTags();
    } catch {
      await refreshTags();
    }
  };

  const handleTagRemove = async (tag: Tag) => {
    if (!contactUid) return;
    try {
      await removeContactTag(tag.id, contactUid);
      await refreshTags();
    } catch {
      await refreshTags();
    }
  };

  return {
    allCircles,
    allTags,
    contactCircles,
    contactTags,
    refreshCircles,
    refreshTags,
    handleCircleAdd,
    handleCircleRemove,
    handleTagAdd,
    handleTagRemove,
  };
}
