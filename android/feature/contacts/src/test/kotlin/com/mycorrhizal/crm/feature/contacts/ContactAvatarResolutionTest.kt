package com.mycorrhizal.crm.feature.contacts

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

// M5 §3.1: the avatar resolves the backend's relative profile-photo paths
// against the configured server origin so the shared OkHttp stack (BaseUrl +
// Auth interceptors) points them at the right server and attaches the JWT —
// and the resulting absolute URL, which is also Coil's disk-cache key, stays
// unique per server (a placeholder-keyed cache would serve one instance's
// avatar to another).
class ContactAvatarResolutionTest {

    @Test
    fun `a relative photo path is prefixed with the configured server origin`() {
        assertEquals(
            "https://crm.example.com/api/v1/contacts/5/profile_picture?thumbnail=true",
            resolvePhotoUri("/api/v1/contacts/5/profile_picture?thumbnail=true", "https://crm.example.com"),
        )
    }

    @Test
    fun `the resolved url differs per server, so cache keys cannot collide`() {
        val path = "/api/v1/contacts/5/profile_picture"
        assertEquals(
            "https://a.example.com$path",
            resolvePhotoUri(path, "https://a.example.com"),
        )
        assertEquals(
            "https://b.example.com$path",
            resolvePhotoUri(path, "https://b.example.com"),
        )
    }

    @Test
    fun `falls back to the placeholder origin when the server origin is unknown`() {
        // Previews/tests without a session: the interceptors still rewrite it.
        assertEquals(
            "http://mycorrhizal.invalid/api/v1/contacts/5/profile_picture",
            resolvePhotoUri("/api/v1/contacts/5/profile_picture", ""),
        )
    }

    @Test
    fun `an absolute http url passes through unchanged`() {
        assertEquals("https://cdn.example.com/pic.jpg", resolvePhotoUri("https://cdn.example.com/pic.jpg", "https://crm.example.com"))
    }

    @Test
    fun `a data uri passes through unchanged`() {
        assertEquals("data:image/png;base64,AAAA", resolvePhotoUri("data:image/png;base64,AAAA", "https://crm.example.com"))
    }

    @Test
    fun `blank or null input resolves to null`() {
        assertNull(resolvePhotoUri(null, "https://crm.example.com"))
        assertNull(resolvePhotoUri("", "https://crm.example.com"))
        assertNull(resolvePhotoUri("   ", "https://crm.example.com"))
    }
}
