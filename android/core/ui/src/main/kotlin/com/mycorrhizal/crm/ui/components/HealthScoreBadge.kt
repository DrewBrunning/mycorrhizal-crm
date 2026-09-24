package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.LocalWarningColors

/**
 * Issue #383 (ADR-0023): the moss/chanterelle/russula band's theme color.
 *
 * `Theme.kt` only overrides the `tertiary` (moss) and `error` (russula) M3
 * color-scheme slots — `tertiaryContainer`/`errorContainer` are left at M3's
 * generic, un-vetted tonal defaults, so this deliberately does NOT reach for
 * those. Chanterelle (the middle, "warning" band) has no M3 slot at all,
 * which is exactly why `LocalWarningColors` exists (see `Theme.kt`'s doc
 * comment) — `foreground` is the contrast-checked non-text warning pair
 * (borders, icon tints), the same one `CadenceScreen`'s overdue indicator
 * uses.
 */
@Composable
fun healthBandColor(band: String): Color = when (band) {
    "moss" -> MaterialTheme.colorScheme.tertiary
    "chanterelle" -> LocalWarningColors.current.foreground
    "russula" -> MaterialTheme.colorScheme.error
    else -> MaterialTheme.colorScheme.onSurfaceVariant
}

/**
 * The band's plain-language label, e.g. for a screen-reader-only
 * accessible name. Color alone must never carry this meaning (see
 * [HealthScoreBadge]'s doc comment), so every call site that shows a
 * band-tinted indicator also surfaces this in words somewhere reachable by
 * TalkBack.
 */
@Composable
fun healthBandLabel(band: String): String = when (band) {
    "moss" -> stringResource(R.string.health_score_band_moss)
    "chanterelle" -> stringResource(R.string.health_score_band_chanterelle)
    "russula" -> stringResource(R.string.health_score_band_russula)
    else -> band
}

/**
 * Issue #1193: the color for a deceased contact's status dot, wherever a
 * health-band color would otherwise apply. Deliberately NOT one of the
 * moss/chanterelle/russula band colors above -- a deceased contact isn't a
 * health verdict, recency-of-interaction scoring is meaningless once someone
 * has died. `onSurfaceVariant` is this theme's existing muted-but-legible
 * role (the same one a band falls back to in [healthBandColor] when it
 * doesn't recognize the value), so this reads as "present but quiet" rather
 * than a fourth status color -- mirrors the web's `deceasedNodeColor`
 * (`text.secondary`) reasoning.
 */
@Composable
fun deceasedColor(): Color = MaterialTheme.colorScheme.onSurfaceVariant

/**
 * A compact colored indicator for a contact's server-computed relationship
 * health score/band (issue #383, ADR-0023). Never recomputed locally — the
 * score, band and every facet behind it come straight from
 * `GET /contacts/:id/score`.
 *
 * **Color-tint-only status is an accessibility trap** (`/CLAUDE.md`
 * `CadenceScreen` precedent: amber text on parchment was found under the
 * WCAG contrast threshold, so warning semantics ride the icon/dot tint, never
 * text color) — so the dot's color is purely decorative reinforcement and the
 * band name is always spelled out in words via [contentDescription], never
 * inferred from color alone.
 *
 * Tapping opens the facet breakdown ([onClick]); [testTag] defaults to
 * `"health-score-badge"` per the ticket's UI-test contract.
 */
@Composable
fun HealthScoreBadge(
    score: Int,
    band: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val color = healthBandColor(band)
    val bandLabel = healthBandLabel(band)
    val description = stringResource(R.string.health_score_badge_description, score, bandLabel)
    val actionLabel = stringResource(R.string.health_score_view_details)
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(50))
            .clickable(onClickLabel = actionLabel, onClick = onClick)
            .semantics(mergeDescendants = true) {
                this.contentDescription = description
                this.role = Role.Button
            }
            .padding(horizontal = 10.dp, vertical = 6.dp)
            .testTag("health-score-badge"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        HealthScoreDot(band = band)
        Text(
            text = score.toString(),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurface,
        )
    }
}

/**
 * The bare colored dot with no click target or text — for a compact list row
 * (e.g. `NetworkRow`) where the row itself carries the merged accessible
 * name/content description. Purely decorative (`contentDescription = null`);
 * the band name must be surfaced in words by the caller's own semantics, the
 * same rule [HealthScoreBadge] follows.
 */
@Composable
fun HealthScoreDot(band: String, modifier: Modifier = Modifier) {
    val color = healthBandColor(band)
    Box(
        modifier = modifier
            .size(10.dp)
            .clip(CircleShape)
            .background(color),
    )
}

/**
 * Issue #1193: [HealthScoreDot]'s deceased-state sibling -- same shape and
 * size, [deceasedColor] instead of a band color. Purely decorative
 * (`contentDescription = null`); the caller's own semantics must surface the
 * deceased state in words, the same rule [HealthScoreDot] follows.
 */
@Composable
fun DeceasedDot(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .size(10.dp)
            .clip(CircleShape)
            .background(deceasedColor()),
    )
}
