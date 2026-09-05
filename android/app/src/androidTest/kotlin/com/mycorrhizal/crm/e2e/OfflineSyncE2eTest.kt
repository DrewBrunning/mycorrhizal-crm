package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.onAllNodesWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkInfo
import androidx.work.WorkManager
import com.mycorrhizal.crm.MainActivity
import com.mycorrhizal.crm.data.di.DataModule
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.REGISTERED_MIGRATIONS
import com.mycorrhizal.crm.data.local.RoomPassphraseStore
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.feature.tracking.InteractionCapture
import com.mycorrhizal.crm.feature.tracking.InteractionSyncWorker
import com.mycorrhizal.crm.feature.tracking.TrackingWorkerScheduler
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiClient
import androidx.room.Room
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import net.zetetic.database.sqlcipher.SupportOpenHelperFactory
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.UUID

/**
 * ANDROID-02 (issue #479): the offline-sync outbox lifecycle, driven against the real
 * docker-compose.test.yml backend (issue #238's harness) with the app's real singletons — the
 * real encrypted Room mirror, the real [PendingInteractionRepository] (the same call the
 * call-log/SMS capture paths make), the real DataStore-backed tracking opt-ins, and the real
 * [InteractionSyncWorker] enqueued through WorkManager (Hilt-injected, so it reads exactly the
 * state the test just wrote).
 *
 * What "offline" means here: the app has no connectivity manager — it discovers offline at the
 * HTTP boundary — so this suite switches the session's base URL (via the real SessionManager) to
 * an unreachable loopback origin. Every worker attempt then fails exactly the way a real offline
 * phone's does ([ApiError.Network]), while the E2E helper's own client keeps talking to the real
 * backend on the real URL for seeding/assertions.
 *
 * Why the outbox matters more than the cache (the ticket's framing): every other table in
 * `mycorrhizal-cache.db` is rebuildable, but `pending_interactions` holds the only copy of a
 * device-detected call/SMS until it syncs. So the suite's spine is: no queued entry is lost, no
 * entry produces a duplicate server record, and entries survive a cold restart of the database
 * stack. The ambiguous-failure and stale-contact scenarios deliberately exercise behaviors that
 * used to be bugs and are now pinned by unit tests too; see each test's doc.
 *
 * Process death / force-stop / reboot fidelity: an instrumentation suite runs in the app's own
 * process, so it cannot kill its own process and come back. The durability test therefore models
 * a cold restart at the persistence boundary — the thing process death actually is for this data:
 * the queue lives in an encrypted file on disk, and the test re-opens that file with a brand-new,
 * independent Room instance (exactly what a restarted process does) and proves the entries are
 * there, intact, then syncs them.
 */
@RunWith(AndroidJUnit4::class)
class OfflineSyncE2eTest : E2eBaseTest() {

    /** An unreachable origin the app's HTTP stack fails against fast (nothing listens here). */
    private val offlineUrl = "http://127.0.0.1:1"

    private val pendingRepo: PendingInteractionRepository
        get() = (compose.activity as MainActivity).pendingInteractionRepository

    private val trackingSettings: TrackingSettingsRepository
        get() = (compose.activity as MainActivity).trackingSettings

    private val sessionManager
        get() = (compose.activity as MainActivity).sessionManager

    private val apiClient: ApiClient
        get() = (compose.activity as MainActivity).apiClient

    private val appContext
        get() = compose.activity.applicationContext

    /** Markers of every server Activity this test expects/created — swept in teardown. */
    private val createdActivityIds = mutableListOf<Long>()

