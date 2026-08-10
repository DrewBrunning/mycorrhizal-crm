package com.mycorrhizal.crm.data.local

import androidx.room.ProvidedTypeConverter
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import kotlinx.datetime.Instant
import kotlinx.datetime.LocalDate
import kotlinx.datetime.TimeZone
import kotlinx.datetime.toLocalDateTime

/**
 * Room stores only primitives; complex fields need adapters. The cache is
 * read-only mirror data, so we keep it simple: JSON-encode composite values
 * to TEXT, dates to epoch millis / ISO strings.
 */
@ProvidedTypeConverter
class Converters(private val moshi: Moshi) {
    @androidx.room.TypeConverter
    fun fromStringList(value: List<String>?): String? =
        value?.let { moshi.adapter<List<String>>(STRING_LIST_TYPE).toJson(it) }

    @androidx.room.TypeConverter
    fun toStringList(value: String?): List<String>? =
        value?.let { moshi.adapter<List<String>>(STRING_LIST_TYPE).fromJson(it) }

    @androidx.room.TypeConverter
    fun fromCard(value: Card?): String? =
        value?.let { moshi.adapter(Card::class.java).toJson(it) }

    @androidx.room.TypeConverter
    fun toCard(value: String?): Card? =
        value?.let { moshi.adapter(Card::class.java).fromJson(it) }

    @androidx.room.TypeConverter
    fun fromCrmEnvelope(value: CRMEnvelope?): String? =
        value?.let { moshi.adapter(CRMEnvelope::class.java).toJson(it) }

    @androidx.room.TypeConverter
    fun toCrmEnvelope(value: String?): CRMEnvelope? =
        value?.let { moshi.adapter(CRMEnvelope::class.java).fromJson(it) }

    @androidx.room.TypeConverter
    fun fromInstant(value: Instant?): Long? =
        value?.toEpochMilliseconds()

    @androidx.room.TypeConverter
    fun toInstant(value: Long?): Instant? =
        value?.let { Instant.fromEpochMilliseconds(it) }

    @androidx.room.TypeConverter
    fun fromLocalDate(value: LocalDate?): String? =
        value?.toString()

    @androidx.room.TypeConverter
    fun toLocalDate(value: String?): LocalDate? =
        value?.let { LocalDate.parse(it) }

    @androidx.room.TypeConverter
    fun fromLocalDateTimeMillis(value: Long?): String? =
        value?.let { Instant.fromEpochMilliseconds(it).toLocalDateTime(TimeZone.UTC).toString() }

    companion object {
        private val STRING_LIST_TYPE = Types.newParameterizedType(List::class.java, String::class.java)
    }
}
