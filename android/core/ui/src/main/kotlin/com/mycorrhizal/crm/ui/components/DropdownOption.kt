package com.mycorrhizal.crm.ui.components

/**
 * One choice of a [LabelledDropdown]. [sectionHeader], when set, draws a small
 * heading above this entry (the preference-category menu groups by section).
 */
data class DropdownOption<T>(
    val value: T,
    val label: String,
    val sectionHeader: String? = null,
)
