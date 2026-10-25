package com.mycorrhizal.crm.model.util

/**
 * The user's 12/24-hour preference, resolved in one place so every time render
 * (occasion ranges, the time picker, later reminders/audit) agrees (#1830).
 *
 * [AUTO] defers to the device setting (`android.text.format.DateFormat.is24HourFormat`),
 * so an en-US user whose phone is set to 24-hour sees 24-hour times. [H12] and
 * [H24] are the explicit overrides #1830's `time_format` setting will supply
 * (`auto | 12h | 24h`); until that backend setting exists the app root always
 * passes [AUTO].
 */
enum class TimeFormatPreference {
    AUTO,
    H12,
    H24
    ;

    /** True when times render 24-hour; [deviceIs24Hour] is the platform setting, used only for [AUTO]. */
    fun is24Hour(deviceIs24Hour: Boolean): Boolean = when (this) {
        AUTO -> deviceIs24Hour
        H12 -> false
        H24 -> true
    }

    companion object {
        /** Parses the `time_format` wire token; anything unknown is [AUTO]. */
        fun fromToken(token: String?): TimeFormatPreference = when (token) {
            "12h" -> H12
            "24h" -> H24
            else -> AUTO
        }
    }
}
