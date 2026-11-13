package com.mycorrhizal.crm.feature.contacts.entities

import com.mycorrhizal.crm.model.network.EntityMergeInput

/**
 * The merge request both entry points send (the entity page and a duplicate suggestion, #2520):
 * merge into [survivorId], filling only the [fill] fields from the loser (ADR 0043 decision 8 -
 * nothing is filled unless listed).
 */
internal fun entityMergeInput(survivorId: String, fill: Set<String>) = EntityMergeInput(
    into = survivorId,
    resolutions = fill.associateWith { "loser" }.ifEmpty { null }
)
