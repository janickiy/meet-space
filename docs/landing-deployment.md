# MeetSpace landing deployment

Updated on 24 October 2026 at **https://meet-space.janickiy.com/**.

The landing is a static Apache site on the existing host `93.89.223.151`.
The application remains at `https://meeting.janickiy.com`.

## Build configuration

Use the Node version required by `frontend/package.json` and run:

```sh
VITE_MARKETING_ONLY=true \
VITE_PUBLIC_SITE_URL=https://meet-space.janickiy.com \
VITE_PUBLIC_APP_URL=https://meeting.janickiy.com \
npm --prefix frontend run build

VITE_MARKETING_ONLY=true \
VITE_PUBLIC_SITE_URL=https://meet-space.janickiy.com \
VITE_PUBLIC_APP_URL=https://meeting.janickiy.com \
node frontend/scripts/check-public-build.mjs
```

Marketing-only mode renders the root landing without account recovery or API
providers. Account and workspace actions use absolute application URLs. The
default application build retains its existing account behavior.

## Latest desktop download update

The current release is `landing-20261025T095000Z-desktop-016`, published on 25 October 2026 (Moscow time).
Windows and Linux now publish version `0.1.6-beta`; all 42 earlier packages remain
in their original version folders. The installer determines the public IPv4 from
the verified SSH host and checks it again before preparing the plan. Step-three
fields are grouped, Back aligns with the content, and the sidebar slogan is removed.
Explicit saved-job recovery is preserved. Windows EXE and Linux DEB/RPM/AppImage
remain available for x64 and ARM64. Source GUI validation covered 20 groups,
39 captured views and 45 layout checks; this is separate from native installation
acceptance of the published packages.

Immutable Desktop package commit: `855374bfe9067dff12b44acf1b3ee646097910ef` in
`janickiy/meet-space-desktop/source`. Source commit:
`4e3a816cb150a9e11f96972781a6b99c1a737569`. The 440 exact build inputs have SHA-256
`3b2ff58a85b29be66f4990df5c0197dc07985d6ff7d752edb9fe785d9c4de5ad`.
All eight public GitHub downloads passed full SHA-256, byte-size and binary/archive
magic checks before activation. The publication check did not execute installers.
These remain unsigned beta packages; automatic updates are disabled.

The metadata-only update copies the current published static artifact into a new
release with immutable asset namespace `desktop-016-20261025t095000z`. Both the
prerendered Windows x64 / Linux DEB x64 cards and all eight client-side download
choices contain the new names, version, URLs, sizes and checksums. Platform links point to real immutable source directories, rather than release tags. Android 0.3.11-beta, existing
assets, architecture punctuation and site verification files remain byte-identical.
The previous release `landing-20261024T195300Z-desktop-015` is retained for rollback.
No application containers, databases or Apache configuration changed.

`frontend/src/landing/downloads.ts` matches the eight published Desktop records.
Evidence is in `frontend/test-results/landing/version-016-downloads-20261024/`:
full public-download verification, catalogue verification, staging and activation
reports, and live HTTPS checks. This evidence covers automated package and live-site metadata checks; browser visual
acceptance is separate.

The publication verifies the deployed static artifact directly over HTTPS.
The last full backend/web build was blocked by pre-existing errors outside these
files: `auth.tsx` imports the absent `accessTokenTTLSeconds` export and
`MeetingInvitationLink.tsx` imports the missing `qrcode.react` dependency.
Those application sources were not changed. Compilation and GUI acceptance are
separate checks; this helper does not claim that a full backend/web build or
real-host installer acceptance was performed.

## Previous architecture punctuation update

That server release was `landing-20261023T074412Z-architecture-dashes`,
published on 23 October 2026. The seven long dashes in Architecture > Application
services were replaced with ASCII hyphens in HTML and client bundles.
The local source correction is in `frontend/src/landing/docs-product.ts`.
Its isolated TypeScript/Vite build and all 17 public checks passed.

The server had advanced to `landing-20261023T063921046564Z-android-0.3.11-beta`
through separate publications. The punctuation fix was applied to that current
artifact, preserving the latest Android package, verification files and all
other content. A new immutable asset namespace
`architecture-dashes-20261023t074412z` preserves historical cached assets.
Only the seven punctuation characters and current resource URLs changed in HTML;
eight copied client bundles received the same seven replacements. The release
contains 1571 public files. The previous server release is retained for rollback.

