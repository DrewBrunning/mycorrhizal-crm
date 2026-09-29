package com.mycorrhizal.crm.testing

import android.content.Context
import com.mycorrhizal.crm.data.passkey.PasskeyCredentialClient
import com.mycorrhizal.crm.data.passkey.PasskeyResult

/**
 * Issue #1293: a [PasskeyCredentialClient] that never touches the platform, so
 * every passkey code path is exercised on the JVM. A real ceremony needs a
 * device, a provider and a public HTTPS server and is never attempted in a unit test.
 */
class FakePasskeyClient(var supported: Boolean = true) : PasskeyCredentialClient {
    val created = mutableListOf<String>()
    val requested = mutableListOf<String>()
    var createResult: PasskeyResult = PasskeyResult.Success("""{"id":"new"}""")
    var getResult: PasskeyResult = PasskeyResult.Success("""{"id":"asserted"}""")

    override fun isSupported() = supported

    override suspend fun createPasskey(context: Context, optionsJson: String): PasskeyResult {
        created += optionsJson
        return createResult
    }

    override suspend fun getPasskey(context: Context, optionsJson: String): PasskeyResult {
        requested += optionsJson
        return getResult
    }
}
