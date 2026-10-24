# MeetSpace website content audit

Checked **19 October 2026** against this checkout and the dedicated client
repositories. The content brief is
`/Users/aleksandranickij/Downloads/CODEX_PROMPT_MEETSPACE_WEBSITE_CONTENT.md`.
The user's direct request for an English landing takes precedence over the
brief's Russian-language examples. This audit verifies implementation and
published files; it is not a live-service acceptance test.

## Current feature matrix

“Implemented” means a UI and server contract exist. Runtime availability can
still depend on server configuration, permissions, devices and network access.

| Feature from the brief                      | Status                                              | Evidence                                                                                                                                                                                  | Public copy boundary                                                                                                                                                                                                     |
| ------------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Video meetings                              | Implemented                                         | `frontend/src/pages/ConferencePage.tsx`, `frontend/src/components/conference/ActiveConferenceRoom.tsx`, `frontend/src/media.ts`, `docs/media-sfu.md`                                      | Say video meetings; do not promise unlimited capacity, guaranteed resolution or network-independent operation.                                                                                                           |
| Screen sharing and moderation               | Implemented                                         | `frontend/src/media.ts`, `docs/conference-recording.md`                                                                                                                                   | Camera and screen can run together. Capture requires explicit user selection and browser/OS support. Moderator policy applies; configured default is one screen sharer. System audio is not available on every platform. |
| Meeting chat                                | Implemented                                         | `frontend/src/components/ChatPanel.tsx`, `internal/transport/http/chat_routes.go`                                                                                                         | Messages, history and attachments follow meeting access rules. Do not call it end-to-end encrypted.                                                                                                                      |
| Direct messages                             | Implemented                                         | `frontend/src/pages/PersonalPage.tsx`, `internal/infrastructure/postgres/personal_repository.go`, `docs/PERSONAL_MESSAGES.md`                                                             | Registered accounts, membership checks, replies, own-message editing/deletion, read state and persistent history. Guest meeting sessions do not unlock personal chats.                                                   |
| Group chats                                 | Implemented                                         | `frontend/src/components/GroupChats.tsx`, `docs/GROUP_CHATS.md`                                                                                                                           | Registered accounts; owner/admin/member roles and membership controls. This is not an organization-wide administration or workspace-role system.                                                                         |
| Personal folders                            | Implemented for meetings **and direct/group chats** | `frontend/src/types.ts`, `frontend/src/components/FolderItems.tsx`, `internal/infrastructure/postgres/folder_repository.go`, `docs/PERSONAL_FOLDERS.md`                                   | Users can organize all three in their own folders. Folders are private organization, not shared team folders, separate chats or a shared file drive.                                                                     |
| Calendar                                    | Implemented                                         | `frontend/src/pages/CalendarPage.tsx`, `frontend/src/components/ScheduleFields.tsx`                                                                                                       | Calendar for scheduled MeetSpace meetings. Do not imply universal Google/Outlook synchronization.                                                                                                                        |
| Recording and playback                      | Implemented; server capabilities required           | `frontend/src/components/RecordingPanel.tsx`, `frontend/src/pages/RecordingDetailPage.tsx`, `internal/usecase/recordings/service.go`, `docs/conference-recording.md`                      | An admitted, joined participant with a full account can start a recording when the server provides recording modes. Guests cannot control recordings. Processing/storage must be configured; completion is asynchronous. |
| File sharing | Implemented as message attachments | `frontend/src/attachmentPolicy.ts`, `internal/domain/chat/attachment_types.go`, `docs/ATTACHMENTS.md` | Up to five attachments per message, each at most 10 MiB. Images, PDF, Office/OpenDocument/text files and the listed archives are supported. Container signatures are checked without unpacking. Do not imply video uploads, antivirus scanning or cloud-drive functionality. |
| Registration and invited guests             | Implemented                                         | `frontend/src/pages/AuthPage.tsx`, `frontend/src/pages/InvitePage.tsx`, `frontend/src/pages/PreJoinPage.tsx`, `internal/app/auth/`                                                        | Accounts are needed for personal workspace features; invitation-based guests can join an authorized meeting. Do not claim registration is mandatory for every meeting attendee.                                          |
| External calendars                          | Partial / configuration-dependent                   | `frontend/src/components/IntegrationsSettings.tsx`, `internal/transport/http/integration_routes.go`, `internal/usecase/integrations/service.go`                                           | Providers have noop/mock/http modes. A configured UI/API is not proof of live external synchronization.                                                                                                                  |
| Transcripts, summaries, captions, analytics | Partial / configuration-dependent                   | `frontend/src/components/RecordingInsights.tsx`, `frontend/src/components/CaptionsPanel.tsx`, `frontend/src/pages/AnalyticsPage.tsx`, `internal/transport/http/platform_status_routes.go` | Capability flags and external processing configuration matter. Omit unconditional AI claims from this landing.                                                                                                           |
| Android                                     | Published beta APK                                  | Dedicated Android repository; `frontend/src/landing/downloads.ts` and `docs/landing-release-audit.md`                                                                                     | `0.3.7-beta`, Android 8.0+, universal APK. No verified Play Store listing or production-signing claim.                                                                                                                   |
| Windows                                     | Published unsigned beta test installers             | Dedicated desktop repository; download manifest/audit                                                                                                                                     | `0.1.1-beta`, x64 and ARM64 EXE. Package availability does not mean every target OS has passed installation and runtime acceptance.                                                                                      |
| Linux                                       | Published beta test packages                        | Dedicated desktop repository; download manifest/audit                                                                                                                                     | `0.1.1-beta`, DEB/RPM/AppImage for x64 and ARM64. Package-specific desktop and sandbox requirements apply.                                                                                                               |
| Docker self-hosting and custom domains      | Implemented; deployment configuration required      | `docker-compose.yml`, `docker-compose.production.yml`, `docs/operations/meeting-host-deployment.md`, `docs/landing-self-hosted-audit.md`                                                  | Linux host, Docker Compose, HTTPS and network/TURN configuration; use the actual documented setup. Not a universal one-click deployment.                                                                                 |
| Free service, license, commercial terms     | Not confirmed                                       | No project license file in this checkout; no pricing contract verified                                                                                                                    | Do not invent a free tier, MIT/Apache license, commercial permission, SLA or unlimited-use promise.                                                                                                                      |