The isolated local build `/private/tmp/meetspace-architecture-dashes.qco30bph/frontend`
is based on older source and is NOT a source baseline for current download
metadata. Future publications must preserve the current server artifact or
reconcile the separately published updates first. Live HTTP verification checked
all seven rendered replacements, matching client bundle text, JS syntax and
continued presence of Android 0.3.11-beta. Evidence is in
`frontend/test-results/landing/architecture-dashes-verification.json`.

## Previous Google verification file update

The preceding release is `landing-20261023T032258Z-google`, published on
23 October 2026. The user-provided `googleeb03dacd0a054c05.html` is served at
`https://meet-space.janickiy.com/googleeb03dacd0a054c05.html` and saved in
`frontend/public/` for subsequent builds. Its original 53 bytes are unchanged;
SHA-256 `1291716c12f63d69ff1c43c8b61b88c9e2e0c11eddeb154c658e4e5bbfd52fa7`.

The static file-only release preserves all 436 preceding public files byte
for byte and adds the Google file. All 437 packaged public files passed
remote checksum validation. Live HTTPS GET returns 200 and text/html with
exact original content. The Yandex verification file and home page remain
byte-identical to the preceding release. No Search Console verification
request was submitted.

The archive is `release-artifacts/landing/landing-20261023T032258Z-google.tar.gz`,
SHA-256 `71176eda5e5ceb7511856605adbfce61a94f00196b941d6f170574858a6e1f0a`.
The previous release `landing-20261023T031512Z-yandex` is retained for rollback.
The clean source baseline is `/private/tmp/meetspace-google.smzpw9kq/frontend`;
verification evidence is in `frontend/test-results/landing/google-verification.json`.

## Previous Yandex verification file update

The preceding release is `landing-20261023T031512Z-yandex`, published on
23 October 2026. The user-provided `yandex_0925aaeefa343feb.html` is served at
`https://meet-space.janickiy.com/yandex_0925aaeefa343feb.html` and is also saved
in `frontend/public/` so future builds retain it. Its original 161 bytes are
unchanged; SHA-256:
`2a598464c5e110af783d636adb77d3a880c8fdfcdff832d93c6b97d7e270c322`.

This static file-only release copies the preceding site's 435 files byte for
byte and adds the verification file. It required no application rebuild or
web-server configuration changes. All 436 packaged public files passed remote
checksum validation before the atomic release switch. A live HTTPS GET returns
200 with text/html and exactly matches the supplied file; the home page still
matches the preceding release.

The archive is `release-artifacts/landing/landing-20261023T031512Z-yandex.tar.gz`,
SHA-256 `c06b1b7c116f18b045b193f58557e0b8f84009df4796e34d3424f253c7eeb5cd`.
The previous `landing-20261023T013226Z-downloads` release is retained for rollback.
A clean source baseline, including the new public file, is available at
`/private/tmp/meetspace-yandex.ae7bmwyz/frontend`. Verification evidence is in
`frontend/test-results/landing/yandex-verification.json`. No Yandex account
verification request was submitted; this change only publishes the supplied file.

## Previous client download update

The preceding release is `landing-20261023T013226Z-downloads`, published on
23 October 2026. Shared download cards now use all nine current client package
records: Windows/Linux 0.1.2-beta and Android 0.3.8-beta. URLs are pinned to
the published commits, with current sizes and SHA-256 values. Desktop links
use the renamed canonical `janickiy/meet-space-desktop` repository. OS target
labels and beta/signature notices are preserved.

The release archive is
`release-artifacts/landing/landing-20261023T013226Z-downloads.tar.gz`, SHA-256
`8893b0a863301b14c994dee45e825834fe8ba5506afebafcdd730fb3432abd90`.
The `current` symlink points to its `site` directory. The previous release
`landing-20261023T012622Z-installation` is retained for rollback.

Eight desktop URLs returned actual binaries via HTTP range requests;
Content-Range sizes and LFS object hashes matched publisher metadata.
Android was fully downloaded and rehashed. No installer was executed.
Independent review confirmed Windows requirements and retained the limits of
Linux compatibility claims. See `docs/landing-release-audit.md` for details.

