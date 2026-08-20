package com.mycorrhizal.crm.testing.a11y

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.junit4.ComposeTestRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp

/**
 * Issue #214: the Compose analog of the web's axe-core route sweep
 * (`frontend/e2e/accessibility.spec.ts`). Walks the whole rendered semantics
 * tree of whatever `composeTestRule.setContent { ... }` last mounted — a
 * real top-level screen, in each per-feature sweep test — and asserts the
 * static a11y invariants a generic tree walk can check mechanically:
 *
 *  1. every clickable/long-clickable node resolves a non-blank accessible
 *     name (its own or a merged descendant's contentDescription/text),
 *  2. every icon-only clickable/long-clickable node (its accessible name
 *     comes from contentDescription, with no visible Text alongside it) has
 *     its own touch target >= 48dp on both axes (Material touch-target
 *     guidance; WCAG 2.5.8 Target Size (Minimum)) — this is #210's and this
 *     ticket's own icon-button class, and the one a tree walk can flag
 *     without a sweeping, purely-cosmetic app-wide rework: Material3's own
 *     default sizes for text-labeled controls (Button/TextButton's 40dp,
 *     chips' 32dp) sit under 48dp by design across virtually every screen in
 *     the app, so applying this check to every text-labeled control as well
 *     would flag the design system itself, not authoring mistakes. That
 *     systemic question is a separate, deliberate design-system decision to
 *     make once, not a per-screen fix this sweep makes — filed as #215
 *     rather than silently dropped,
 *  3. every long-clickable node exposes a non-blank `onLongClickLabel` (the
 *     #210 class — a long-press with no label is invisible to TalkBack's
 *     actions menu and unreachable by switch access) — except text fields
 *     (`SemanticsProperties.EditableText` present, editable or read-only:
 *     BasicTextField registers OnLongClick for cursor/selection handling
 *     either way), whose long-press is the platform's own text-selection
 *     affordance, not a custom gesture; accessibility services already
 *     understand that one natively,
 *  4. no two distinct nodes inside the same interactive node's subtree
 *     announce the identical non-blank text/contentDescription (the
 *     concrete "TalkBack reads it twice" bug — e.g. an icon whose
 *     contentDescription duplicates an adjacent Text). This is a
 *     deliberately narrower, mechanically-checkable slice of the issue's
 *     "no contentDescription on decorative-only nodes": whether an image is
 *     decorative is authorial intent a tree walk cannot know, so that
 *     general claim is NOT asserted here — only this concrete duplicate-
 *     announcement pattern is.
 *
 * Every violation is collected before failing — one `AssertionError` listing
 * all of them — mirroring `assertNoBlockingViolations`'s reporting style on
 * the web side rather than stopping at the first hit.
 *
 * Does *not* cover color contrast, heading structure, or focus order — those
 * are either theme-token concerns (pinned elsewhere) or need a real
 * accessibility-service integration test (`AccessibilityChecks.enable()`,
 * once instrumented UI tests exist) rather than a static tree walk.
 */
fun ComposeTestRule.assertAccessibleSemantics() {
    assertAccessibleSemantics(onRoot().fetchSemanticsNode())
}

/**
 * Issue #205: the Android Accessibility Test Framework's
 * `DuplicateSpeakableTextCheck` — no two focusable nodes on screen may share
 * the same contentDescription. A bare "Rename"/"Delete" on every row of a list
 * makes TalkBack announce "Rename, button" N times with nothing to say which
 * row each belongs to. Screens with per-row icon actions assert this with two
 * or more rows seeded.
 */
fun ComposeTestRule.assertNoDuplicateContentDescriptions() {
    assertNoDuplicateContentDescriptions(onRoot().fetchSemanticsNode())
}

fun assertNoDuplicateContentDescriptions(root: SemanticsNode) {
    val seen = mutableMapOf<String, SemanticsNode>()
    val violations = mutableListOf<String>()

    fun walk(node: SemanticsNode) {
        if (isInteractive(node)) {
            node.config.getOrNull(SemanticsProperties.ContentDescription)
                ?.filter { it.isNotBlank() }
                ?.forEach { cd ->
                    val prior = seen.putIfAbsent(cd, node)
                    if (prior != null && prior !== node) {
                        violations += "contentDescription \"$cd\" on ${describe(node)} " +
                            "duplicates ${describe(prior)}"
                    }
                }
        }
        node.children.forEach { walk(it) }
    }
    walk(root)

    if (violations.isNotEmpty()) {
        throw AssertionError(
            "Found ${violations.size} duplicate contentDescription(s):\n" +
                violations.joinToString("\n") { "  - $it" },
        )
    }
}

/** Material guidance / WCAG 2.5.8 Target Size (Minimum). */
private val MIN_TOUCH_TARGET = 48.dp

/** Slack for float px<->dp round-tripping; not a real size tolerance. */
private const val TOUCH_TARGET_EPSILON_PX = 0.5f

