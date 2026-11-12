package com.mycorrhizal.crm

import android.net.Uri

/** Route pattern of the standing-share screen for a note, an activity or a circle (issue #2518). */
internal const val STANDING_SHARE_ROUTE = "standing-share/{entityType}/{entityId}"

/** Route of the standing-share screen for [entityType] (a `ShareGrantEntityTypes` value) / [entityId]. */
internal fun standingShareRoute(entityType: String, entityId: String): String =
    "standing-share/${Uri.encode(entityType)}/${Uri.encode(entityId)}"