The isolated source is in `/private/tmp/meetspace-download-update.nd2i_yfh/frontend`.
Unrelated pending edits were excluded. Build, formatting and all 17 public
checks passed; all 435 live public files matched release checksums. Only the
shared download cards and canonical desktop repository references changed in
public HTML. All documentation and server release metadata remain unchanged.
Browser verification covered nine selection states (Windows, Android and the
three Linux distributions on both architectures); version, URL and SHA-256
matched each selected file. Live browser verification confirmed new versions,
file sizes, working links and no horizontal overflow.

Evidence is in `frontend/test-results/landing/download-update-audit.json`,
`download-update-ui.json`, `download-update-build.log`,
`published-download-update.jpg` and `live-deployment.json`.

## Previous installation documentation update

The preceding release is `landing-20261023T012622Z-installation`, published on
23 October 2026. The installation article and its `/docs/self-hosted` alias
now reflect the live installer: prebuilt Docker images by default, explicit
source builds, automatic IPv4 detection, default project settings, optional
SMTP, Enter confirmation, current management/resume commands and manual
release deployment. Existing section anchors and the simple wget download
command remain available. Only this article's review date changed.

The live bootstrap and matching prebuilt manifest were checked against commit
`21530d396a8112ecfc46803b3ecb1c5870c67853` and prerelease
`v0.4.0-prebuilt.20261023.1`. Documentation and scripts were read only; the
installer was not executed and no server application was installed or updated.
The bootstrap SHA-256 matched its published checksum:
`584bbd5ab8f32c4b900fd6466cabd8a9ac6de9f14b319f539c128915f4eda889`.

The previous release `landing-20261023T011336Z-footer-author` remains available
for rollback. The archive is
`release-artifacts/landing/landing-20261023T012622Z-installation.tar.gz`, SHA-256
`c46ba651f2929ef8dde9daffef4faec074ea82bf4f7078acc1e687b892dce3e9`.
The `current` symlink points to this release's `site` directory.

The isolated build is in `/private/tmp/meetspace-install-update.ih8h_f_c/frontend`.
Unrelated pending source edits were preserved and excluded from publication.
TypeScript, Vite, prerendering, formatting and all 17 public-document checks
passed. Only the installation page and its alias changed, apart from hashed
asset references. All 414 live public files matched release checksums. Browser
checks confirmed the new content and no horizontal overflow at 1280 and 390 px.
Evidence: `frontend/test-results/landing/installation-update-build.log`,
`installation-update-audit.json`, `published-installation-update.jpg` and
`live-deployment.json` in the same directory.

## Previous footer author update

The preceding release is `landing-20261023T011336Z-footer-author`, published on
23 October 2026. The shared landing footer now reads
`© 2026 MeetSpace · Яницкий Александр`, with the `Автор:` prefix removed.
The source change is in `frontend/src/pages/Landing.tsx`.

The prior release `landing-20261020T170919Z-overview` is retained for rollback.
The archive is `release-artifacts/landing/landing-20261023T011336Z-footer-author.tar.gz`.
An isolated copy of the previously published source was rebuilt, preserving
unrelated pending source edits. All 17 public-document checks passed; comparison
confirmed only the requested footer copy and generated asset names changed.
All 393 published files matched checksums. Live browser verification on
`/download` confirmed the final footer text after client-side rendering.

## Previous documentation overview update

The current release is `landing-20261020T170919Z-overview`, published on
20 October 2026. The overview now explains MeetSpace, how to choose a guide
and documentation coverage in clear Russian with emphasis and a short list.
The three existing anchors, four quick-start cards, repository and release
links are preserved. The date is removed from overview prose and header
metadata on both `/docs` and `/docs/overview`; the adjacent separator is also
omitted. Other articles retain their existing metadata dates.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T170919Z-overview`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T170151Z-before-start` is retained for rollback. The local archive
is `release-artifacts/landing/landing-20261020T170919Z-overview.tar.gz`, SHA-256
`68fbef01932dbb5439a8e2d547296e487ac91d0921206f4f4e4233563aeb8953`.

