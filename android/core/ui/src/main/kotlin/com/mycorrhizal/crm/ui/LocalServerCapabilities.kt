package com.mycorrhizal.crm.ui

import androidx.compose.runtime.staticCompositionLocalOf
import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesInfo

/**
 * The capability set the current session resolved from GET /health (issue
 * #1263, ADR 0028 Decision 2), provided at the app root next to
 * [LocalServerVersion]. Screens consult it to hide entries a deployment does not
 * offer (an embedded local server omits shares, webhooks, API tokens, DAV
 * serving, push, and the account/admin surfaces).
 *
 * [ServerCapabilitiesInfo.Unknown] is the fail-open default: before the check
 * resolves, in previews, or when /health was unreachable, every capability is
 * treated as present — the client must never hide functionality because it could
 * not confirm the server's capability list.
 */
val LocalServerCapabilities = staticCompositionLocalOf { ServerCapabilitiesInfo.Unknown }
