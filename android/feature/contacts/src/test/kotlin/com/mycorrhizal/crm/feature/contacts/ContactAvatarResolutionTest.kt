package com.mycorrhizal.crm.feature.contacts

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

// M5 §3.1: the avatar resolves the backend's relative profile-photo paths
// against the placeholder origin so the shared OkHttp stack (BaseUrl + Auth
// interceptors) rewrites them onto the server and attaches the JWT.
class ContactAvatarResolutionTest {

    @Test
    fun `a relative photo path is prefixed with the placeholder origin`() {
        assertEquals(
            "http://mycorrhizal.invalid/api/v1/contacts/5/profile_picture?thumbnail=true",
            resolvePhotoUri("/api/v1/contacts/5/profile_picture?thumbnail=true"),
        )
    }

    @Test
    fun `an absolute http url passes through unchanged`() {
        assertEquals("https://cdn.example.com/pic.jpg", resolvePhotoUri("https://cdn.example.com/pic.jpg"))
    }

    @Test
    fun `a data uri passes through unchanged`() {
        assertEquals("data:image/png;base64,AAAA", resolvePhotoUri("data:image/png;base64,AAAA"))
    }

    @Test
    fun `blank or null input resolves to null`() {
        assertNull(resolvePhotoUri(null))
        assertNull(resolvePhotoUri(""))
        assertNull(resolvePhotoUri("   "))
    }
}