Formatting, production build and all 17 public-document checks passed. All
372 published files matched release checksums. Both overview HTML documents
were checked for absence of «20 октября» and preservation of anchors, quick
links and source references. All other public HTML is unchanged apart from
hashed resource references. The isolated build is in
`/private/tmp/meetspace-overview.3czg6oj3/frontend`, preserving separate pending
source edits. Live browser verification confirmed the new sections, four cards,
nine bold spans, no date in body/header metadata and no horizontal overflow.
Evidence is in `frontend/test-results/landing/overview-build.log`,
`frontend/test-results/landing/overview-qa.json`,
`frontend/test-results/landing/published-overview.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous before-you-begin documentation update

The preceding release is `landing-20261020T170151Z-before-start`, published on
20 October 2026. «Перед началом работы» was rewritten in plain Russian with
five preparation sections, two DNS/HTTPS subheadings, structured lists and
emphasis. All five existing anchors and seven help/source links remain intact.
The article preserves the fresh-only installer/operator-release distinction,
server and rootful Docker prerequisites, strict installer DNS/network checks,
certificate/firewall/media requirements, complete recovery materials,
disabled-by-default integrations and cross-network functional acceptance.
These requirements were preserved from the pinned installer article and
neighboring documentation, without claiming a new installer-code audit.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T170151Z-before-start`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T165144Z-support-contacts` is retained for rollback. The archive is
`release-artifacts/landing/landing-20261020T170151Z-before-start.tar.gz`, SHA-256
`4a12835a8ee342fd9d558ac1930dd57a9557766a946bcec968dec1b605fc1326`.

Formatting, production build and all 17 public-document checks passed. All 351
published files matched release checksums. Other public HTML copy is unchanged
apart from generated resource references. Inline emphasis now renders in main
section bullets, steps and notes. The isolated build is in
`/private/tmp/meetspace-before-start.vqqxlllv/frontend`, preserving separate
pending source edits. Browser checks at 1280 and 320 px confirmed all anchors,
twenty list items, twenty-five bold spans, readable 17/16 px body text and no page
overflow or visible formatting markers. Evidence is in
`frontend/test-results/landing/before-start-build.log`,
`frontend/test-results/landing/before-start-qa.json`,
`frontend/test-results/landing/published-before-start.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous technical support page update

The preceding release is `landing-20261020T165144Z-support-contacts`, published
on 20 October 2026. The support page now uses the user-provided technical support
and individual development copy, with janickiy@mail.ru and @janickiy contacts.
Two CTA buttons link to `mailto:janickiy@mail.ru` and `https://t.me/janickiy`.
The email action has no new-tab target; Telegram uses a protected new tab.
The instruction recommending two buttons and pasted invisible glyphs are not
public content. The original intro anchor is preserved; a visually hidden plain
heading avoids a duplicate visible title and an invisible keyboard-focus stop.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T165144Z-support-contacts`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T164325Z-storage-queues` is retained for rollback. The archive is
`release-artifacts/landing/landing-20261020T165144Z-support-contacts.tar.gz`,
SHA-256 `4ab1181ede4d62cef0e598ac7cf8c0ac0d2f120384823e03e60c419953c2e72a`.

Formatting, production build and all 17 public-document checks passed. All 330
published files matched their release checksums. Other public HTML copy remains
unchanged apart from generated resource references. The isolated build is in
`/private/tmp/meetspace-support.y5_drt5r/frontend`, preserving separate pending
source edits. Browser checks confirmed exact public copy, contact hrefs, protected
Telegram target, no focusable links inside hidden headings and no page overflow.
At 320 px, buttons stack, fit the 272 px content width and remain 48.5 px high.
No message was sent and neither external contact action was activated.
Evidence is in `frontend/test-results/landing/support-build.log`,
`frontend/test-results/landing/support-qa.json`,
`frontend/test-results/landing/published-support-contacts.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous storage and queues documentation update

The preceding release is `landing-20261020T164325Z-storage-queues`, published
on 20 October 2026. «Хранилище и очереди» was rewritten in plain Russian in the
same style as the other updated architecture sections. It has three h3
subsections covering stored data, queues/background tasks and backup/recovery,
with nine bullets, four paragraphs and fifteen bold spans. The existing
`persistent-data` anchor is unchanged.

The rewrite preserves the distinction between PostgreSQL metadata and object
file bytes, persistent recording spool, temporary Redis state, acknowledged
RabbitMQ recording commands and outbox, separate durable PostgreSQL jobs with
bounded retries/leases, incompatible MinIO/SeaweedFS disk formats, and the
complete set of backup data and integration encryption keys. These details were
reviewed against the repository architecture/data-flow documentation.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T164325Z-storage-queues`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T161859Z-application-services` is retained for rollback. The archive
is `release-artifacts/landing/landing-20261020T164325Z-storage-queues.tar.gz`,
SHA-256 `a6c685db0c2c42362755ba15e3d7dc7894be7676d6aba56f94ed43798e5c7a16`.

Formatting, production build and all 17 public-document checks passed. All 308
published files matched release checksums. Other public copy is unchanged,
apart from automatic reading-time metadata and generated asset references. The
isolated build is in `/private/tmp/meetspace-storage-queues.enpfjr_j/frontend`,
leaving separate pending source edits intact. Browser checks at 1280 and 390 px
confirmed structure, emphasis, retained 17/16 px body text and no page overflow.
Evidence is in `frontend/test-results/landing/storage-queues-build.log`,
`frontend/test-results/landing/storage-queues-qa.json`,
`frontend/test-results/landing/published-storage-queues.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous application services documentation update

