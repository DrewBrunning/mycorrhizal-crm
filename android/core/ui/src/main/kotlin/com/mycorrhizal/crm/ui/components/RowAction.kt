package com.mycorrhizal.crm.ui.components

/**
 * One entry of a [RowOverflowMenu].
 *
 * Lives in its own file (not RowOverflowMenu.kt) so detekt's
 * MatchingDeclarationName rule — which only counts top-level class/object
 * declarations, and so would otherwise require the file to be named after
 * this data class — stays satisfied.
 */
data class RowAction(
    val label: String,
    val onClick: () -> Unit,
    /** Draw the label in the error colour (destructive actions). */
    val destructive: Boolean = false,
)