    @Before
    fun offlineSyncSetUp() {
        runBlocking {
            System.loadLibrary("sqlcipher") // for the cold-reopen test's independent SQLCipher open
            // The suite owns the interaction outbox; keep the app's 15-minute periodic sync worker
            // from running mid-test and draining rows at an unpredictable moment.
            WorkManager.getInstance(appContext)
                .cancelUniqueWork(TrackingWorkerScheduler.UNIQUE_INTERACTION_SYNC)
            // Sweep any device:* activities a crashed earlier run left on the server — this suite is
            // the only producer of that external_ref shape, so they are all leftovers.
            backend.listAllActivities()
                .filter { it.optString("external_ref").startsWith("device:") }
                .forEach { runCatching { backend.deleteActivity(it.getLong("ID")) } }
            // The worker gates on the tracking opt-ins; flip them on through the real repository.
            trackingSettings.setCallTrackingEnabled(true)
            trackingSettings.setSmsTrackingEnabled(true)
            check(trackingSettings.callTrackingEnabled()) { "call tracking did not persist" }
        }
    }

    @After
    fun offlineSyncTearDown() {
        runBlocking {
            createdActivityIds.forEach { id -> runCatching { backend.deleteActivity(id) } }
            createdActivityIds.clear()
            runCatching {
                trackingSettings.setCallTrackingEnabled(false)
                trackingSettings.setSmsTrackingEnabled(false)
            }
        }
    }

    // ------------------------------------------------------------------ helpers

    /** A queued row plus the server-side markers (external_ref + epoch-second date) that identify
     *  the Activity the sync worker creates from it. */
    private class Queued(val pending: PendingInteraction) {
        val externalRef: String get() = "device:${pending.id}"
        val epochSecond: Long get() = pending.timestampMillis / 1000
    }

    private suspend fun goOffline() {
        sessionManager.setServerUrl(offlineUrl)
    }

    private suspend fun goOnline() {
        sessionManager.setServerUrl(E2eConfig.serverUrl)
    }

    /** Enables both tracking opt-ins and queues an interaction the way the capture paths do. */
    private suspend fun queue(
        kind: String,
        direction: String?,
        phoneNumber: String,
        timestampMillis: Long,
        matchedContactId: Int? = null,
    ): Queued {
        pendingRepo.record(
            PendingInteraction(
                timestampMillis = timestampMillis,
                kind = kind,
                direction = direction,
                phoneNumber = phoneNumber,
                matchedContactId = matchedContactId,
            ),
        )
        val row = pendingRepo.unsynced().single { it.phoneNumber == phoneNumber }
        return Queued(row)
    }

    /** A distinct timestamp for every row so no two rows in a run share a server date. */
    private var nextTimestamp = System.currentTimeMillis() - 60_000

    private fun freshTimestamp(): Long {
        nextTimestamp += 1000
        return nextTimestamp
    }

    private fun uniquePhone(label: String): String {
        val token = UUID.randomUUID().toString().replace("-", "").take(6)
        return "+1555$label$token"
    }

    /** The worker's own date wire-format, replicated (see the class doc's drift note). */
    private fun workerDate(epochMillis: Long): String =
        DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(
            Instant.ofEpochMilli(epochMillis).atOffset(ZoneOffset.UTC),
        )

    /** The ActivityInput the worker would build for [row] — used to simulate an ambiguous first
     *  attempt byte-for-byte (same Moshi serialization path, same header). If this mapping ever
     *  drifts from InteractionSyncWorker's, the server's different-body check answers 422 and the
     *  test fails loudly rather than silently passing. */
    private fun workerInputFor(row: PendingInteraction): ActivityInput {
        val type = when (row.kind) {
            InteractionCapture.KIND_CALL -> "call"
            else -> "message"
        }
        val title = when (row.kind) {
            InteractionCapture.KIND_MESSAGE -> "Message"
            InteractionCapture.KIND_CALL ->
                if (row.direction == InteractionCapture.DIR_MISSED) "Missed call" else "Call"
            else -> "Interaction"
        }
        return ActivityInput(
            title = title,
            date = workerDate(row.timestampMillis),
            contactIds = row.matchedContactId?.let { listOf(it) },
            type = type,
            externalRef = "device:${row.id}",
        )
    }

