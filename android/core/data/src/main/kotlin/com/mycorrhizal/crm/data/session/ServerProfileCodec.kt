package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import java.net.URLDecoder
import java.net.URLEncoder

/**
 * Serializes [ProfilesSnapshot] to/from the single DataStore string slot ADR
 * 0028 Decision 1 calls for. Deliberately a tiny hand-rolled tab/newline codec
 * (with URL-encoded URL/label fields) rather than a general JSON binding: the
 * shape is fixed and the parser must be trivially unit-testable on the JVM.
 *
 * A row is `id \t kind \t encodedUrl \t encodedLabel [\t archived]`; the fifth
 * column (`1` = read-only archive, issue #1265) is absent on rows written before
 * it existed and decodes as not archived. Malformed rows are
 * dropped rather than throwing — a corrupt prefs entry must not brick startup
 * (ADR-0002: degrade, don't crash); at worst the user re-adds a profile.
 */
internal object ServerProfileCodec {

    private const val KIND_REMOTE = "remote"
    private const val KIND_LOCAL = "local"
    private const val ARCHIVED_FLAG = "1"

    fun encode(snapshot: ProfilesSnapshot): String =
        snapshot.profiles.joinToString("\n") { profile ->
            val kind = when (profile.kind) {
                is ServerProfileKind.Remote -> KIND_REMOTE
                ServerProfileKind.Local -> KIND_LOCAL
            }
            val url = profile.remoteUrl.orEmpty()
            val columns = listOf(profile.id, kind, enc(url), enc(profile.label))
            (if (profile.archived) columns + ARCHIVED_FLAG else columns).joinToString("\t")
        }

    fun decode(raw: String?, activeId: String?): ProfilesSnapshot {
        val profiles = raw
            ?.lineSequence()
            ?.mapNotNull { decodeRow(it) }
            ?.toList()
            .orEmpty()
        val resolvedActive = activeId?.takeIf { id -> profiles.any { it.id == id } }
            ?: profiles.firstOrNull()?.id
        return ProfilesSnapshot(profiles = profiles, activeProfileId = resolvedActive)
    }

    private fun decodeRow(row: String): ServerProfile? {
        if (row.isBlank()) return null
        val parts = row.split('\t')
        if (parts.size != 4 && parts.size != 5) return null
        val id = parts[0].takeIf { it.isNotBlank() } ?: return null
        val label = dec(parts[3])
        val kind = when (parts[1]) {
            KIND_REMOTE -> ServerProfileKind.Remote(dec(parts[2]))
            KIND_LOCAL -> ServerProfileKind.Local
            else -> return null
        }
        return ServerProfile(id = id, kind = kind, label = label, archived = parts.getOrNull(4) == ARCHIVED_FLAG)
    }

    private fun enc(value: String): String = URLEncoder.encode(value, Charsets.UTF_8.name())

    private fun dec(value: String): String = runCatching {
        URLDecoder.decode(value, Charsets.UTF_8.name())
    }.getOrDefault(value)
}
