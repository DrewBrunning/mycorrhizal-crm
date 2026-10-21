package com.mycorrhizal.crm.ui.components

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.AMBIENT_REASON_HOUSEHOLD
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1734: the plain-language reason an ambient contact counts as in regular contact — the
 * explanation shown instead of a days-since-last-interaction figure. [reason] is the server's
 * `ambient_reason` (`household` or `flag`). Anything else (an absent or future value on an
 * ambient contact) falls back to the flag wording, which makes no claim about *why*.
 */
@StringRes
fun ambientReasonRes(reason: String?): Int = when (reason) {
    AMBIENT_REASON_HOUSEHOLD -> R.string.ambient_reason_household
    else -> R.string.ambient_reason_flag
}

/**
 * Issue #1734: the ambient explanation block — the reason (`Lives with you (household)` or
 * `Marked as ambient`) and, when [showExplainer], the one-line "counts as in regular contact
 * without logging activities" note. Shown in place of any days-since-last-interaction or overdue
 * readout for an ambient contact.
 */
@Composable
fun AmbientExplanation(
    reason: String?,
    modifier: Modifier = Modifier,
    showExplainer: Boolean = true,
) {
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            text = stringResource(ambientReasonRes(reason)),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.tertiary,
        )
        if (showExplainer) {
            Text(
                text = stringResource(R.string.ambient_explainer),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