    /** Runs the real [InteractionSyncWorker] once through WorkManager and waits for it to finish. */
    private suspend fun runSyncWorkerOnce(): WorkInfo.State {
        val name = "e2e-offline-sync-${UUID.randomUUID()}"
        val wm = WorkManager.getInstance(appContext)
        wm.enqueueUniqueWork(
            name,
            ExistingWorkPolicy.REPLACE,
            OneTimeWorkRequestBuilder<InteractionSyncWorker>().build(),
        )
        val deadlineMs = System.currentTimeMillis() + 60_000
        while (true) {
            val state = withContext(Dispatchers.IO) {
                wm.getWorkInfosForUniqueWork(name).get().lastOrNull()?.state
            }
            if (state == WorkInfo.State.SUCCEEDED ||
                state == WorkInfo.State.FAILED ||
                state == WorkInfo.State.CANCELLED
            ) {
                return state
            }
            if (System.currentTimeMillis() > deadlineMs) {
                error("interaction sync worker did not reach a terminal state within 60s")
            }
            delay(250)
        }
    }

    /** The server Activities created for [queued] — matched on external_ref + exact second. */
    private fun serverActivitiesFor(queued: Queued): List<JSONObject> =
        backend.listAllActivities(includeContacts = true).filter { activity ->
            activity.optString("external_ref") == queued.externalRef &&
                secondOf(activity.optString("date")) == queued.epochSecond
        }

    private fun secondOf(rfc3339: String): Long? = runCatching {
        OffsetDateTime.parse(rfc3339).toEpochSecond()
    }.getOrNull()

    private fun assertExactlyOneServerActivity(queued: Queued, context: String) {
        val matches = serverActivitiesFor(queued)
        assertEquals(
            "exactly one server Activity for ${queued.externalRef} ($context); got $matches",
            1,
            matches.size,
        )
        createdActivityIds += matches.single().getLong("ID")
    }

    // ------------------------------------------------------------------ tests

    /** Recommended action 1: queue while offline, reconnect, sync — exactly one server record
     *  per queued entry, nothing lost, nothing duplicated, outbox drained. */
    @Test
    fun queueWhileOfflineThenReconnect_syncsExactlyOneServerRecordPerEntry() = runBlocking {
        val matchedContact = createTestContact(uniqueName("Sync"), "Target")
        val call = queue(
            kind = InteractionCapture.KIND_CALL,
            direction = InteractionCapture.DIR_INCOMING,
            phoneNumber = uniquePhone("matched"),
            timestampMillis = freshTimestamp(),
            matchedContactId = matchedContact.id.toInt(),
        )
        val missed = queue(
            kind = InteractionCapture.KIND_CALL,
            direction = InteractionCapture.DIR_MISSED,
            phoneNumber = uniquePhone("missed"),
            timestampMillis = freshTimestamp(),
        )
        val message = queue(
            kind = InteractionCapture.KIND_MESSAGE,
            direction = InteractionCapture.DIR_INCOMING,
            phoneNumber = uniquePhone("message"),
            timestampMillis = freshTimestamp(),
        )

        // Offline: the periodic-style sync runs and fails at the HTTP boundary; nothing is lost
        // and nothing is acked.
        goOffline()
        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertEquals(3, pendingRepo.unsynced().size)
        assertEquals(0, serverActivitiesFor(call).size + serverActivitiesFor(missed).size + serverActivitiesFor(message).size)

        // Reconnect: the next run drains the queue and creates one Activity per entry.
        goOnline()
        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertTrue("outbox must be drained after a successful run", pendingRepo.unsynced().isEmpty())

        assertExactlyOneServerActivity(call, "matched incoming call")
        assertExactlyOneServerActivity(missed, "unmatched missed call")
        assertExactlyOneServerActivity(message, "unmatched message")
    }

