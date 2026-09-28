package com.mycorrhizal.crm.domain.profile

/**
 * ADR 0028 Decision 1: the app talks to exactly one *server profile* at a time.
 * A profile is either a [Remote] server (today's only shipping kind, identified
 * by its URL) or a [Local] on-device store (the embedded backend, issue #1262 —
 * modelled now so switching and storage are built around the distinction, but
 * not offered in the UI until that host lands).
 */
sealed interface ServerProfileKind {
    /** A profile backed by a remote Mycorrhizal server at [url]. */
    data class Remote(val url: String) : ServerProfileKind

    /**
     * A profile backed by the on-device embedded server (ADR 0028 Decision 2).
     * It has no user-entered URL: its transport is the app-private Unix socket.
     */
    data object Local : ServerProfileKind
}

/**
 * One configured server the user can switch to. [id] is a stable UUID generated
 * at creation; credentials are keyed by it (see `TokenStorage` and siblings), so
 * the ID must never be reused or change.
 */
data class ServerProfile(
    val id: String,
    val kind: ServerProfileKind,
    val label: String,
) {
    /** The remote origin, or null for a [ServerProfileKind.Local] profile. */
    val remoteUrl: String? get() = (kind as? ServerProfileKind.Remote)?.url
}

/**
 * The label a profile gets when one is created implicitly — a migrated legacy
 * install, or a login typed straight into the Auth screen. Deliberately derived
 * from the host (readable and recognisable in a list) rather than the raw URL.
 */
fun defaultProfileLabel(url: String): String {
    val host = url
        .substringAfter("://", url)
        .substringBefore('/')
        .substringBefore('?')
        .substringBefore('#')
    return host.ifBlank { url }.ifBlank { "Server" }
}
