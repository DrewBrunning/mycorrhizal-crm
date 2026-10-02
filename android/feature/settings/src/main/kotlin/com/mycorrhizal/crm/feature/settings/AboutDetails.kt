package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.about.AppBuildInfo
import com.mycorrhizal.crm.model.network.ServerHealth

/**
 * Issue #1420: the plain-text build details put on the clipboard for a bug
 * report. Mirrors the web About card's `version: … / commit: … / built: …`
 * shape; optional lines are omitted, never filled with a placeholder.
 */
internal object AboutDetails {

    /** "1.3.0 (abc1234)", or the bare version when there is no commit. */
    fun versionWithCommit(version: String, commit: String?): String =
        if (commit.isNullOrBlank()) version else "$version ($commit)"

    /** "release / obtainium" plus the applicationId suffix when present. */
    fun buildLabel(info: AppBuildInfo): String {
        val base = if (info.flavor.isBlank()) info.buildType else "${info.buildType} / ${info.flavor}"
        return info.applicationIdSuffix?.let { "$base $it" } ?: base
    }

    fun format(info: AppBuildInfo, server: ServerHealth?): String = buildList {
        add("version: ${info.versionName}")
        add("code: ${info.versionCode}")
        add("build: ${buildLabel(info)}")
        info.commit?.takeIf { it.isNotBlank() }?.let { add("commit: $it") }
        info.buildDate?.takeIf { it.isNotBlank() }?.let { add("built: $it") }
        server?.version?.takeIf { it.isNotBlank() }?.let { add("server: ${versionWithCommit(it, server.commit)}") }
        server?.buildDate?.takeIf { it.isNotBlank() }?.let { add("server built: $it") }
    }.joinToString("\n")
}
