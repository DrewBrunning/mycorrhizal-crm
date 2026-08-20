package com.mycorrhizal.crm.feature.contacts

/**
 * Issue #236: small path-string helpers shared by the Seafile/Nextcloud file
 * pickers' ViewModel navigation (`ContactDetailViewModel.enterSeafileDir`/
 * `backSeafileDir`/`linkSeafileItem`) and their breadcrumb composables. Paths
 * are always `/`-rooted, mirroring the backend's WebDAV/Seafile path shape
 * (`SeafileItem.parent_dir`, `WebDAVItem.path`) and web's own `joinDirPath`/
 * `parentDir` in `SeafileFilePickerDialog.tsx`/`NextcloudFilePickerDialog.tsx`.
 */

/** Joins a directory path and a child name into a single `/`-rooted path, avoiding "//". */
internal fun joinDirPath(dir: String, name: String): String {
    val base = dir.trimEnd('/')
    return if (base.isEmpty()) "/$name" else "$base/$name"
}

/** The parent of [path], or null when [path] is already the root ("/"). */
internal fun parentDirPath(path: String): String? {
    val trimmed = path.trim('/')
    if (trimmed.isEmpty()) return null
    val segments = trimmed.split('/').dropLast(1)
    return if (segments.isEmpty()) "/" else "/" + segments.joinToString("/")
}

/** The path's `/`-separated breadcrumb segments (empty for the root). */
internal fun pathSegments(path: String): List<String> =
    path.split('/').filter { it.isNotBlank() }