fun assertAccessibleSemantics(root: SemanticsNode) {
    val allNodes = mutableListOf<SemanticsNode>()
    collectAll(root, allNodes)

    val violations = mutableListOf<String>()

    for (node in allNodes) {
        if (!isInteractive(node)) continue

        val onLongClick = node.config.getOrNull(SemanticsActions.OnLongClick)
        val hasCD = hasContentDescription(node)
        val hasText = hasVisibleText(node)

        if (!hasCD && !hasText) {
            violations += "${describe(node)} is clickable/long-clickable but has no accessible " +
                "name (no contentDescription and no text)"
        }

        // See invariant 2's doc comment: only icon-only controls (name from
        // contentDescription, no visible Text) get the touch-target check.
        if (hasCD && !hasText) {
            val minPx = with(node.layoutInfo.density) { MIN_TOUCH_TARGET.toPx() }
            if (node.size.width < minPx - TOUCH_TARGET_EPSILON_PX ||
                node.size.height < minPx - TOUCH_TARGET_EPSILON_PX
            ) {
                val sizeDp = with(node.layoutInfo.density) {
                    "${node.size.width.toDp().value.toInt()}x${node.size.height.toDp().value.toInt()}dp"
                }
                violations += "${describe(node)} touch target is $sizeDp, smaller than the " +
                    "${MIN_TOUCH_TARGET.value.toInt()}dp minimum"
            }
        }

        val isTextField = node.config.contains(SemanticsProperties.EditableText)
        if (onLongClick != null && onLongClick.label.isNullOrBlank() && !isTextField) {
            violations += "${describe(node)} has an onLongClick action with no onLongClickLabel"
        }

        collectDuplicateAnnouncements(node, violations)
    }

    if (violations.isNotEmpty()) {
        throw AssertionError(
            "Found ${violations.size} accessibility violation(s):\n" +
                violations.joinToString("\n") { "  - $it" },
        )
    }
}

private fun collectAll(node: SemanticsNode, into: MutableList<SemanticsNode>) {
    into += node
    node.children.forEach { collectAll(it, into) }
}

private fun isInteractive(node: SemanticsNode): Boolean =
    node.config.getOrNull(SemanticsActions.OnClick) != null ||
        node.config.getOrNull(SemanticsActions.OnLongClick) != null

private fun hasContentDescription(node: SemanticsNode): Boolean =
    node.config.getOrNull(SemanticsProperties.ContentDescription)?.any { it.isNotBlank() } == true

private fun hasVisibleText(node: SemanticsNode): Boolean =
    node.config.getOrNull(SemanticsProperties.Text)?.any { it.text.isNotBlank() } == true

/**
 * Within [interactiveNode]'s subtree, flags any two distinct descendants
 * that contribute the identical non-blank contentDescription/text *to the
 * same announced name* — i.e. the "TalkBack reads it twice" bug where an
 * icon's contentDescription duplicates an adjacent Text merged into the same
 * node. Deliberately excludes nested interactive descendants (e.g. two
 * sibling `AssistChip`s that happen to share a label, such as two same-named
 * contacts tagged on one activity): each is independently
 * screen-reader-focusable and announced on its own turn, never combined into
 * [interactiveNode]'s own utterance (mirrors Compose's own merging algorithm,
 * which skips a mergeDescendants descendant for the same reason — see
 * `SemanticsNode.mergeConfig`), so two of them sharing text is not a
 * duplicate announcement at all.
 */
private fun collectDuplicateAnnouncements(interactiveNode: SemanticsNode, violations: MutableList<String>) {
    val contributions = mutableListOf<Pair<SemanticsNode, String>>()
    collectMergedTextContributions(interactiveNode, contributions, isRoot = true)

    val seen = mutableMapOf<String, SemanticsNode>()
    for ((contributor, string) in contributions) {
        val prior = seen.putIfAbsent(string, contributor)
        if (prior != null && prior !== contributor) {
            violations += "${describe(interactiveNode)} announces \"$string\" twice — from both " +
                "${describe(prior)} and ${describe(contributor)}"
        }
    }
}

/**
 * Collects contentDescription/text from descendants that are actually
 * merged into [node]'s own announced name — i.e. everything except nested
 * interactive descendants, which are never folded in (see the doc comment
 * on [collectDuplicateAnnouncements]) and are skipped entirely: neither
 * their own label nor anything further inside them counts toward [node]'s
 * duplicate-announcement check.
 */
private fun collectMergedTextContributions(
    node: SemanticsNode,
    into: MutableList<Pair<SemanticsNode, String>>,
    isRoot: Boolean,
) {
    if (!isRoot) {
        if (isInteractive(node)) return
        node.config.getOrNull(SemanticsProperties.ContentDescription)?.forEach {
            if (it.isNotBlank()) into += node to it.trim()
        }
        node.config.getOrNull(SemanticsProperties.Text)?.forEach {
            if (it.text.isNotBlank()) into += node to it.text.trim()
        }
    }
    node.children.forEach { collectMergedTextContributions(it, into, isRoot = false) }
}

private fun describe(node: SemanticsNode): String {
    val cd = node.config.getOrNull(SemanticsProperties.ContentDescription)?.joinToString()
    val text = node.config.getOrNull(SemanticsProperties.Text)?.joinToString { it.text }
    val label = cd ?: text
    return if (label != null) "node #${node.id} (\"$label\")" else "node #${node.id}"
}