The preceding release is `landing-20261020T161859Z-application-services`, published
on 20 October 2026. «Сервисы приложения» now uses the exact user-provided copy:
two introductory paragraphs, the h3 «Основные компоненты», seven component bullets
and two concluding paragraphs, with ten bold spans. The existing anchor is
preserved. Subsection lists render emphasis and participate in documentation search.
The supplied em dashes are retained as part of the latest requested copy.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T161859Z-application-services`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T161411Z-deployment-model` is retained for rollback. The local
archive is `release-artifacts/landing/landing-20261020T161859Z-application-services.tar.gz`,
SHA-256 `f80300fbbb80e24838f19cf145343011af3055d2741ecf885210bf682dfb903c`.

Formatting, production build and all 17 public-document checks passed. All 287
published files matched release checksums. Comparison with the previous HTML
confirmed no copy changed outside the requested section, apart from automatic
reading-time metadata and generated asset references. The isolated build is in
`/private/tmp/meetspace-application-services.ff40jr3a/frontend`; separate pending
source edits are preserved. Browser verification confirmed four paragraphs,
seven bullets, ten bold spans, correct element order and no page overflow.
Searching «Live Worker» returns this section. Evidence is in
`frontend/test-results/landing/application-services-build.log`,
`frontend/test-results/landing/application-services-qa.json`,
`frontend/test-results/landing/published-application-services.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous deployment model documentation update

The preceding release is `landing-20261020T161411Z-deployment-model`, published
on 20 October 2026. The architecture section «Модель развёртывания» now contains
the exact user-provided eight paragraphs, the h3 «Технологическая архитектура»
and six bold spans. The `deployment-model` anchor and existing text diagram
remain unchanged. All other published copy and enlarged typography are preserved.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T161411Z-deployment-model`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T150709Z-security-monitoring` is retained for rollback. The archive
is `release-artifacts/landing/landing-20261020T161411Z-deployment-model.tar.gz`,
SHA-256 `ccf6d383adbadc4acb440105c02945922fb70fd2421d589e1ddbed85de4598fb`.

Formatting, production build and all 17 public-document checks passed. All
266 published files matched their release checksums. Comparison with prior HTML
confirmed only the requested section, its table-of-contents title and automatic
reading-time estimate differ apart from generated asset references. The isolated
build is in `/private/tmp/meetspace-deployment-model.i6rep2j6/frontend`, preserving
separate pending working-tree edits. Live browser checks confirmed eight
paragraphs, one h3, six bold spans, the diagram, 17 px body text and no horizontal
overflow. Evidence is in `frontend/test-results/landing/deployment-model-build.log`,
`frontend/test-results/landing/deployment-model-qa.json`,
`frontend/test-results/landing/published-deployment-model.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous security and monitoring documentation update