    /** Recommended action 3: an ambiguous failure — the server committed, the client never saw
     *  the response — must not duplicate on retry. The outbox row's Idempotency-Key makes the
     *  retry replay the stored outcome instead. */
    @Test
    fun ambiguousFailureRetry_doesNotDuplicateTheServerRecord() = runBlocking {
        val contact = createTestContact(uniqueName("Ambig"), "Target")
        queue(
            kind = InteractionCapture.KIND_CALL,
            direction = InteractionCapture.DIR_INCOMING,
            phoneNumber = uniquePhone("ambig"),
            timestampMillis = freshTimestamp(),
            matchedContactId = contact.id.toInt(),
        )
        val row = pendingRepo.unsynced().single()
        val queued = Queued(row)
        val key = checkNotNull(row.idempotencyKey) { "queued row must carry an idempotency key" }

        // Simulate the ambiguous first attempt: the exact request the worker makes — same body,
        // same key, through the app's own client — reaches the server and commits, but the client
        // "never sees the response" (we discard the outcome, which is precisely the ambiguity).
        assertTrue(
            "the simulated first attempt must commit",
            apiClient.createActivity(workerInputFor(row), key).isSuccess,
        )
        assertExactlyOneServerActivity(queued, "after the ambiguous first attempt")

        // The real worker retries the still-unsynced row with the same key. The server replays
        // its stored response; it must NOT create a second Activity.
        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertTrue("the retried row must now be drained", pendingRepo.unsynced().isEmpty())
        assertExactlyOneServerActivity(queued, "after the retried sync")
    }

    /** Recommended action 4: a queued interaction whose matched contact was deleted server-side
     *  while offline resolves per ADR-0009 — the interaction syncs unassociated rather than
     *  staying stuck in the queue (the CON-03 conflict policy, not a second Android-only one). */
    @Test
    fun staleContactLinkOnReconnect_isDroppedAndTheInteractionStillSyncs() = runBlocking {
        val contact = createTestContact(uniqueName("Conflic"), "Target")
        queue(
            kind = InteractionCapture.KIND_CALL,
            direction = InteractionCapture.DIR_OUTGOING,
            phoneNumber = uniquePhone("conflict"),
            timestampMillis = freshTimestamp(),
            matchedContactId = contact.id.toInt(),
        )
        val queued = Queued(pendingRepo.unsynced().single())

        // The contact vanishes while the device is offline (deleted on another client).
        goOffline()
        backend.deleteContact(contact.id)
        goOnline()

        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertTrue("the conflicted row must drain, not sit in the queue forever", pendingRepo.unsynced().isEmpty())

        val matches = serverActivitiesFor(queued)
        assertEquals(1, matches.size)
        val activity = matches.single()
        createdActivityIds += activity.getLong("ID")
        val participants = activity.optJSONArray("contacts")
        assertTrue(
            "the interaction must survive as an unassociated Activity (its link was stale)",
            participants == null || participants.length() == 0,
        )
    }

