package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.painter.ColorPainter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.Dp
import coil3.compose.SubcomposeAsyncImage

/**
 * A contact's circular avatar. Renders a photo from:
 *  - a `data:` URI (the server's current wire format for thumbnails), or
 *  - an http(s) URL (the planned profile-picture URL from ticket 82), or
 *  - the person icon fallback when neither is present.
 *
 * Handles both the list's flat `photoThumbnail` and the detail's
 * `card.media[].uri` (kind=photo), which the detail endpoint guarantees via
 * buildMedia. Coil loads `data:` URIs natively.
 */
@Composable
fun ContactAvatar(
    photoUri: String?,
    contentDescription: String?,
    size: Dp,
    modifier: Modifier = Modifier,
) {
    val uri = photoUri?.trim()?.takeIf { it.isNotEmpty() }
    val isLoadable = uri != null && (uri.startsWith("data:") || uri.startsWith("http"))
    Box(
        modifier = modifier.size(size),
        contentAlignment = Alignment.Center,
    ) {
        if (isLoadable) {
            SubcomposeAsyncImage(
                model = uri,
                contentDescription = contentDescription,
                contentScale = ContentScale.Crop,
                modifier = Modifier.size(size).clip(CircleShape),
                loading = {
                    Box(Modifier.size(size).clip(CircleShape)) {
                        Image(
                            painter = ColorPainter(MaterialTheme.colorScheme.surfaceVariant),
                            contentDescription = null,
                            modifier = Modifier.matchParentSize(),
                        )
                    }
                },
                error = {
                    PersonFallback(size)
                },
            )
        } else {
            PersonFallback(size)
        }
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