The preceding release is `landing-20261020T150709Z-security-monitoring`, published
on 20 October 2026. Architecture documentation now uses the requested headings
«Безопасность и защита данных» and «Мониторинг, производительность и стабильность
работы», with the exact user-provided text. Existing anchors are unchanged.
The security block contains five paragraphs and five bold spans. The monitoring
block has nine paragraphs, two h3 subsections, three bold spans and its three
existing source links. Paragraph emphasis is rendered as semantic strong elements;
subsection text participates in documentation search. Reading time updates from
five to six minutes automatically.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T150709Z-security-monitoring`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T145509Z-docs-typography` is retained for rollback. The archive is
`release-artifacts/landing/landing-20261020T150709Z-security-monitoring.tar.gz`,
SHA-256 `17bd9b0d2cd72bac832151869a07e140ea35d16cc1d4f52aee3da6b190abf6ea`.

Formatting, the production build and all 17 public-document checks passed.
All 245 published files matched release checksums. Comparison against prior HTML
confirmed only the two requested blocks, their table-of-contents titles and
calculated reading time changed apart from generated asset references. An
isolated build in `/private/tmp/meetspace-security-copy.2vs82bys/frontend` leaves
unrelated pending working-tree copy edits intact. Browser verification confirmed
paragraphs, emphasis, subheadings and links; a search for «масштабирование» finds
the monitoring section. Desktop 1280 px and mobile 390 px have no page overflow.
Evidence is in `frontend/test-results/landing/security-copy-build.log`,
`frontend/test-results/landing/security-monitoring-qa.json`,
`frontend/test-results/landing/published-security-copy.jpg`,
`frontend/test-results/landing/published-monitoring-copy.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous documentation typography update

The preceding release is `landing-20261020T145509Z-docs-typography`, published
on 20 October 2026. Documentation article text is now 17 px on desktop and
16 px on phones. Section headings, article descriptions, navigation, table
text, notes and supporting labels were enlarged. Architecture diagram labels
are 13–16 px instead of 8–12 px; service cards stack on phones while storage
cards use two columns.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T145509Z-docs-typography`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T142526Z-architecture-copy` is retained for rollback. The archive is
`release-artifacts/landing/landing-20261020T145509Z-docs-typography.tar.gz`, SHA-256
`239eb16da1ee56e1f04b996bbf5f13db5a5422e748e3dce3654908ea7d3d517f`.

All 17 public-document checks passed and all 223 published files matched release
checksums. All published HTML copy was preserved. The isolated source build is
in `/private/tmp/meetspace-docs-type.nerYDQ/frontend`; separate pending source-copy
edits remain outside this style update. Browser checks at 1280, 390 and 320 px
confirmed larger font sizes, no horizontal page overflow and no clipped diagram
labels. Evidence is in `frontend/test-results/landing/docs-type-build.log`,
`frontend/test-results/landing/docs-typography-qa.json`,
`frontend/test-results/landing/published-docs-typography.jpg` and
`frontend/test-results/landing/live-deployment.json`.

## Previous architecture wording update

The preceding release is `landing-20261020T142526Z-architecture-copy`, published
on 20 October 2026. In the architecture documentation, PostgreSQL storage now
uses «данные конференций», and the sixth step of «Как работает встреча» has been
removed. The live list contains five steps.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T142526Z-architecture-copy`;
the `current` symlink points to its `site` directory. The preceding release
`landing-20261020T134025Z-invitation-copy` is retained for rollback. The local archive
is `release-artifacts/landing/landing-20261020T142526Z-architecture-copy.tar.gz`,
SHA-256 `d1c66ca115da7962fbadb985f0f3354d18217293d60ed9c8f5e71a55ac4205a3`.

All 17 public-document checks passed, and all 201 published files matched release
checksums. Old hashed resources remain available for cached visitors. An isolated
build in `/private/tmp/meetspace-postgres-copy.j8aRfv/frontend` preserved the
published guest-session and attachment-format wording, leaving separate pending
working-tree edits intact. Comparison against the previous HTML confirmed only
the two requested text changes apart from generated resource references. Live
browser verification confirmed the PostgreSQL phrase and five ordered steps.
Evidence is in `frontend/test-results/landing/postgres-copy-build.log`,
`frontend/test-results/landing/live-deployment.json` and
`frontend/test-results/landing/published-architecture-flow.jpg`.

## Previous invitation wording update

