package com.mycorrhizal.crm

import androidx.lifecycle.ViewModel
import com.mycorrhizal.crm.data.share.ShareDraftHolder
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

/**
 * ADR 0029 §6 (issue #1271): the composable-facing handle on [ShareDraftHolder]. Text shared
 * into the app is stashed under a random key so the picker route carries only the key, never
 * the text; the note form takes it once. Backing out of the picker/form discards it.
 */
@HiltViewModel
class ShareIntakeViewModel @Inject constructor(
    private val drafts: ShareDraftHolder,
) : ViewModel() {

    /** Stashes [text] and returns the key to route with. */
    fun stash(text: String): String = drafts.put(text)

    /** Drops the draft behind [key] (the picker was backed out of). */
    fun discard(key: String) {
        drafts.take(key)
    }

    /** Drops every held draft (the main tree was torn down — logout or lock). */
    fun discardAll() = drafts.clear()
}

/** Route of the "Share to…" contact picker; [key] is the [ShareDraftHolder] key. */
internal fun sharePickerRoute(key: String): String = "share/pick-contact?key=${android.net.Uri.encode(key)}"

/** Route of the new-note form prefilled from the [ShareDraftHolder] draft behind [key]. */
internal fun sharedNoteRoute(contactId: Int, key: String): String =
    "contacts/$contactId/notes/new?prefill=${android.net.Uri.encode(key)}"
