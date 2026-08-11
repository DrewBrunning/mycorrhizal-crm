package com.mycorrhizal.crm.model

import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory

/**
 * Central Moshi instance for the app.
 *
 * Every `@JsonClass(generateAdapter = true)` model gets a codegen adapter
 * (moshi-kotlin-codegen via KSP). [KotlinJsonAdapterFactory] — the
 * library-sanctioned integration — discovers those generated adapters
 * (ticket §1.3: reflection-free at runtime for annotated models, R8-friendly)
 * and applies the nullSafe wrapping the generated code relies on. Add a new
 * model to this project and its adapter is found automatically.
 */
object MoshiProvider {

    @Volatile
    private var instance: Moshi? = null

    fun get(): Moshi = instance ?: synchronized(this) {
        instance ?: Moshi.Builder()
            .add(KotlinJsonAdapterFactory())
            .build()
            .also { instance = it }
    }
}
