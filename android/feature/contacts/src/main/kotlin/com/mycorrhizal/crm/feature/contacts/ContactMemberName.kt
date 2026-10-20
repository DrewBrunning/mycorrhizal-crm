package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.ContactSummary

/**
 * The name to show for a resolved circle/tag/household member, or null when the
 * contact could not be resolved (callers then show a translated "Unknown
 * contact", never the raw vCard UID — issue #1768).
 *
 * M5 §3.2: uses the shared components-first derivation
 * (firstname "nickname" lastname) rather than a given-name-only `fn`.
 */
fun memberDisplayName(contact: ContactSummary?): String? {
    if (contact == null) return null
    return contact.displayName.takeIf { !it.startsWith("#") } ?: contact.nickname
}