    /** Recommended action 2: the outbox lives in the encrypted on-disk mirror, so it survives a
     *  cold restart of the process — modeled here by reopening the real DB file with a brand-new,
     *  independent Room instance (what a restarted process does) and then syncing. */
    @Test
    fun queuedOutboxSurvivesColdReopenOfTheEncryptedDatabase() = runBlocking {
        val first = queue(
            kind = InteractionCapture.KIND_CALL,
            direction = InteractionCapture.DIR_INCOMING,
            phoneNumber = uniquePhone("cold-a"),
            timestampMillis = freshTimestamp(),
        )
        val second = queue(
            kind = InteractionCapture.KIND_MESSAGE,
            direction = InteractionCapture.DIR_INCOMING,
            phoneNumber = uniquePhone("cold-b"),
            timestampMillis = freshTimestamp(),
            matchedContactId = createTestContact(uniqueName("Cold"), "Target").id.toInt(),
        )

        // A brand-new AppDatabase over the app's real encrypted file — the same builder shape
        // DataModule uses — i.e. a cold process reading the disk state.
        val dbFile = appContext.getDatabasePath(DataModule.DB_NAME)
        val passphrase = RoomPassphraseStore(appContext).getOrCreate()
        val cold = Room.databaseBuilder(appContext, AppDatabase::class.java, dbFile.absolutePath)
            .openHelperFactory(SupportOpenHelperFactory(passphrase.toByteArray()))
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .fallbackToDestructiveMigration()
            .build()
        try {
            val rows = cold.pendingInteractionDao().getUnsynced()
            assertEquals(2, rows.size)
            val coldCall = rows.single { it.phoneNumber == first.pending.phoneNumber }
            assertEquals(first.pending.id, coldCall.id)
            assertEquals(first.pending.kind, coldCall.kind)
            assertEquals(first.pending.timestampMillis, coldCall.timestampMillis)
            val coldMessage = rows.single { it.phoneNumber == second.pending.phoneNumber }
            assertEquals(second.pending.matchedContactId, coldMessage.matchedContactId)
            // ANDROID-02: the rows carried their idempotency keys across the cold open, so a
            // restarted worker is still retry-safe.
            assertTrue(rows.all { !it.idempotencyKey.isNullOrBlank() })
        } finally {
            cold.close()
        }

        // After the "restart", the app syncs exactly what was queued before it died.
        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertTrue(pendingRepo.unsynced().isEmpty())
        assertExactlyOneServerActivity(first, "cold-restart survivor")
        assertExactlyOneServerActivity(second, "cold-restart survivor")
    }

    /** Recommended action 6: a long offline stretch — dozens of entries while the server moved
     *  on — drains cleanly with one server record each and no losses. */
    @Test
    fun longOfflinePeriodManyEntries_allSyncExactlyOnceAfterReconnect() = runBlocking {
        val queued = mutableListOf<Queued>()
        val contacts = (1..3).map { createTestContact(uniqueName("Long"), "Target$it") }
        repeat(30) { i ->
            val matched = contacts[i % contacts.size].id.toInt()
            val kind = if (i % 2 == 0) InteractionCapture.KIND_CALL else InteractionCapture.KIND_MESSAGE
            val direction = if (kind == InteractionCapture.KIND_CALL) InteractionCapture.DIR_INCOMING else InteractionCapture.DIR_INCOMING
            queued += queue(
                kind = kind,
                direction = direction,
                phoneNumber = uniquePhone("long$i"),
                timestampMillis = freshTimestamp(),
                matchedContactId = matched,
            )
        }

        goOffline()
        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertEquals("nothing must be lost or synced while offline", 30, pendingRepo.unsynced().size)

        goOnline()
        assertEquals(WorkInfo.State.SUCCEEDED, runSyncWorkerOnce())
        assertTrue(pendingRepo.unsynced().isEmpty())
        queued.forEach { assertExactlyOneServerActivity(it, "long-offline drain") }
    }

    /** Recommended action 5 (cache side): a server-side deletion propagates to the client view
     *  after a refresh — the cache converges, no ghost survives. */
    @Test
    fun serverSideDeletionDoesNotLeaveAGhostAfterRefresh() {
        val given = uniqueName("Ghost")
        val surname = "Target"
        val contact = createTestContact(given, surname)
        val displayName = "$given $surname"

        // Load the contact list online so the mirror is warm.
        navigateViaDrawer("Contacts")
        searchFor(given)
        waitForText(displayName)

        // Delete it on the server (another device / the web), as if the device went offline,
        // made its edit, and is now back online refreshing.
        runBlocking { backend.deleteContact(contact.id) }

        // A fresh refresh must converge: the ghost disappears once the online list is re-fetched.
        searchFor(given)
        compose.waitUntil(30_000) {
            compose.onAllNodesWithText(displayName).fetchSemanticsNodes().isEmpty()
        }
    }
}
