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
import com.mycorrhizal.crm.ui.LocalServerUrl

/**
 * A contact's circular avatar. Renders a photo from:
 *  - a `data:` URI (the server's current wire format for thumbnails), decoded
 *    directly to a Bitmap — the reliable path on every device/Coil version;
 *  - a relative path (the M6 profile-picture URL, e.g.
 *    `/api/v1/contacts/{id}/profile_picture?thumbnail=true`), resolved against
 *    the placeholder origin so the shared OkHttp stack's BaseUrlInterceptor
 *    rewrites it onto the configured server and AuthInterceptor attaches the
 *    JWT — the ImageLoader is wired to that same client (M5 §3.1);
 *  - an absolute http(s) URL via Coil;
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
    val serverOrigin = LocalServerUrl.current
    val uri = photoUri?.trim()?.takeIf { it.isNotEmpty() }
    val resolvedUri = resolvePhotoUri(uri, serverOrigin)
    val isHttp = resolvedUri != null && resolvedUri.startsWith("http")
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
                    model = resolvedUri,
                    contentDescription = contentDescription,
                    contentScale = ContentScale.Crop,
                    modifier = Modifier.size(size).clip(CircleShape),
                )
            }
            else -> PersonFallback(size)
        }
    }
}

/**
 * Resolves a photo URI for the image loader (M5 §3.1): relative profile-photo
 * paths (the M6 wire format) are prefixed with the configured [serverOrigin]
 * when known, else the placeholder origin — in both cases the shared OkHttp
 * stack's BaseUrlInterceptor points the request at the configured server and
 * the AuthInterceptor attaches the JWT. Resolving to the REAL origin keeps the
 * Coil disk-cache key per-server (a placeholder-keyed cache would serve one
 * instance's avatar to another). Absolute http(s) URLs and `data:` URIs pass
 * through untouched; blank input resolves to null.
 */
internal fun resolvePhotoUri(uri: String?, serverOrigin: String): String? {
    if (uri == null) return null
    val trimmed = uri.trim()
    if (trimmed.isEmpty()) return null
    return if (trimmed.startsWith("/")) {
        val origin = serverOrigin.trim().trimEnd('/').takeIf { it.isNotBlank() }
            ?: com.mycorrhizal.crm.network.ApiClient.PLACEHOLDER_ORIGIN
        origin + trimmed
    } else {
        trimmed
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
