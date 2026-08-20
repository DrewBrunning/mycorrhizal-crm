package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import android.graphics.PixelFormat
import android.view.Gravity
import android.view.WindowManager
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter

/**
 * A `SYSTEM_ALERT_WINDOW` overlay shown after a call ends (§6.5). Renders a
 * pre-filled activity form ([QuickCaptureSheet]) so an interaction can be
 * logged without leaving the call screen; the caller's number is matched to a
 * contact for the participant chip. The overlay requires the SYSTEM_ALERT_WINDOW
 * special permission; without it, nothing is shown (graceful degradation).
 * It dismisses itself after the [QuickCaptureAutoDismiss] window — which
 * exists so an ignored sheet does not sit on screen forever, and only applies
 * while the sheet is untouched: the first focus or keystroke cancels it, so a
 * user mid-form is never timed out (WCAG 2.2.1, #201). It also dismisses on
 * tap of X/Cancel. Because the timer only fires with no interaction at all,
 * nothing typed is ever silently dropped — there is nothing to drop.
 *
 * An instance (not a singleton `object`) so the view it retains is scoped to
 * its owner's lifetime rather than the process's — [CallDetectionService]
 * owns one and calls [dismiss] from `onDestroy`. A static/singleton holder
 * here would pin a Context-carrying View for as long as the process runs
 * (lint: StaticFieldLeak).
 *
 * The form is a full Compose tree inside the overlay window (there is no
 * ViewModelStoreOwner there, so editing/saving state lives in
 * [QuickCaptureFormState], which is unit-testable without Compose).
 */
class QuickCaptureOverlay(
    private val contactRepository: ContactRepository,
    private val activityRepository: ActivityRepository,
) {

    private var composeView: ComposeView? = null
    private val autoDismiss = QuickCaptureAutoDismiss(onDismiss = { dismiss() })

    fun show(context: Context, phoneNumber: String? = null) {
        dismiss()
        val windowManager = context.getSystemService(Context.WINDOW_SERVICE) as WindowManager
        val sheet = ComposeView(context).apply {
            setContent {
                MycorrhizalTheme(darkTheme = isSystemInDarkTheme()) {
                    QuickCaptureSheetHost(
                        phoneNumber = phoneNumber,
                        contactRepository = contactRepository,
                        activityRepository = activityRepository,
                        onDismiss = { dismiss() },
                        onFirstInteraction = autoDismiss::cancel,
                    )
                }
            }
        }
        val params = WindowManager.LayoutParams(
            WindowManager.LayoutParams.MATCH_PARENT,
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
            // The sheet holds editable text, so it must be focusable (a
            // NOT_FOCUSABLE overlay cannot receive the keyboard).
            WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.BOTTOM
            y = (BOTTOM_OFFSET_DP * context.resources.displayMetrics.density).toInt()
            softInputMode = WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE
        }
        runCatching { windowManager.addView(sheet, params) }
            .onSuccess { composeView = sheet }
        autoDismiss.arm()
    }

    fun dismiss() {
        autoDismiss.cancel()
        composeView?.let { v ->
            runCatching {
                (v.context.getSystemService(Context.WINDOW_SERVICE) as WindowManager).removeView(v)
            }
            composeView = null
        }
    }

    internal fun isShowingForTest(): Boolean = composeView != null

    private companion object {
        const val BOTTOM_OFFSET_DP = 120f
    }
}

/**
 * Resolves the caller's number to a contact (best-effort, offline Room match)
 * and hands a [QuickCaptureFormState] to the sheet. [resolving] keeps the chip
 * honest while the lookup is in flight. [onFirstInteraction] cancels the
 * overlay's auto-dismiss timer the first time the user engages the form (#201).
 */
@Composable
private fun QuickCaptureSheetHost(
    phoneNumber: String?,
    contactRepository: ContactRepository,
    activityRepository: ActivityRepository,
    onDismiss: () -> Unit,
    onFirstInteraction: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    var contact by remember(phoneNumber) { mutableStateOf<ContactSummary?>(null) }
    var resolved by remember(phoneNumber) { mutableStateOf(phoneNumber.isNullOrBlank()) }

    LaunchedEffect(phoneNumber) {
        if (!phoneNumber.isNullOrBlank()) {
            contact = runCatching { contactRepository.findByPhone(phoneNumber) }.getOrNull()
        }
        resolved = true
    }

    // stringResource (not LocalContext.current.getString) so a Configuration
    // change (locale) invalidates this instead of leaving a stale string
    // baked into the remembered FormState -- lint's LocalContextGetResourceValueCall.
    val blankTitleMessage = stringResource(R.string.quick_capture_title_required)
    val formState = remember(contact, resolved, blankTitleMessage) {
        QuickCaptureFormState(
            activityRepository = activityRepository,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(
                contact = contact,
                nowIso = nowIsoString(),
            ),
            blankTitleMessage = blankTitleMessage,
            onFirstInteraction = onFirstInteraction,
        )
    }

    QuickCaptureSheet(
        state = formState,
        onDismiss = onDismiss,
        onFirstInteraction = onFirstInteraction,
        resolving = !resolved,
    )
}

private fun nowIsoString(): String =
    DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(Instant.now().atOffset(ZoneOffset.UTC))
