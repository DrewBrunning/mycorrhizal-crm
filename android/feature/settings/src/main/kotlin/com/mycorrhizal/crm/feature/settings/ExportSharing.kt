package com.mycorrhizal.crm.feature.settings

import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import com.mycorrhizal.crm.ui.R
import java.io.File

/**
 * Writes a finished dataset export to the cache and hands it to the share
 * sheet via FileProvider, so the user can save it (Files) or send it
 * anywhere. Shared by [DataScreen] (issue #710) and [CustomExportScreen]
 * (issue #835) — both produce the same [DataExport] shape.
 */
internal fun shareExportFile(context: Context, export: DataExport) {
    val dir = File(context.cacheDir, "exports").apply { mkdirs() }
    val file = File(dir, export.fileName)
    try {
        file.writeBytes(export.bytes)
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
        val intent = Intent(Intent.ACTION_SEND).apply {
            type = export.mimeType
            putExtra(Intent.EXTRA_STREAM, uri)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        context.startActivity(
            Intent.createChooser(intent, context.getString(R.string.data_export_share_title)),
        )
    } catch (_: Exception) {
        // No activity can handle the share — nothing to do.
    }
}
