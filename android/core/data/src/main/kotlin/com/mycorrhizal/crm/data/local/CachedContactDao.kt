package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedContactDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(contacts: List<CachedContact>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(contact: CachedContact)

    @Query("SELECT * FROM cached_contacts WHERE id = :id")
    suspend fun getById(id: Int): CachedContact?

    @Query("SELECT * FROM cached_contacts WHERE id IN (:ids)")
    suspend fun getByIds(ids: List<Int>): List<CachedContact>

    @Query("SELECT * FROM cached_contacts WHERE deleted = 0 ORDER BY fn COLLATE NOCASE ASC")
    suspend fun getAll(): List<CachedContact>

    @Query("SELECT * FROM cached_contacts WHERE deleted = 1")
    suspend fun getDeleted(): List<CachedContact>

    @Query(
        """
        SELECT * FROM cached_contacts
        WHERE deleted = 0
          AND (
              fn LIKE '%' || :query || '%'
              OR firstname LIKE '%' || :query || '%'
              OR lastname LIKE '%' || :query || '%'
              OR primaryEmail LIKE '%' || :query || '%'
              OR primaryPhone LIKE '%' || :query || '%'
          )
        ORDER BY fn COLLATE NOCASE ASC
        """,
    )
    suspend fun search(query: String): List<CachedContact>

    /**
     * FTS4 search over the cached contact mirror (Phase 2 item 13). The FTS
     * table's rowid aliases cached_contacts.id; a MATCH on the joined columns
     * returns the matching cached rows. `query` is a single search term —
     * FTS' prefix syntax is applied so "dav" also matches "David".
     */
    @Query(
        """
        SELECT c.* FROM cached_contacts_fts f
        JOIN cached_contacts c ON c.id = f.rowid
        WHERE cached_contacts_fts MATCH :query || '*'
          AND c.deleted = 0
        ORDER BY c.fn COLLATE NOCASE ASC
        """,
    )
    suspend fun searchFts(query: String): List<CachedContact>

    /**
     * FTS4 search taking a complete MATCH expression, unlike [searchFts] which treats its
     * argument as a bare prefix term appended with `'*'` in SQL. Used for phone-shaped queries
     * (T76), where the caller has already built an OR-of-prefix-matches expression restricted
     * to the `phonesNormalized` column — see `ContactRepositoryImpl.phoneMatchExpr`.
     */
    @Query(
        """
        SELECT c.* FROM cached_contacts_fts f
        JOIN cached_contacts c ON c.id = f.rowid
        WHERE cached_contacts_fts MATCH :matchExpr
          AND c.deleted = 0
        ORDER BY c.fn COLLATE NOCASE ASC
        """,
    )
    suspend fun searchFtsMatch(matchExpr: String): List<CachedContact>

    @Query("DELETE FROM cached_contacts")
    suspend fun deleteAll()

    @Query("DELETE FROM cached_contacts WHERE id IN (:ids)")
    suspend fun deleteByIds(ids: List<Int>)

    @Query("DELETE FROM cached_contacts WHERE id = :id")
    suspend fun deleteById(id: Int)

    /** M24: flip a cached contact's archived flag (archive/unarchive from the detail screen). */
    @Query("UPDATE cached_contacts SET archived = :archived WHERE id = :id")
    suspend fun setArchived(id: Int, archived: Boolean)

    /** Records the device LOOKUP_KEY after a T57 import (§7.5.4). */
    @Query("UPDATE cached_contacts SET deviceLookupKey = :lookupKey WHERE id = :id")
    suspend fun setDeviceLookupKey(id: Int, lookupKey: String?)

    /**
     * Best-effort phone match for call/SMS tracking (§6.1/6.2): the device
     * number and the cached primaryPhone are both normalized to digits-only
     * before comparison, so formatting differences (spaces/dashes/parens)
     * don't hide a match. Exact digit-match only; international-prefix
     * normalization is a deliberate non-goal here (see §12 open question 1).
     */
    @Query(
        """
        SELECT * FROM cached_contacts
        WHERE deleted = 0
          AND REPLACE(REPLACE(REPLACE(REPLACE(primaryPhone, ' ', ''), '-', ''), '(', ''), ')', '')
              = REPLACE(REPLACE(REPLACE(REPLACE(:phone, ' ', ''), '-', ''), '(', ''), ')', '')
        LIMIT 1
        """,
    )
    suspend fun findByPhoneDigits(phone: String): CachedContact?

    @Query(
        """
        SELECT * FROM cached_contacts
        WHERE deleted = 0
          AND primaryEmail = :email COLLATE NOCASE
        LIMIT 1
        """,
    )
    suspend fun findByEmail(email: String): CachedContact?
}
