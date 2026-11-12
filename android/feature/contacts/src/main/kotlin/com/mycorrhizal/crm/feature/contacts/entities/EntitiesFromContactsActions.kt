package com.mycorrhizal.crm.feature.contacts.entities

/** Everything the content can ask for; the screen wires it to the view model. */
internal data class EntitiesFromContactsActions(
    val onSelectKind: (String) -> Unit = {},
    val onSelected: (key: String, selected: Boolean) -> Unit = { _, _ -> },
    val onName: (key: String, name: String) -> Unit = { _, _ -> },
    val onRole: (key: String, role: String) -> Unit = { _, _ -> },
    val onPrimary: (key: String, primary: Boolean) -> Unit = { _, _ -> },
    val onContact: (key: String, contactId: Int, included: Boolean) -> Unit = { _, _, _ -> },
    val onCreate: () -> Unit = {},
    val onRetry: () -> Unit = {},
    val onLoadMore: () -> Unit = {},
    val onDismissResult: () -> Unit = {}
)
