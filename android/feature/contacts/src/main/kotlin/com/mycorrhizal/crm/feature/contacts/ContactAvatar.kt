package com.mycorrhizal.crm.feature.contacts

import android.graphics.BitmapFactory
import android.util.Base64
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.painter.ColorPainter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.Dp
import coil3.compose.AsyncImage

/**
 * A contact's circular avatar. Renders a photo from:
 *  - a `data:` URI (the server's current wire format for thumbnails), decoded
 *    directly to a Bitmap — the reliable path on every device/Coil version;
 *  - an http(s) URL (the planned profile-picture URL from ticket 82) via Coil;
 *  - the person icon fallback when neither is present.
 *
 * Handles both the list's flat `photoThumbnail` and the detail's
 * `card.media[].uri` (kind=photo), which the detail endpoint guarantees via
 * buildMedia.
 */
@Composable
fun ContactAvatar(
    photoUri: String?,
    contentDescription: String?,
    size: Dp,
    modifier: Modifier = Modifier,
) {
    val uri = photoUri?.trim()?.takeIf { it.isNotEmpty() }
    val isHttp = uri != null && uri.startsWith("http")
    val dataBitmap = remember(uri) {
        if (uri != null && uri.startsWith("data:")) decodeDataUri(uri) else null
    }
    Box(
        modifier = modifier.size(size),
        contentAlignment = Alignment.Center,
    ) {
        val imageBitmap = dataBitmap
        when {
            imageBitmap != null -> {
                Image(
                    bitmap = imageBitmap,
                    contentDescription = contentDescription,
                    contentScale = ContentScale.Crop,
                    modifier = Modifier.size(size).clip(CircleShape),
                )
            }
            isHttp -> {
                AsyncImage(
                    model = uri,
                    contentDescription = contentDescription,
                    contentScale = ContentScale.Crop,
                    modifier = Modifier.size(size).clip(CircleShape),
                )
            }
            else -> PersonFallback(size)
        }
    }
}

/** Decodes a `data:image/...;base64,<data>` URI into an [ImageBitmap], or null. */
private fun decodeDataUri(uri: String): ImageBitmap? {
    val comma = uri.indexOf(',')
    if (comma < 0) return null
    val base64 = uri.substring(comma + 1)
    return try {
        val bytes = Base64.decode(base64, Base64.DEFAULT)
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
    } catch (_: Exception) {
        null
    }
}

@Composable
private fun PersonFallback(size: Dp) {
    Box(
        modifier = Modifier.size(size).clip(CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Image(
            painter = ColorPainter(MaterialTheme.colorScheme.surfaceVariant),
            contentDescription = null,
            modifier = Modifier.matchParentSize(),
        )
        Icon(
            imageVector = Icons.Outlined.Person,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(size * 0.6f),
        )
    }
}
