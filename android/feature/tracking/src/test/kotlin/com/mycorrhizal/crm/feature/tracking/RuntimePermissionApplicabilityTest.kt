package com.mycorrhizal.crm.feature.tracking

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1919: POST_NOTIFICATIONS only exists as a runtime grant from API 33.
 * Below the floor the checker must treat it as already satisfied rather than
 * returning DENIED for a permission the platform does not know — otherwise the
 * "Show notifications" toggle would ask for a permission that cannot be
 * granted and stay permanently off on API 26–32.
 */
class RuntimePermissionApplicabilityTest {

    @Test
    fun `POST_NOTIFICATIONS is not applicable below API 33`() {
        assertFalse(isRuntimePermissionApplicable(TrackingPermissions.POST_NOTIFICATIONS, 26))
        assertFalse(isRuntimePermissionApplicable(TrackingPermissions.POST_NOTIFICATIONS, 32))
    }

    @Test
    fun `POST_NOTIFICATIONS is applicable from API 33`() {
        assertTrue(isRuntimePermissionApplicable(TrackingPermissions.POST_NOTIFICATIONS, 33))
        assertTrue(isRuntimePermissionApplicable(TrackingPermissions.POST_NOTIFICATIONS, 37))
    }

    @Test
    fun `the other capture permissions are unaffected by the version gate`() {
        (TrackingPermissions.CALL_TRACKING + TrackingPermissions.SMS_TRACKING).forEach { permission ->
            assertTrue(isRuntimePermissionApplicable(permission, 26))
            assertTrue(isRuntimePermissionApplicable(permission, 37))
        }
    }
}
