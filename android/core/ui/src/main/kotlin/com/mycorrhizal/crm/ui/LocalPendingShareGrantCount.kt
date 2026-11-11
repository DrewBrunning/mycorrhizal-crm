package com.mycorrhizal.crm.ui

import androidx.compose.runtime.compositionLocalOf

/**
 * The number of standing shares waiting for the signed-in user to accept (ADR 0042 decision 11),
 * provided at the app root from the server's in-app notification count so the drawer, the rail and
 * the shares screen can badge without each owning a view model. A count only — never a label or
 * sender. Defaults to 0 in previews/tests and before the first load.
 */
val LocalPendingShareGrantCount = compositionLocalOf { 0 }
