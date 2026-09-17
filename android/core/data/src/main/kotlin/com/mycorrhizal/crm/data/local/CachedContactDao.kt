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

    /** Issue #212: flip a cached contact's favorite flag (favorite/unfavorite). */
    @Query("UPDATE cached_contacts SET isFavorite = :isFavorite WHERE id = :id")
    suspend fun setFavorite(id: Int, isFavorite: Boolean)

    /** Records the device LOOKUP_KEY after a T57 import (§7.5.4). */
    @Query("UPDATE cached_contacts SET deviceLookupKey = :lookupKey WHERE id = :id")
    suspend fun setDeviceLookupKey(id: Int, lookupKey: String?)

    /**
     * Call/SMS tracking match (§6.1/6.2, T76, issue #963): resolves a device
     * number to a cached contact by its [PhoneKey] — the same last-10-digit
     * canonical key the server and offline search use. [phoneKey] is matched as
     * a *whole token* against `phonesNormalized` (the space-joined full-digit +
     * key tokens [PhoneKey.flatten] builds from **every** number a contact
     * stores), so it finds a number regardless of punctuation, an
     * international (`+`/country-code) vs. local form, or a trunk-prefix
     * difference — and matches any of the contact's numbers, not just
     * [primaryPhone]. The `' ' || … || ' '` anchoring makes it a token-exact
     * match: a key that is merely a suffix of a longer unrelated number (e.g.
     * `5551234` vs. a stored `15551234`) never matches.
     *
     * The caller passes an already-computed, non-empty [PhoneKey.key]: the
     * repository returns null for a <7-digit number, which keys to `""` and
     * must never match (short codes / extensions).
     */
    @Query(
        """
        SELECT * FROM cached_contacts
        WHERE deleted = 0
          AND instr(' ' || phonesNormalized || ' ', ' ' || :phoneKey || ' ') > 0
        LIMIT 1
        """,
    )
    suspend fun findByPhoneKey(phoneKey: String): CachedContact?

    @Query(
        """
        SELECT * FROM cached_contacts
        WHERE deleted = 0
          AND primaryEmail = :email COLLATE NOCASE
        LIMIT 1
        """,
    )
    suspend fun findByEmail(email: String): CachedContact?

    /**
     * Issue #1122: ids of cached, non-deleted contacts that have a primary
     * phone but have never had a full detail fetch (`card IS NULL`) — so their
     * [CachedContact.phonesNormalized] only knows [CachedContact.primaryPhone]
     * and a call/SMS from any other number they store can never match. Ordered
     * by id so a bounded per-run consumer (see ContactPhoneIndexBackfillWorker)
     * makes steady progress across repeated calls rather than re-picking the
     * same rows.
     */
    @Query(
        """
        SELECT id FROM cached_contacts
        WHERE deleted = 0 AND card IS NULL AND primaryPhone IS NOT NULL
        ORDER BY id ASC
        LIMIT :limit
        """,
    )
    suspend fun getIdsMissingPhoneIndex(limit: Int): List<Int>
}
