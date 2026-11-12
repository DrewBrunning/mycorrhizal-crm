package com.mycorrhizal.crm.ui

import androidx.compose.runtime.Composable
import com.mycorrhizal.crm.domain.compat.ServerCapabilities
import com.mycorrhizal.crm.domain.compat.ServerFeature

/**
 * Whether the connected server (the version resolved once per session into
 * [LocalServerVersion], issue #528) is new enough to provide [feature] (issue #2527).
 * Fail-open: an unknown version is "supported", so an unreachable /health never hides anything.
 */
@Composable
fun isServerFeatureSupported(feature: ServerFeature): Boolean =
    ServerCapabilities.isSupported(LocalServerVersion.current, feature)
