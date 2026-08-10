package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * A user-defined link-action template (§7.6.6): for a custom LinkFieldType
 * protocol (e.g. "matrix"), one or more MIMETYPE + intent-URI pairs the user
 * added locally. Stored in Room only — never synced to the server.
 */
@Entity(tableName = "custom_link_actions")
data class CustomLinkAction(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val protocol: String,
    val label: String,
    val kind: String,
    val mimeType: String,
    /** Intent URI template, `{handle}` placeholder substituted at runtime. */
    val intentUriTemplate: String,
)
