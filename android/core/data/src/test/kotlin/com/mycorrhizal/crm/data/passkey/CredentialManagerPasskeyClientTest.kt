package com.mycorrhizal.crm.data.passkey

import androidx.credentials.exceptions.CreateCredentialCancellationException
import androidx.credentials.exceptions.CreateCredentialCustomException
import androidx.credentials.exceptions.CreateCredentialInterruptedException
import androidx.credentials.exceptions.CreateCredentialNoCreateOptionException
import androidx.credentials.exceptions.CreateCredentialProviderConfigurationException
import androidx.credentials.exceptions.CreateCredentialUnknownException
import androidx.credentials.exceptions.CreateCredentialUnsupportedException
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialInterruptedException
import androidx.credentials.exceptions.GetCredentialProviderConfigurationException
import androidx.credentials.exceptions.GetCredentialUnknownException
import androidx.credentials.exceptions.GetCredentialUnsupportedException
import androidx.credentials.exceptions.NoCredentialException
import androidx.credentials.exceptions.domerrors.AbortError
import androidx.credentials.exceptions.domerrors.DataError
import androidx.credentials.exceptions.domerrors.InvalidStateError
import androidx.credentials.exceptions.domerrors.NotAllowedError
import androidx.credentials.exceptions.domerrors.SecurityError
import androidx.credentials.exceptions.publickeycredential.CreatePublicKeyCredentialDomException
import androidx.credentials.exceptions.publickeycredential.GetPublicKeyCredentialDomException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1293 / ADR 0034: the pure decisions around the untestable Credential
 * Manager calls — envelope unwrap, device-support rule, and the failure taxonomy
 * (cancel = silent, no provider = degraded, not associated = distinct state).
 */
class CredentialManagerPasskeyClientTest {

    // --- envelope unwrap -------------------------------------------------------

    @Test
    fun `the publicKey member is extracted verbatim`() {
        val envelope = """{"publicKey":{"challenge":"abc","timeout":60000,"rp":{"id":"crm.example.com","name":"M"}}}"""

        assertEquals(
            """{"challenge":"abc","timeout":60000,"rp":{"id":"crm.example.com","name":"M"}}""",
            unwrapPublicKeyOptions(envelope),
        )
    }

    @Test
    fun `other members are ignored and order does not matter`() {
        assertEquals(
            """{"challenge":"x"}""",
            unwrapPublicKeyOptions("""{"mediation":"optional","publicKey":{"challenge":"x"},"extra":[1,2]}"""),
        )
    }

    @Test
    fun `numbers are not re-encoded`() {
        assertEquals("""{"timeout":300000}""", unwrapPublicKeyOptions("""{"publicKey":{"timeout":300000}}"""))
    }

    @Test
    fun `a body without a publicKey member is returned unchanged`() {
        val bare = """{"challenge":"x","rp":{"id":"a.b"}}"""
        assertEquals(bare, unwrapPublicKeyOptions(bare))
    }

    @Test
    fun `non-object and malformed bodies are returned unchanged`() {
        assertEquals("[1,2]", unwrapPublicKeyOptions("[1,2]"))
        assertEquals("not json", unwrapPublicKeyOptions("not json"))
        assertEquals("", unwrapPublicKeyOptions(""))
        assertEquals("""{"publicKey":""", unwrapPublicKeyOptions("""{"publicKey":"""))
    }

    // --- device support ---------------------------------------------------------

    @Test
    fun `Android 14 and later always ships Credential Manager`() {
        assertTrue(passkeysSupportedOn(34, playProviderBundled = false))
        assertTrue(passkeysSupportedOn(36, playProviderBundled = false))
    }

    @Test
    fun `older Android needs the bundled Play services provider - the foss flavor has none`() {
        assertFalse(passkeysSupportedOn(33, playProviderBundled = false))
        assertFalse(passkeysSupportedOn(26, playProviderBundled = false))
        assertTrue(passkeysSupportedOn(33, playProviderBundled = true))
        assertTrue(passkeysSupportedOn(26, playProviderBundled = true))
    }

    // --- registration failures --------------------------------------------------

