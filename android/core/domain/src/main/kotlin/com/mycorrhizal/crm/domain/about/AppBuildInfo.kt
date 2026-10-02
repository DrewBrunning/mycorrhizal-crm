package com.mycorrhizal.crm.domain.about

/**
 * Identity of the running app build (issue #1420), supplied by `:app` from its
 * `BuildConfig` — feature modules cannot see another module's BuildConfig.
 *
 * [versionCode] is the only value that tells release candidates apart:
 * `docker-publish.yml` strips `-rc.N` from [versionName], so `rc.2` and the
 * final release report the same name.
 *
 * [commit] and [buildDate] are blank when the build was not stamped (local
 * builds); they are normalised to null so the UI shows nothing rather than a
 * placeholder.
 */
data class AppBuildInfo(
    val versionName: String,
    val versionCode: Int,
    val buildType: String,
    val flavor: String,
    val applicationId: String,
    val commit: String? = null,
    val buildDate: String? = null,
) {
    /**
     * The applicationId suffix beyond the shipping id (e.g. `.localtest`), or
     * null for the real release id — makes a side-by-side debug build visibly
     * not the release.
     */
    val applicationIdSuffix: String?
        get() = applicationId.removePrefix(BASE_APPLICATION_ID)
            .takeIf { it.isNotEmpty() && applicationId.startsWith(BASE_APPLICATION_ID) }

    companion object {
        const val BASE_APPLICATION_ID = "com.mycorrhizal.crm"
    }
}
