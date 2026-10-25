package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ScoreReasonCode
import com.mycorrhizal.crm.model.network.ScoreReasonParams
import com.mycorrhizal.crm.ui.R
import java.text.NumberFormat

/**
 * A relationship-health explanation in the user's language, rendered from the
 * server's [reasonCode] + [params] (v1.5.0 review: the English `reason`, with
 * its "day(s)", used to be shown verbatim in every locale).
 *
 * Falls back to [fallback] (the server's English sentence) for a code this
 * build does not know, a missing parameter, or a server that predates reason
 * codes, so a newer server never renders a broken template.
 */
@Composable
fun scoreReasonText(reasonCode: String?, params: ScoreReasonParams, fallback: String): String {
    val every: @Composable (Int) -> String = { n ->
        pluralStringResource(R.plurals.score_reason_every, n, n)
    }
    val text: String? = when (reasonCode) {
        ScoreReasonCode.CLOSENESS_DIRECT -> params.relation?.let { relation ->
            stringResource(R.string.score_reason_closeness_direct, relationshipRoleLabel(relation))
        }

        ScoreReasonCode.CLOSENESS_HOPS -> params.hops?.let { hops ->
            pluralStringResource(R.plurals.score_reason_closeness_hops, hops, hops)
        }

        ScoreReasonCode.CLOSENESS_UNKNOWN -> stringResource(R.string.score_reason_closeness_unknown)

        ScoreReasonCode.RECENCY_NONE -> stringResource(R.string.score_reason_recency_none)

        ScoreReasonCode.RECENCY_DAYS -> {
            val days = params.days
            val interval = params.intervalDays
            if (days != null && interval != null) {
                pluralStringResource(
                    R.plurals.score_reason_recency_days,
                    days,
                    days,
                    every(interval)
                )
            } else {
                null
            }
        }

        ScoreReasonCode.FREQUENCY_COUNT -> {
            val count = params.count
            val window = params.windowDays
            val expected = params.expected
            if (count != null && window != null && expected != null) {
                // The app's configured locale (the per-app language), not the
                // JVM default, decides "12.9" vs "12,9".
                val locale = LocalConfiguration.current.locales[0]
                val expectedText = NumberFormat.getNumberInstance(locale).apply {
                    maximumFractionDigits = 1
                }.format(expected)
                pluralStringResource(
                    R.plurals.score_reason_frequency_count,
                    count,
                    count,
                    pluralStringResource(R.plurals.score_reason_last_days, window, window),
                    expectedText
                )
            } else {
                null
            }
        }

        ScoreReasonCode.REACH_OUT_PENDING -> stringResource(R.string.score_reason_reach_out_pending)

        ScoreReasonCode.REACH_OUT_NONE -> stringResource(R.string.score_reason_reach_out_none)

        ScoreReasonCode.LAST_UPDATED_DAYS -> params.days?.let { days ->
            pluralStringResource(R.plurals.score_reason_last_updated_days, days, days)
        }

        ScoreReasonCode.AMBIENT_HOUSEHOLD -> stringResource(R.string.score_reason_ambient_household)

        ScoreReasonCode.AMBIENT_FLAG -> stringResource(R.string.score_reason_ambient_flag)

        ScoreReasonCode.TARGET_CADENCE_POLICY -> {
            val score = params.score
            val interval = params.intervalDays
            if (score != null && interval != null) {
                stringResource(R.string.score_reason_target_cadence_policy, score, every(interval))
            } else {
                null
            }
        }

        ScoreReasonCode.TARGET_TIER -> {
            val score = params.score
            val interval = params.intervalDays
            val tier = params.tier
            if (score != null && interval != null && tier != null) {
                stringResource(
                    R.string.score_reason_target_tier,
                    score,
                    targetTierLabel(tier),
                    every(interval)
                )
            } else {
                null
            }
        }

        ScoreReasonCode.TARGET_AMBIENT -> stringResource(R.string.score_reason_target_ambient)

        else -> null
    }
    return text ?: fallback
}