    @Test
    fun `create - a user cancel is silent`() {
        assertEquals(PasskeyResult.Cancelled, classifyCreateFailure(CreateCredentialCancellationException()))
        assertEquals(
            PasskeyResult.Cancelled,
            classifyCreateFailure(CreatePublicKeyCredentialDomException(NotAllowedError(), "declined")),
        )
        assertEquals(
            PasskeyResult.Cancelled,
            classifyCreateFailure(CreatePublicKeyCredentialDomException(AbortError(), "aborted")),
        )
    }

    @Test
    fun `create - no provider or unsupported device is the degraded state`() {
        assertEquals(PasskeyResult.NoProvider, classifyCreateFailure(CreateCredentialProviderConfigurationException()))
        assertEquals(PasskeyResult.NoProvider, classifyCreateFailure(CreateCredentialUnsupportedException()))
        assertEquals(PasskeyResult.NoProvider, classifyCreateFailure(CreateCredentialNoCreateOptionException()))
    }

    @Test
    fun `create - the app-not-associated DOM class is its own state`() {
        assertEquals(
            PasskeyResult.NotAssociated,
            classifyCreateFailure(CreatePublicKeyCredentialDomException(SecurityError(), "The incoming request cannot be validated")),
        )
        // A NotAllowedError that says the request cannot be validated is association, not a user cancel.
        assertEquals(
            PasskeyResult.NotAssociated,
            classifyCreateFailure(
                CreatePublicKeyCredentialDomException(NotAllowedError(), "The incoming request cannot be validated"),
            ),
        )
        assertEquals(
            PasskeyResult.NotAssociated,
            classifyCreateFailure(CreatePublicKeyCredentialDomException(DataError(), "app is not associated with rp id")),
        )
    }

    @Test
    fun `create - an excluded credential is reported as already registered`() {
        assertEquals(
            PasskeyResult.AlreadyRegistered,
            classifyCreateFailure(CreatePublicKeyCredentialDomException(InvalidStateError(), "exists")),
        )
    }

    @Test
    fun `create - everything else is a generic failure`() {
        assertTrue(classifyCreateFailure(CreateCredentialUnknownException()) is PasskeyResult.Failed)
        assertTrue(classifyCreateFailure(CreateCredentialInterruptedException()) is PasskeyResult.Failed)
        assertTrue(classifyCreateFailure(CreateCredentialCustomException("t", "m")) is PasskeyResult.Failed)
        assertTrue(
            classifyCreateFailure(CreatePublicKeyCredentialDomException(DataError(), "bad data")) is PasskeyResult.Failed,
        )
    }

    // --- assertion failures -----------------------------------------------------

    @Test
    fun `get - a user cancel is silent`() {
        assertEquals(PasskeyResult.Cancelled, classifyGetFailure(GetCredentialCancellationException()))
        assertEquals(
            PasskeyResult.Cancelled,
            classifyGetFailure(GetPublicKeyCredentialDomException(NotAllowedError(), "timed out")),
        )
        assertEquals(
            PasskeyResult.Cancelled,
            classifyGetFailure(GetPublicKeyCredentialDomException(AbortError(), "aborted")),
        )
    }

    @Test
    fun `get - no passkey on this device is distinct from a provider problem`() {
        assertEquals(PasskeyResult.NoMatchingPasskey, classifyGetFailure(NoCredentialException()))
        assertEquals(PasskeyResult.NoProvider, classifyGetFailure(GetCredentialProviderConfigurationException()))
        assertEquals(PasskeyResult.NoProvider, classifyGetFailure(GetCredentialUnsupportedException()))
    }

    @Test
    fun `get - the app-not-associated DOM class is its own state`() {
        assertEquals(
            PasskeyResult.NotAssociated,
            classifyGetFailure(GetPublicKeyCredentialDomException(SecurityError(), "The incoming request cannot be validated")),
        )
        assertEquals(
            PasskeyResult.NotAssociated,
            classifyGetFailure(GetPublicKeyCredentialDomException(NotAllowedError(), "Unable to verify asset links")),
        )
    }

    @Test
    fun `get - everything else is a generic failure`() {
        assertTrue(classifyGetFailure(GetCredentialUnknownException()) is PasskeyResult.Failed)
        assertTrue(classifyGetFailure(GetCredentialInterruptedException()) is PasskeyResult.Failed)
        assertTrue(classifyGetFailure(GetPublicKeyCredentialDomException(DataError(), "bad")) is PasskeyResult.Failed)
    }
}
