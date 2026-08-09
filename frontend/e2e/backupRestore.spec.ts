import { test, expect, request as playwrightRequest } from '@playwright/test';
import { spawn, execSync, ChildProcess } from 'child_process';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as net from 'net';
import { fileURLToPath } from 'url';
import { toContactRecordInput } from '../src/api/contacts';

// N6 (docs/fork-plan/tickets/26-N6-backup-restore.md): full backup/restore.
// The ticket is a docs + Makefile task with a hard requirement that the
// *documented procedure is actually tested* — back up a populated instance,
// destroy the .db and the photo directory, restore, and verify contacts,
// notes, activities, reminders, relationships, and photos all survive.
//
// This spec is that e2e test. It deliberately does NOT touch the shared 7300
// stack (that database is shared by the whole parallel suite — a restore would
// wipe it, and the container image has no sqlite tooling). Instead it:
//
//   1. builds the backend binary from source,
//   2. runs it against an isolated throwaway data directory on a free port,
//   3. seeds a user + contact + note + activity + reminder + relationship +
//      photo + attachment through the real API,
//   4. runs the real `make backup` Makefile target against the live server and
//      rsync-equivalents the photo + attachment directories,
//   5. kills the server and destroys the database + photo + attachment files,
//   6. restores everything from the backup,
//   7. restarts the server and verifies, via the API, that every seeded row —
//      and the bytes on disk behind the photo and attachment — survived.
//
// It needs a Go toolchain + make on the host (it compiles the backend), which
// the e2e CI job provisions (see e2e-tests.yml). On a machine without them the
// spec skips with a message rather than failing the whole suite.

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BACKEND_DIR = path.resolve(__dirname, '../../backend');

const TEST_USER = {
  username: `backupuser${Date.now()}`,
  email: `backupuser${Date.now()}@example.com`,
  password: 'TestPassword123!',
};

// A 1x1 PNG: the photo upload path validates that uploads are real images.
const PNG_1x1 = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64'
);

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.once('error', reject);
    srv.listen(0, () => {
      const port = (srv.address() as net.AddressInfo).port;
      srv.close(() => resolve(port));
    });
  });
}

