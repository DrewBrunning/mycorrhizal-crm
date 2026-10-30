package com.mycorrhizal.crm.model.network

import com.squareup.moshi.JsonAdapter
import com.squareup.moshi.JsonReader
import com.squareup.moshi.JsonWriter
import com.squareup.moshi.Moshi
import java.lang.reflect.Type

/**
 * Issue #2006: writes [ActivityInput.endDate] as an explicit `"end_date": null` when it is unset.
 *
 * Moshi omits nulls by default, but on `PUT /activities/{id}` an omitted `end_date` means "keep
 * the stored value" while `null` means "clear it", so an editor that blanked the field must send
 * the null. Every other field keeps the default omit-when-null behaviour. Registered by
 * [com.mycorrhizal.crm.model.MoshiProvider]; a Moshi without it still works, it just never
 * clears.
 */
object ActivityInputAdapterFactory : JsonAdapter.Factory {
    override fun create(type: Type, annotations: Set<Annotation>, moshi: Moshi): JsonAdapter<*>? {
        if (type != ActivityInput::class.java || annotations.isNotEmpty()) return null
        val delegate = moshi.nextAdapter<ActivityInput>(this, type, annotations)
        val mapAdapter = moshi.adapter(Any::class.java)
        return object : JsonAdapter<ActivityInput>() {
            override fun fromJson(reader: JsonReader): ActivityInput? = delegate.fromJson(reader)

            override fun toJson(writer: JsonWriter, value: ActivityInput?) {
                if (value == null) {
                    delegate.toJson(writer, null)
                    return
                }
                val fields = LinkedHashMap<String, Any?>()
                (delegate.toJsonValue(value) as? Map<*, *>)?.forEach { (k, v) ->
                    fields[k as String] = v
                }
                fields["end_date"] = value.endDate
                val previous = writer.serializeNulls
                writer.serializeNulls = true
                try {
                    mapAdapter.toJson(writer, fields)
                } finally {
                    writer.serializeNulls = previous
                }
            }
        }
    }
}
