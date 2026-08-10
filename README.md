<p align="center">
<img width="192" height="192" alt="mark-mycelium-light-192" src="https://github.com/user-attachments/assets/5f8e7a54-b8e6-408a-b594-9131739822da" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Status-Beta-purple" alt="Status: Beta">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Backend-Go-00ADD8?logo=go" alt="Backend: Go"></a>
  <a href="https://reactjs.org"><img src="https://img.shields.io/badge/Frontend-React-61DAFB?logo=react" alt="Frontend: React"></a>
  <a href="https://kotlinlang.org/"><img src="https://img.shields.io/badge/Android-Kotlin-B125EA?logo=kotlin" alt="Android: Kotlin"></a>
</p>

# Mycorrhizal CRM

Mycorrhizal CRM is a self-hosted contact relationship management solution. It is a fork of [Meerkat CRM](https://github.com/fbuchner/meerkat-crm) by Frederic Buchner.

> ⚠️ Mycorrhizal CRM is a **structural fork** of [Meerkat CRM](https://github.com/fbuchner/meerkat-crm). Because this project introduces custom database schemas, modified tables, and expanded data types (such as bidirectional relationships and custom field mappings), **it is not directly database-compatible with upstream Meerkat.** 
> 
> * Direct database migrations from upstream Meerkat are **NOT** supported at this time.
> * Syncing Options: You can sync contacts between Meerkat and Mycorrhizal using CardDAV (though data not supported by standard CardDAV specs will not sync) or by exporting data from one and importing it into the other (though data not defined in the vCard 3.0 RFC is not guaranteed to persist across the export and import).

---

## Features & Enhancements On Top Of Upstream Meerkat

Mycorrhizal builds heavily upon the solid foundation of Meerkat, adding modern protocol support, deeper structural relationships, and lifestyle tracking utilities.

Everything in this section is **built and working today**. Things that are planned but not yet implemented are listed separately under [On the roadmap](#on-the-roadmap) — nothing below is aspirational.

### Modern Data Formats & Syncing
- **Expanded Protocol Support:** In addition to vCard 3.0 and CardDAV/CalDAV, Mycorrhizal adds full support for **vCard 4.0** and **JSContact**.
- **Flexible Export:** Granular selective field export so you can choose exactly which fields get exported for available formats.
- **Field Sensitivity:** Mark fields as private or secret to exclude them from exports and external sync entirely.
- **Serve Interactions as CalDAV:** Expose activities and life events to a calendar client, and eventually two-way calendar sync.

### Relationships, Households & Pets
- **Bidirectional Relationship Graphs:** Relationships are no longer strictly unidirectional. Creating a connection automatically maps it both ways and facilitates relationship-based searching.
- **Multi-Hop Graph Traversal:** Explore how you're connected to someone through intermediaries, not just direct links.
- **Household Tracking:** Automatically suggests relationships for contacts sharing the same address. Search by a household to pull lists for event invites, mail, or holiday cards.
- **Pets as Contacts:** Add pets directly to your CRM and search for owners using their pet's name with the relationship support.
- **Circles & Tags:** Two distinct grouping mechanisms — circles for the social groups a person belongs to, tags for free-form labeling.

### Data Management & Organization
- **Contact Merging:** Seamlessly merge duplicate contact records.
- **Custom Fields & Mappings:** Support for custom fields, including custom mappings to vCard fields to enable extended properties.
- **Repurposed General Notes:** Upstream Meerkat's journaling notes have been refactored into general notes that can be cleanly associated with any contact at a later point, including a capture inbox for notes you file later.
- **Full-Text Search:** SQLite FTS5 search across contacts, notes and addresses, with relationship synonyms and household scoping.
- **Bulk Operations:** Apply circle, tag and delete operations across many contacts at once.
- **One-Time Cross-User Sharing:** Share specific contacts with other users on the same instance, including granular selection of which fields are shared. *(Note: This is a one-time point-in-time copy/share to the target user rather than an ongoing real-time sync).*
- **Files & Documents:** Upload and associate documents and files with a contact
- **Full Backup & Restore:** `make backup` produces a consistent online SQLite snapshot (safe while the server runs), with a documented restore procedure covering the database, photos and attachments together. See [Deployment → Backups](https://drewbrunning.github.io/mycorrhizal-crm/deployment.html#backups).

### Staying In Touch
- **Cadence & Relationship Health:** Set how often you intend to be in touch with someone and see who has gone quiet. Cadence resets on a real interaction, not on ticking off a task.
- **Prep View:** A per-person briefing pulling together recent history, open agenda items and life events before you see or call someone.
- **Conversation Agenda:** Keep a running list of things to raise next time you talk to a given person.
- **Notification Channels:** Reminders can be delivered by email, [ntfy](https://ntfy.sh), [Gotify](https://gotify.net), or browser push — see [Notifications](#notifications) below.

### Tracking & Integrations
- **Expanded Life Event Reminders:** Automated reminders for major life events like anniversaries, complementing existing birthday tracking, organised into categories.
- **Gift Tracking:** Modeled after [Monica](https://github.com/monicahq/monica), allowing you to track gift ideas, past gifts given, and received items, with links and notes.
- **Immich Integration:** Link contacts directly to identified persons/faces in an [Immich](https://github.com/immich-app/immich) instance to easily view photos of individuals right from their profile.
- **External Links:** Deep-link a contact into other systems you run, with a configurable link-type registry (`tel:`, `sms:`, WhatsApp, and anything else you define).
- **Audit Trail:** A per-record history of what changed and when.

### On the roadmap

Not built yet. Listed so the feature set above can be read as a description of what exists rather than of what is intended:

- **Files & Documents:** Integrations with Seafile and Paperless-ngx via APIs and OwnCloud/NextCloud via WebDAV.
- **Two-Factor Authentication:** TOTP as a second factor on login. SSO via OIDC is available today as an alternative.
- **Native Android app client:** Phase 1 (core client) shipped 2026-08-10 in `android/` — login against the beta backend, contact list + detail, offline cache. Call/SMS tracking, quick-capture, and device-contacts import are planned follow-up phases.

---

## Notifications

Reminders can be delivered through four channels. Email is configured server-side; the other three are configured per user, in the app under **Settings → Notifications**, because each user has their own topic, token and devices.

| Channel | Configured | What you need |
|---|---|---|
| **Email** | Server (`.env`) | Either a [Resend](https://resend.com) API key, or SMTP host/credentials. Both may be set, in which case each email is sent through both. |
| **ntfy** | Per user, in-app | Your ntfy server URL and a topic. Works with the public ntfy.sh or a self-hosted instance. |
| **Gotify** | Per user, in-app | Your Gotify server URL and an application token. The token is stored encrypted at rest. |
| **Browser push** | Per user, in-app | Nothing to configure. The VAPID keypair is generated once on first use and stored in the database. |

`REMINDER_TIME` and `REMINDER_TIMEZONE` control *when* the daily reminder run happens; they apply to every enabled channel, not just email.

The only server-side setting for ntfy/Gotify/push is `WEBHOOK_BLOCK_PRIVATE_URLS`. It defaults to `false` so the server can reach a self-hosted ntfy or Gotify on a private address — set it to `true` on a multi-tenant or cloud deployment, where posting to internal addresses on user-supplied URLs would be an SSRF risk.

> **⚠️ Browser push requires HTTPS.** Push notifications are delivered to a service worker, and browsers refuse to register one on a plain-HTTP origin. `localhost` is exempt, so local testing works, but a LAN deployment reached over `http://` cannot register a device. The other three channels have no such requirement.

Because the app registers a service worker, it is served cache-first. A newly deployed version therefore announces itself with a "new version available" prompt instead of appearing silently — reload when you see it.

---

## Installation

### Docker (Recommended)

Mycorrhizal CRM ships as a single all-in-one image that bundles the frontend and
backend into one container, built locally from source (no published registry
image is required). The easiest way to run it is with Docker Compose:

1. **Download the Docker Compose file:**
    ```sh
    curl -O https://raw.githubusercontent.com/DrewBrunning/mycorrhizal-crm/main/docker-compose.yml
    curl -O https://raw.githubusercontent.com/DrewBrunning/mycorrhizal-crm/main/.env.example
    ```

2. **Configure environment:**
    ```sh
    # Copy the environment template
    cp .env.example .env

    # Edit with your settings
    nano .env
    ```

3. **Build and start the container:**
    ```sh
    docker compose up -d --build
    ```

4. **Access the application:**
    Open http://localhost:7300 in your browser.


## Contributing

### Bugs and feature requests
This application is currently in beta. Bugs are expected in testing, but are hopefully few and far-between. Please submit issues via GitHub to note 

### Development
To set up this repository for development, follow these steps:

1. **Clone the repository:**
    ```sh
    git clone https://github.com/DrewBrunning/mycorrhizal-crm.git
    cd mycorrhizal-crm
    ```

1. **Run the backend:**
Ensure you have [Go](https://golang.org/doc/install) installed. Then, set up your environment configuration:
   ```sh
    cd backend
    # Copy the example environment file and configure it with your settings
    cp .env.example .env
    
    # Install dependencies and run
    go mod tidy
    source .env
    go run main.go
   ```
   The project uses an SQLite database for storage. Database migrations run automatically on startup.


1. **Run the frontend (in a second terminal):**
   ```sh
   cd frontend

   yarn install
   yarn start
   ```

You can find a more comprehensive overview for developers in the [developer README](README-developer.md).
