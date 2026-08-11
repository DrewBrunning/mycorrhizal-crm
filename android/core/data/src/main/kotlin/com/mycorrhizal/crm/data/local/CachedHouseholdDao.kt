package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedHouseholdDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(households: List<CachedHousehold>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(household: CachedHousehold)

    @Query("SELECT * FROM cached_households ORDER BY name COLLATE NOCASE ASC")
    suspend fun getAll(): List<CachedHousehold>

    @Query("SELECT * FROM cached_households WHERE id = :id")
    suspend fun getById(id: String): CachedHousehold?

    @Query("DELETE FROM cached_households WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_households")
    suspend fun deleteAll()
}

@Dao
interface CachedHouseholdMemberDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(members: List<CachedHouseholdMember>)

    @Query("SELECT * FROM cached_household_members WHERE householdId = :householdId")
    suspend fun getByHouseholdId(householdId: String): List<CachedHouseholdMember>

    @Query("DELETE FROM cached_household_members WHERE householdId = :householdId")
    suspend fun deleteByHouseholdId(householdId: String)

    @Query("DELETE FROM cached_household_members WHERE householdId = :householdId AND memberVCardUid = :vcardUid")
    suspend fun deleteMember(householdId: String, vcardUid: String)
}