No required core feature above needs a fictional “in development” state.
Do not mark published client builds “coming soon” merely because their GitHub
Releases asset arrays are empty: the binaries are published in repository
folders and have already been verified in this session.

## Folder scope and historical documentation

The new brief's folder description is supported by current code:

- `FolderItemKind` is `conversation | conference`.
- `PersonalConversation` includes `direct | group`.
- `FolderItems` displays group titles or direct peers and routes both to the
  existing `/personal/:id` page; meetings use their existing meeting route.
- `folderConversationsSQL` explicitly selects both direct and group types.
  `folderConferencesSQL` selects meetings; all folder operations are scoped
  to the current account and existing access rights.
- One item can be in multiple folders. Removing a folder or its membership
  does not delete the underlying conversations or meetings. Files and
  recordings do not automatically become standalone folder items.

Earlier statements in `docs/GROUP_CHATS.md` that folders are future work are
historical stage notes, superseded by `docs/PERSONAL_FOLDERS.md` and current
code. The same applies to the previous version of this audit, whose client
availability was based only on the server checkout.

## Recording, chat and storage wording

Recording is **not owner-only**. Current UI permits a joined, admitted account
holder to start an active meeting's recording when the server advertises a
recording mode. The initiator or meeting owner can stop it; completed-recording
delete permissions follow the same ownership distinction. A visible recording
indicator and start announcement exist. Guests cannot start/stop recording.
Do not promise immediate processing, unlimited retention or autonomous consent
management.

Messages and membership state are persisted in PostgreSQL. File/recording
objects use the configured object storage, with MinIO present in the standard
Compose stack; a recorder spool is also configured. Redis and RabbitMQ support
realtime coordination and background processing. On a self-hosted deployment,
the operator controls these resources. This does not imply end-to-end
encryption, data being stored only on the user's device, or a guarantee that
provider integrations never send data elsewhere.

## Download installation copy

The existing download audit already verifies nine binaries. No additional
binary downloads or execution were needed for this content review.

| Platform | Suitable requirements / installation wording                                                                                                                                                                                                                           | Avoid                                                                                                                                                                     |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Android  | Android 8.0 or newer (`minSdk = 26`); universal APK contains `arm64-v8a`, `armeabi-v7a`, `x86`, `x86_64`. Download the beta APK, use Android's normal installer, then enter the server's root HTTPS URL and sign in.                                                   | A Play Store claim, automatic updates, a production signer or guaranteed upgrade compatibility. Historical release docs are not signing evidence for the current APK.     |
| Windows  | Select x64 or ARM64 EXE. Documented targets are Windows 10 **22H2 x64**, Windows 11 x64 and Windows 11 native ARM64. Open the matching installer and choose an HTTPS server. Keep “Unsigned beta test build” visible.                                                  | Certified compatibility on every target, x86, Windows 7/8/8.1, Windows 10 ARM64, signed production release, automatic updates, or instructions to disable OS protections. |
| Linux    | DEB for Ubuntu/Debian/Mint families, RPM for Fedora, or AppImage; choose the matching x64/ARM64 package. Use a graphical session, unlocked GNOME Keyring/KWallet and working Electron sandbox. On Wayland, screen sharing needs PipeWire and a working desktop portal. | Treating a package's existence as full distro certification; claiming an APT/DNF repository or built-in updater. Never recommend `--no-sandbox` or disabling AppArmor.    |
| Web      | Open the deployment's HTTPS URL in a browser that supports WebRTC. Microphone/camera permission is needed when using those devices; screen sharing support varies.                                                                                                     | A universal browser/version compatibility guarantee without testing.                                                                                                      |

Desktop evidence:
[README](https://github.com/janickiy/meet-space-win/blob/main/desktop/README.md),
[Windows installer acceptance](https://github.com/janickiy/meet-space-win/blob/main/docs/desktop/06_WINDOWS_INSTALLERS_CURRENT.md),
[Linux QA](https://github.com/janickiy/meet-space-win/blob/main/docs/desktop/linux/07_FINAL_QA.md).
Android requirements:
[app configuration](https://github.com/janickiy/meet-space-android/blob/ba89d3cd8ba91ad7f1b7f2c89cb56d21aee7aa01/app/build.gradle.kts).
Actual checks and checksum sources remain in `docs/landing-release-audit.md`.

## Content and route integration constraints

Use the existing application routes for account actions: `/register`, `/login`,
`/meetings/new`, `/personal`. Account-gated routes should be described as an
action the user completes after sign-in, not an anonymous workspace preview.

The public structure can use `/`, `/download`, `/self-hosted` and
`/docs/self-hosted`, with capabilities and FAQ as home-page sections; `/welcome`
remains the existing public alias. The current implementation provides a
public prerender path and per-page metadata. The previous audit's assertion
that the project has no prerender support is obsolete.

The supplied copy can be translated faithfully while qualifying broad phrases
such as “from any device.” User quotes, customer logos, performance figures,
security absolutes and pricing must not be fabricated. Business and education
sections may describe use cases, without claiming unimplemented enterprise
administration, LMS features or compliance certifications.