The preceding release is `landing-20261020T134025Z-invitation-copy`, published on
20 October 2026. The invitation paragraph in `/docs/features#access-and-moderation`
now says «могут отправлять приглашения», removing only the word «явные».

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020T134025Z-invitation-copy`;
the `current` symlink points to its `site` directory. The previous release
`landing-20261020-copy-compat.1` is retained for rollback. The archive is
`release-artifacts/landing/landing-20261020T134025Z-invitation-copy.tar.gz`, with
SHA-256 `436204c12bd10acf486c7794d0f5cd7662543e270d9d5224196ed762ccb7ce72`.

All 17 public-document checks passed, and all 169 published files matched the
release checksums. Old hashed assets are retained for cached visitors. Comparison
with the preceding release confirmed no other HTML copy changed. The exact
requested paragraph was verified in the live browser. Evidence is in
`frontend/test-results/landing/invitation-copy-build.log`,
`frontend/test-results/landing/live-deployment.json` and
`frontend/test-results/landing/published-invitation-copy.jpg`.

## Previous text-only update

The preceding release is `landing-20261020-copy-compat.1`, published on
20 October 2026. The final call to action now reads «Начните общение.».
The operating-system FAQ lists Android 8.0+, Windows 10 22H2/11 x64,
Windows 11 ARM64, Ubuntu 22.04/24.04 LTS x64, Debian 12/13 x64,
Fedora Workstation 44 x64 and Ubuntu 24.04 LTS ARM64. Linux Mint 22.3 x64
is explicitly identified as Live-mode testing; complete desktop feature
acceptance is not claimed.

The release is stored at
`/var/www/meet-space.janickiy.com/releases/landing-20261020-copy-compat.1`;
the `current` symlink points to its `site` directory. The previous release
listed below is retained for rollback. The local archive is
`release-artifacts/landing/landing-20261020-copy-compat.1.tar.gz` with SHA-256
`986667ac2fe63f2c9d7ed3d5678fcbbca7bcb95aae10728a1802a1164ef03fe8`.

All 17 public-document build checks passed. All 148 published files matched
their checksums; old hashed assets are retained for visitors with cached pages.
Browser checks passed at 1440, 390 and 320 px without horizontal overflow or
JavaScript errors. Private-page restrictions and application redirects were
verified. Apache configuration and all 19 application/infrastructure containers
remained unchanged; the application still serves
`v1.0.0-meeting.20261020-completed-chat.1`. Browser evidence is in
`frontend/test-results/landing/copy-compat-published-qa.json`.

## Previous published release

- Release: `landing-20261020T130434Z-893566bb90f0`.
- Source base: `893566bb90f07bdcec4542eec5e00116c423e852`, including the current
  uncommitted landing and documentation changes. The release does not claim to
  be a clean Git commit.
- Local archive:
  `release-artifacts/landing/landing-20261020T130434Z-893566bb90f0.tar.gz`.
- Server release:
  `/var/www/meet-space.janickiy.com/releases/landing-20261020T130434Z-893566bb90f0`.
- Web root: `/var/www/meet-space.janickiy.com/current`, a symlink to the release's
  `site` directory.
- Apache vhost: `/etc/apache2/sites-available/meet-space.janickiy.com.conf`.
- Source vhost templates: `dockers/landing/apache-http.conf` and
  `dockers/landing/apache-https.conf`.

The archive contains public files, vhost templates, a release manifest and
SHA-256 checksums. The neutral private SPA fallback `app.html` is excluded.
Future static updates can use a new immutable release directory and switch
the `current` symlink after verification. Existing releases should be retained
for rollback. No application containers or databases were changed.

The previous release `landing-20261020T125717Z-893566bb90f0` remains on the
server for rollback. The current release includes a 16:9 desktop conference preview
(1010 × 569 CSS pixels, including its border). Browser checks at 1920 × 1080
and 1366 × 768 confirm that the complete window fits below the navigation.
The four 3:2 participant portraits remain uncropped, chat content fits without
clipping, and the separate mobile layout remains in place.

## HTTPS and routing

Let's Encrypt certificate:
`/etc/letsencrypt/live/meet-space.janickiy.com/`.
The certificate obtained on publication expires on 18 January 2027. The existing
enabled `certbot.timer` handles renewal through the shared webroot
`/var/www/letsencrypt`; its deployment hook reloads Apache gracefully.
Private key contents and authentication credentials are not stored here.

HTTP redirects to the same HTTPS hostname, with an exception for ACME
challenges. The vhost serves all prerendered public documents directly and
returns 404 for unknown pages or API routes. Existing account-route bookmarks
redirect to the application host. HTML is revalidated; hashed assets are cached.

## Verification

- Production build and all 17 public-document checks passed.
- All **127 live public files** matched the release SHA-256 checksums, including
  all 17 HTML documents, scripts, styles, images and fonts.
- TLS validation, HTTP-to-HTTPS, five application redirects and 404 boundaries
  passed from an external client.
- Canonical URLs and social metadata use the landing domain; documentation is
  Russian and retains the corrected two-column workspace table.
- Browser checks passed for the home page, Fedora RPM selection and Russian
  documentation search. No browser warning or error was recorded.
- Apache syntax checks passed before each graceful reload. The existing
  application vhost checksum and public version response remained unchanged.

Local evidence is in `frontend/test-results/landing/live-deployment.json`,
`production-domain-build.log` and `published-landing.png`. Widescreen update
evidence is in `widescreen-build.log`, `widescreen-preview-qa.json` and
`published-widescreen-preview.png` in the same directory.


The interface documentation update adds nine browser screenshots from the live
web application, factual Russian captions, full-size links and responsive
uncropped display. All nine new public JPEG files are covered by release
checksums. Browser validation confirmed reserved image proportions before lazy
loading, no horizontal overflow at 390 px, and search by screenshot alternative
text. Evidence is in `interface-screenshots-build.log`,
`interface-screenshots-desktop.jpg` and `interface-screenshots-mobile.jpg` under
`frontend/test-results/landing/`.


The installation article download step is now the single requested command
`wget https://meeting.janickiy.com/downloads/install.sh`. Running the installer
remains a separate step. The installer and checksum are also available as direct
links; both endpoints returned HTTPS 200 during validation. Build evidence is in
`frontend/test-results/landing/installer-download-build.log`.