function backendIsUsable(): boolean {
  try {
    execSync('go version', { stdio: 'ignore' });
    execSync('make --version', { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

function buildServer(buildDir: string): string {
  const bin = path.join(buildDir, 'mycorrhizal-server');
  execSync(`go build -o ${JSON.stringify(bin)} .`, { cwd: BACKEND_DIR, stdio: 'pipe' });
  return bin;
}

interface RunningServer {
  proc: ChildProcess;
  port: number;
  dataDir: string;
  dbPath: string;
  photosDir: string;
  attachmentsDir: string;
}

function startServer(bin: string, port: number, dataDir: string): RunningServer {
  const photosDir = path.join(dataDir, 'photos');
  const attachmentsDir = path.join(dataDir, 'attachments');
  fs.mkdirSync(photosDir, { recursive: true });
  fs.mkdirSync(attachmentsDir, { recursive: true });
  const dbPath = path.join(dataDir, 'mycorrhizal.db');

  const proc = spawn(bin, [], {
    env: {
      ...process.env,
      PORT: String(port),
      SQLITE_DB_PATH: dbPath,
      PROFILE_PHOTO_DIR: photosDir,
      ATTACHMENTS_DIR: attachmentsDir,
      FRONTEND_URL: `http://localhost:${port}`,
      JWT_SECRET_KEY: 'backup-restore-e2e-secret-key-min-32-chars!',
      DISABLE_REGISTRATION: 'false',
      // The whole lifecycle runs from one origin, so every request shares one
      // rate-limit bucket (same reason docker-compose.test.yml raises these).
      API_RATE_LIMIT_INTERVAL_MS: '1',
      API_RATE_LIMIT_BURST: '100000',
      GIN_MODE: 'release',
      LOG_LEVEL: 'fatal',
      LOG_PRETTY: 'false',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  proc.stderr?.on('data', (d) => process.stderr.write(`[backup-restore backend] ${d}`));

  return { proc, port, dataDir, dbPath, photosDir, attachmentsDir };
}

async function waitForHealth(port: number, timeoutMs = 30000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://localhost:${port}/health`);
      if (res.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`backend on :${port} did not become healthy within ${timeoutMs}ms`);
}

function stopServer(srv: RunningServer): Promise<void> {
  return new Promise((resolve) => {
    if (!srv.proc || srv.proc.exitCode !== null) return resolve();
    const timer = setTimeout(() => {
      if (srv.proc.exitCode === null) srv.proc.kill('SIGKILL');
    }, 10000);
    timer.unref();
    srv.proc.once('exit', () => {
      clearTimeout(timer);
      resolve();
    });
    srv.proc.kill('SIGTERM');
  });
}

function copyTree(src: string, dest: string): void {
  fs.cpSync(src, dest, { recursive: true });
}

function emptyDir(dir: string): void {
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
}

test.describe('Full backup/restore (N6)', () => {
  test('a populated instance survives backup → destroy → restore with every entity intact', async () => {
    test.setTimeout(300_000);

    if (!backendIsUsable()) {
      test.skip('go/make toolchain not available on this host; cannot compile and drive the backend');
    }

    const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mycorrhizal-backup-e2e-'));
    const buildDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mycorrhizal-backup-build-'));

    const serverBin = buildServer(buildDir);
    const port = await freePort();
    let srv: RunningServer | undefined;

    try {
      srv = startServer(serverBin, port, dataDir);
      await waitForHealth(port);

      // --- Seed a full instance through the real API -------------------------
      const api = await playwrightRequest.newContext({ baseURL: `http://localhost:${port}` });

      const reg = await api.post('/api/v1/register', { data: TEST_USER });
      expect(reg.ok(), `registration failed: ${reg.status()} ${await reg.text()}`).toBeTruthy();

      const login = await api.post('/api/v1/login', {
        data: { identifier: TEST_USER.username, password: TEST_USER.password },
      });
      expect(login.ok(), `login failed: ${login.status()}`).toBeTruthy();

      const mkContact = async (firstname: string, lastname: string) => {
        const res = await api.post('/api/v1/contacts', {
          data: toContactRecordInput({ firstname, lastname }),
        });
        expect(res.ok(), `contact create failed: ${res.status()} ${await res.text()}`).toBeTruthy();
        const body = await res.json();
        const c = body.contact || body;
        return { id: c.id ?? c.ID, uid: c.uid };
      };

      const contact = await mkContact('BackupRestore', 'Alice');
      const second = await mkContact('BackupRestore', 'Bob');
      expect(contact.uid).toBeTruthy();

      const noteContent = `note that must survive the restore ${Date.now()}`;
      const note = await api.post(`/api/v1/contacts/${contact.id}/notes`, {
        data: { content: noteContent, date: new Date().toISOString() },
      });
      expect(note.ok(), `note create failed: ${note.status()}`).toBeTruthy();

      const activityTitle = `activity that must survive ${Date.now()}`;
      const activity = await api.post('/api/v1/activities', {
        data: { title: activityTitle, date: new Date().toISOString(), contact_ids: [contact.id] },
      });
      expect(activity.ok(), `activity create failed: ${activity.status()}`).toBeTruthy();

      const reminderMessage = `reminder that must survive ${Date.now()}`;
      const reminder = await api.post(`/api/v1/contacts/${contact.id}/reminders`, {
        data: {
          message: reminderMessage,
          by_mail: false,
          remind_at: new Date(Date.now() + 86400000).toISOString(),
          recurrence: 'once',
          reoccur_from_completion: false,
          contact_id: contact.id,
        },
      });
      expect(reminder.ok(), `reminder create failed: ${reminder.status()}`).toBeTruthy();

      const edge = await api.post('/api/v1/relationship-edges', {
        data: { source_id: contact.uid, target_id: second.uid, type: 'friend_of' },
      });
      expect(edge.ok(), `relationship create failed: ${edge.status()}`).toBeTruthy();

      const photo = await api.post(`/api/v1/contacts/${contact.id}/profile_picture`, {
        multipart: {
          photo: { name: 'portrait.png', mimeType: 'image/png', buffer: PNG_1x1 },
        },
      });
      expect(photo.ok(), `photo upload failed: ${photo.status()}`).toBeTruthy();

      const attachment = await api.post(`/api/v1/contacts/${contact.id}/attachments`, {
        multipart: {
          file: { name: 'contract.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4 backup-restore contract') },
        },
      });
      expect(attachment.ok(), `attachment upload failed: ${attachment.status()}`).toBeTruthy();
      const attachmentBody = await attachment.json();
      const attachmentId = (attachmentBody.attachment || attachmentBody).ID;

      // The photo and attachment bytes actually live on disk before the backup.
      const photosBefore = fs.readdirSync(srv.photosDir);
      const attachmentsBefore = fs.readdirSync(srv.attachmentsDir);
      expect(photosBefore.length).toBeGreaterThan(0);
      expect(attachmentsBefore.length).toBeGreaterThan(0);
      await api.dispose();

      // --- Back up: the real `make backup` target + dir copies --------------
      const backupDb = path.join(dataDir, 'backup.db');
      const backupPhotos = path.join(dataDir, 'backup-photos');
      const backupAttachments = path.join(dataDir, 'backup-attachments');
      execSync('make backup', {
        cwd: BACKEND_DIR,
        env: {
          ...process.env,
          SQLITE_DB_PATH: srv.dbPath,
          BACKUP_PATH: backupDb,
        },
        stdio: 'pipe',
      });
      expect(fs.existsSync(backupDb)).toBeTruthy();
      copyTree(srv.photosDir, backupPhotos);
      copyTree(srv.attachmentsDir, backupAttachments);

      // --- Destroy the instance ---------------------------------------------
      await stopServer(srv);
      for (const suffix of ['', '-wal', '-shm']) {
        fs.rmSync(`${srv.dbPath}${suffix}`, { force: true });
      }
      expect(fs.existsSync(srv.dbPath)).toBeFalsy();
      emptyDir(srv.photosDir);
      emptyDir(srv.attachmentsDir);

      // --- Restore from the backup ------------------------------------------
      copyTree(backupDb, srv.dbPath);
      copyTree(backupPhotos, srv.photosDir);
      copyTree(backupAttachments, srv.attachmentsDir);

      // --- Restart and verify everything survived ---------------------------
      srv = startServer(serverBin, port, dataDir);
      await waitForHealth(port);

      const api2 = await playwrightRequest.newContext({ baseURL: `http://localhost:${port}` });
      const login2 = await api2.post('/api/v1/login', {
        data: { identifier: TEST_USER.username, password: TEST_USER.password },
      });
      expect(login2.ok(), `login after restore failed: ${login2.status()}`).toBeTruthy();

      const search = await api2.get(
        `/api/v1/contacts?search=${encodeURIComponent('BackupRestore')}&limit=10`
      );
      expect(search.ok()).toBeTruthy();
      const searchBody = await search.json();
      const names = (searchBody.contacts || []).map((c: any) => `${c.firstname} ${c.lastname}`);
      expect(names).toContain('BackupRestore Alice');
      expect(names).toContain('BackupRestore Bob');

      const notes = await api2.get(`/api/v1/contacts/${contact.id}/notes`);
      expect(notes.ok()).toBeTruthy();
      expect(await notes.text()).toContain(noteContent);

      const reminders = await api2.get(`/api/v1/contacts/${contact.id}/reminders`);
      expect(reminders.ok()).toBeTruthy();
      expect(await reminders.text()).toContain(reminderMessage);

      const activities = await api2.get('/api/v1/activities');
      expect(activities.ok()).toBeTruthy();
      expect(await activities.text()).toContain(activityTitle);

      const edges = await api2.get(
        `/api/v1/relationship-edges?contact_id=${encodeURIComponent(contact.uid)}&limit=100`
      );
      expect(edges.ok()).toBeTruthy();
      expect(await edges.text()).toContain(second.uid);

      // The photo survives as real bytes on disk (the upload path re-encodes
      // to JPEG, so compare against the stored file, not the PNG we sent) and
      // is served back byte-for-byte.
      const photoBytes = fs.readFileSync(path.join(srv.photosDir, photosBefore[0]));
      expect(photoBytes.length).toBeGreaterThan(0);
      expect(photoBytes[0]).toBe(0xff);
      expect(photoBytes[1]).toBe(0xd8); // JPEG magic
      const photoServed = await api2.get(`/api/v1/contacts/${contact.id}/profile_picture`);
      expect(photoServed.ok()).toBeTruthy();
      expect((await photoServed.body()).equals(photoBytes)).toBeTruthy();

      // The attachment survives as real bytes on disk and downloads cleanly.
      const attachmentBytes = fs.readFileSync(path.join(srv.attachmentsDir, attachmentsBefore[0]));
      expect(attachmentBytes.toString()).toContain('backup-restore contract');
      const download = await api2.get(`/api/v1/attachments/${attachmentId}/download`);
      expect(download.ok()).toBeTruthy();
      expect((await download.body()).equals(attachmentBytes)).toBeTruthy();
      await api2.dispose();
    } finally {
      if (srv) await stopServer(srv).catch(() => {});
      fs.rmSync(dataDir, { recursive: true, force: true });
      fs.rmSync(buildDir, { recursive: true, force: true });
    }
  });
});