The desktop hero conference chat column is now `clamp(230px,25.8cqw,260px)`
instead of `clamp(250px,28.7cqw,290px)`, about 10% narrower. At 1366×768, the
frame remains 1010×569px, both chat messages fit without clipping and all four
portraits retain their 3:2 proportions. Evidence is in `narrower-chat-build.log`,
`narrower-chat-qa.json` and `published-narrower-chat.jpg` under
`frontend/test-results/landing/`.


Logo links in the standalone landing header, mobile menu, footer and documentation
now navigate to the root homepage. Clicking the shared logo while already on the
homepage also returns to the top. Browser checks confirmed navigation from the
download page and installation documentation, plus a same-page return from
scrollY 720 to 0. The application build retains its existing `/welcome` destination.
Build evidence is in `frontend/test-results/landing/logo-home-build.log`.


Public typography now uses ASCII hyphens in place of em dashes: 58 replacements
across 19 landing/documentation source and SVG-title files. All 17 prerendered
public HTML documents, including metadata, were checked for absence of em dashes.
Private application fallback copy and authentic browser screenshots were outside
this text-only change. Build evidence is in
`frontend/test-results/landing/short-dashes-build.log`.


The requested wording «веб платформа» is now used without a hyphen in the
homepage title, footer and documentation. Three source occurrences were changed,
including both ASCII and nonbreaking hyphen variants. The 17 public HTML pages
were checked for absence of the former spelling. Build evidence is in
`frontend/test-results/landing/web-platform-build.log`.


The shared product description now reads: «Видеоконференции, звонки, личные и
групповые чаты, планирование встреч и хранение записей встреч.» This updates the
homepage description and shared marketing footer. Build evidence is in
`frontend/test-results/landing/calls-description-build.log`.


The seven feature cards now place their decorative icons and h3 titles in a
shared horizontally aligned heading row. Desktop grid spans are retained. Below
600 px the feature grid uses one column with compact 36 px icons, and the first
card title scales to fit 320 px screens. Browser checks at 1280 px, 390 px and 320 px
confirmed aligned icon/title centers and no heading or page overflow. Evidence
is in `feature-heading-build.log`, `feature-heading-qa.json` and
`published-feature-heading.jpg` under `frontend/test-results/landing/`.


The video detail description now omits the participant-management phrase as
requested, and the first FAQ answer includes calls alongside video conferences.
The exact Russian text was verified in the live browser. Evidence is in
`video-description-build.log`, `published-video-description.jpg` and
`published-faq-calls.jpg` under `frontend/test-results/landing/`.
