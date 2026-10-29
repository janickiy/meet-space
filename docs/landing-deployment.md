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

## Latest acceptance checks introduction update

Current release: `landing-20261028T033753Z-acceptance-copy`, published on
28 October 2026 (Europe/Moscow) from
`landing-20261028T033347Z-installer-copy`.
Replaced the introduction to Before you begin > Check after installation with
the two requested paragraphs about testing the web interface, authentication,
audio and video before inviting users. Only “Важно:” is bold in the second
paragraph about Docker, WebRTC and network configuration. The four checks,
version verification and final note are unchanged.
Source: `frontend/src/landing/docs-install.ts`.

Scoped active article data and matching SSR are published under
`acceptance-copy-20261028t033753z`. Source SSR matches the parent baseline;
replacing the changed introduction yields identical before/after HTML.
Other paragraphs, articles, metadata, reading time and CSS are unchanged.
Prettier, whitespace, Node24 syntax, exact live HTML/client comparisons and
all 32,569 prepared-file checksums passed. Parent checksums passed before
preparation. Browser checks confirmed two exact paragraphs, bold “Важно:”,
all four steps, no overflow and no runtime errors.
Free space after preparation: 260,034,560 bytes. No full application rebuild.
Unchanged files are hard-linked; changes use private files and atomic replacement.

Evidence: `frontend/test-results/landing/acceptance-copy-browser-qa.json`,
`acceptance-copy-source-verification.json`, `acceptance-copy-published-verification.json`,
`published-acceptance-copy.png` and `verify-acceptance-copy.mjs`.
Snapshots and scripts: `/private/tmp/meetspace-acceptance-copy-20261028`.

## Previous prepared-image installer explanation update

Preceding release: `landing-20261028T033347Z-installer-copy`, published on
28 October 2026 (Europe/Moscow) from
`landing-20261028T032800Z-before-begin-copy`.
Replaced the first paragraph under Before you begin > New installation from
prepared images with the exact requested explanation of the interactive
installer. `install.sh` is rendered as bold inline code. Added handling for an
entire code fragment inside bold text; no existing active article used that
combination, so other rendered fragments are unchanged. Sources:
`frontend/src/landing/docs-install.ts` and `frontend/src/pages/DocumentationPage.tsx`.

Scoped active article data, rich-text expression and matching SSR are published
under `installer-copy-20261028t033347z`. Source SSR matches the parent baseline;
the HTML diff contains only the paragraph and automatic reading time (4 to 5
minutes). Other article data, paragraphs, SEO metadata and CSS are unchanged.
Prettier, whitespace, Node24 syntax, exact public HTML/client comparisons and
all 32,191 prepared-file checksums passed. Parent checksums passed before
preparation. Browser verification confirmed exact wording, nested bold code
with weight 700 and size 17px, no overflow and no runtime errors.
Free space after preparation: 276,955,136 bytes. No full application rebuild.
Unchanged files are hard-linked; changes use private files and atomic replacement.

Evidence: `frontend/test-results/landing/installer-copy-browser-qa.json`,
`installer-copy-source-verification.json`, `installer-copy-published-verification.json`,
`published-installer-copy.png` and `verify-installer-copy.mjs`.
Snapshots and scripts: `/private/tmp/meetspace-installer-copy-20261028`.

## Previous Before you begin wording update

Preceding release: `landing-20261028T032800Z-before-begin-copy`, published on
28 October 2026 (Europe/Moscow) from
`landing-20261028T032431Z-support-heading`.
Replaced the first paragraph in Before you begin > Choose your installation
path with the exact requested wording: “Перед началом определите,
**устанавливаете ли вы MeetSpace впервые или обновление и настройка уже
установленной платформы**. От этого зависит порядок действий.”
Source: `frontend/src/landing/docs-install.ts`. Bold emphasis is preserved.

Published scoped active article data and matching SSR under
`before-begin-copy-20261028t032800z`. Source SSR matches the parent baseline;
replacing the one changed paragraph yields identical before/after HTML.
Other paragraphs, article data, metadata and CSS are unchanged. Prettier,
whitespace, Node24 syntax, exact public HTML/client checks and all 31,813
prepared-file checksums passed. Parent checksums passed before preparation.
Browser checks confirmed the exact wording, bold span, no overflow and no
runtime errors. Free space after preparation: 293,883,904 bytes.
No full application rebuild. Unchanged files are hard-linked; changes use
private files and atomic replacement.

Evidence: `frontend/test-results/landing/before-begin-copy-browser-qa.json`,
`before-begin-copy-source-verification.json`, `before-begin-copy-published-verification.json`,
`published-before-begin-copy.png` and `verify-before-begin-copy.mjs`.
Snapshots and scripts: `/private/tmp/meetspace-before-begin-copy-20261028`.

## Previous support heading update

Preceding release: `landing-20261028T032431Z-support-heading`, published on
28 October 2026 (Europe/Moscow) from
`landing-20261028T031517Z-direct-call-screenshot`.
Made the Support section heading visible as “Коммерческая разработка” directly
before the existing individual development and adaptation paragraph. The same
`where-to-get-help` anchor is preserved, and the table of contents uses the new
heading. Both commercial paragraphs, the introduction and contacts are unchanged.
Source: `frontend/src/landing/docs-product.ts`.

Scoped active Support article data and matching SSR are published under
`support-heading-20261028t032431z`. Source SSR matches the parent, and the
before/after HTML differs only in the visible heading and its contents links.
Other active articles and CSS are unchanged. Prettier, whitespace, Node24 syntax,
exact live HTML/client comparisons and all 31,435 prepared-file checksums passed.
The parent release checksums passed before preparation. Browser verification
confirmed the visible H2 immediately before the requested paragraph, preserved
text, matching table of contents, no overflow and no runtime errors.
Free space after preparation: 310,476,800 bytes. No full application rebuild.
Unchanged files are hard-linked; changes use private files and atomic replacement.

Evidence: `frontend/test-results/landing/support-heading-browser-qa.json`,
`support-heading-source-verification.json`, `support-heading-published-verification.json`,
`published-support-heading.png` and `verify-support-heading.mjs`.
Snapshots and scripts: `/private/tmp/meetspace-support-heading-20261028`.

## Previous direct call screenshot update

Preceding release: `landing-20261028T031517Z-direct-call-screenshot`, published on
28 October 2026 (Europe/Moscow) from
`landing-20261028T025735Z-appearance-screenshot-order`.
Replaced the group chat screenshot below Direct calls with the second supplied
attachment, copied byte-for-byte to `media/docs/direct-call.png` (2480×1700).
The image now belongs to the Direct calls subsection. Alt text and caption
describe the outgoing personal call, waiting for an answer and call controls.
The full-size link points to the original PNG. The older group chat asset is
retained for cached pages and rollback. All nine article images remain present.

Published scoped Interface data and matching SSR under
`direct-call-screenshot-20261028t031517z`. Source SSR matches the parent;
removing the replaced figure yields identical before/after HTML. Other active
articles and CSS are unchanged. Prettier, whitespace, Node24 syntax, exact live
HTML/client comparisons, image bytes/MIME/SHA-256 and all 31,057 prepared-file
checksums passed. Both active and parent release checksums passed after
activation. Browser verification confirmed the correct subsection, one new image,
no old group chat image, original image dimensions, full-size link, no overflow
and no runtime errors. Free space after preparation: 327,278,592 bytes.
No full application rebuild. Unchanged files are hard-linked; modifications use
private files and atomic replacement.

Evidence: `frontend/test-results/landing/direct-call-screenshot-browser-qa.json`,
`direct-call-screenshot-source-verification.json`,
`direct-call-screenshot-published-verification.json`,
`published-direct-call-screenshot.png` and `verify-direct-call-screenshot.mjs`.
Snapshots and scripts: `/private/tmp/meetspace-direct-call-screenshot-20261028`.

## Previous appearance screenshot placement update

Preceding release: `landing-20261028T025735Z-appearance-screenshot-order`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T024712Z-interface-lead`.
Moved the appearance settings screenshot into Interface > Account settings >
Devices and appearance, immediately after its two paragraphs and before
Saved preferences. The same image, full-size link, alt text, caption and
928×780 dimensions are retained; all nine article screenshots remain present.
Only `docs-interface.ts` data changed. Existing subsection renderer/search
support and CSS were reused.

Scoped active Interface data and matching SSR figure relocation are published
under `appearance-screenshot-order-20261028t025735z`. Source SSR matches the
parent, and removing the one moved figure yields identical before/after HTML.
Prettier, whitespace, Node24 syntax, exact live page/client comparisons and
unchanged CSS checks passed. Browser checks confirmed one image in the correct
subsection, the next subsection heading, preserved caption/dimensions, no
overflow and no runtime errors. Other active articles are unchanged. All
30,678 prepared-file checksums and current-parent checksums passed. No full
application rebuild. Unchanged files are hard-linked; modifications use private
files and atomic replacement.

Before activation, available space was restored without removing rollback
contents: 7,603 byte-identical same-path regular files with one link in
`landing-20261027T112854Z-conference-copy/site` were atomically replaced by
hard links to identical files in `landing-20261027T111750Z-docs-cards/site`.
Device, size, owner/group, mode, SHA-256 and unchanged inode/mtime were checked.
Both complete rollback releases passed checksums before and after deduplication.
Released 336,506,880 allocated bytes; free space at activation was 344,797,184
bytes. The preceding completed remote staging payload was also cleaned; local
snapshots and all release/rollback contents remain. Script: `deduplicate.py`
in the input directory below. Publishers must privately replace linked files.

Evidence: `frontend/test-results/landing/appearance-screenshot-order-browser-qa.json`,
`appearance-screenshot-order-source-verification.json`,
`appearance-screenshot-order-published-verification.json`,
`appearance-screenshot-order-client-verification.json`,
`published-appearance-screenshot-order.png` and `verify-appearance-screenshot-order.mjs`.
Snapshots and scripts: `/private/tmp/meetspace-appearance-screenshot-order-20261028`.

## Previous Interface workspace summary relocation

Preceding release: `landing-20261028T024712Z-interface-lead`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T023821Z-interface-signin-copy`.
Moved “Рабочее пространство объединяет встречи, переписку, календарь, записи
и настройки.” directly below the Interface H1, preserving bold emphasis on
“Рабочее пространство”. The first section paragraph retains only the sign-in
instruction. Added optional rich-text article lead support and first-section
search indexing, while keeping metadata descriptions and other article leads
unchanged. Explicit bold styling now covers header leads.

Scoped active Interface data, renderer/search expressions, matching SSR and
lead-only CSS under `interface-lead-20261028t024712z` are published. Source SSR
matches the parent baseline and proves the HTML diff consists only of moving
the existing sentence. Prettier, whitespace, Node24 syntax, exact public page,
client/CSS comparisons and active-data checks passed. Browser checks confirm
one occurrence directly after H1, bold weight 700, the original sign-in text,
working search result, unchanged Architecture description, no overflow and
no runtime errors. All 30,300 prepared-file checksums and parent checksums
passed. Changed files were privately replaced over a hard-linked copy. No
full application rebuild. Free space after preparation: 22,458,368 bytes.
Only the preceding completed remote staging payload and archive were cleaned;
local snapshots and release/rollback directories remain intact.

Evidence: `frontend/test-results/landing/interface-lead-browser-qa.json`,
`interface-lead-source-verification.json`, `interface-lead-published-verification.json`,
`interface-lead-client-verification.json`, `published-interface-lead.png` and
`verify-interface-lead.mjs`. Snapshots and script:
`/private/tmp/meetspace-interface-lead-20261028`.

## Previous Interface sign-in wording update

Preceding release: `landing-20261028T023821Z-interface-signin-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T011405Z-footer-polish`.
Replaced only the opening sentence in Interface > Your workspace with:
“Откройте MeetSpace в браузере по адресу вашего сервера и авторизуйтесь, используя
данные своей учётной записи.” Source: `frontend/src/landing/docs-interface.ts`.
The active article and matching SSR sentence are updated under
`interface-signin-copy-20261028t023821z`; all other text, metadata and styles
are unchanged after namespace normalization. Source SSR, Prettier, whitespace,
Node24 syntax, exact public HTML/client comparisons and browser checks passed.
No page overflow or runtime errors. All 29,922 prepared-file checksums and
parent checksums passed. Free space after preparation: 36,818,944 bytes.
No full application rebuild.

Removed only five completed remote staging payload directories and their
archives from this thread, reclaiming 14,807,040 bytes; local snapshots and
all release/rollback directories were retained. Evidence:
`frontend/test-results/landing/interface-signin-copy-source-verification.json`,
`interface-signin-copy-published-verification.json`, `interface-signin-copy-browser-qa.json`,
`published-interface-signin-copy.png` and `verify-interface-signin-copy.mjs`.
Snapshots and script: `/private/tmp/meetspace-interface-signin-copy-20261028`.

## Previous shared landing footer polish

Preceding release: `landing-20261028T011405Z-footer-polish`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T010810Z-remove-ru-badge`.
Increased shared marketing footer typography on desktop and mobile: description
14px, headings/column links 13px, copyright and back-to-top 12px, platform
credits 11px. Removed only the server-release link, including the unused
RELEASE import in `Landing.tsx`. Existing source font declarations in
`landing.css` were updated; the scoped release applies equivalent footer-only
CSS overrides. The Android/Tux attribution and all 13 remaining footer links
are preserved. Documentation and other footer components are unchanged.

The shared client link and matching SSR anchor were removed on root, welcome,
download and self-hosted pages under `footer-polish-20261028t011405z`.
Source footer SSR matches the exact scoped removal. Prettier, diff whitespace,
Node24 syntax, exact live download HTML/client/CSS comparisons and release-link
absence on all four pages passed. Browser checks at desktop and 390px mobile
width confirmed all five intended font sizes, 13 retained links, no overflow,
no release label and no runtime errors. All 29,544 prepared-file checksums and
parent checksums passed. Changed files were privately replaced over a hard-linked
copy. No full application rebuild. Free space after preparation: 40,427,520 bytes.
Evidence: `frontend/test-results/landing/footer-polish-source-verification.json`,
`footer-polish-published-verification.json`, `footer-polish-browser-qa.json`,
`published-footer-polish.png`, `published-footer-polish-mobile.png` and
`verify-footer-polish.mjs`. Snapshots and script:
`/private/tmp/meetspace-footer-polish-20261028`.

## Previous documentation RU badge removal

Preceding release: `landing-20261028T010810Z-remove-ru-badge`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T010315Z-system-requirements-rewrite`.
Removed the RU badges from the desktop sidebar and mobile documentation menu
in `DocumentationPage.tsx`, the active renderer and all 13 documentation SSR
pages. Trimmed the label's trailing space. Article data, locale metadata,
CSS and other page content remain unchanged. Asset namespace:
`remove-ru-badge-20261028t010810z`. No full application rebuild.

Source SSR matches the scoped HTML removal. Prettier, whitespace checks,
Node24 syntax, all 13 live page checks and exact client/overview comparisons
passed. Browser checks found no badge elements, overflow or runtime errors.
All 29,166 prepared-file checksums and parent checksums passed; files were
privately replaced over the hard-linked copy. Free space after preparation:
64,954,368 bytes. Evidence: `frontend/test-results/landing/remove-ru-badge-browser-qa.json`,
`remove-ru-badge-source-verification.json`, `remove-ru-badge-published-verification.json`,
`published-remove-ru-badge.png`, `published-remove-ru-badge-detail.png` and
`verify-remove-ru-badge.mjs`. Snapshots and script:
`/private/tmp/meetspace-remove-ru-badge-20261028`.

## Previous System requirements plain-language rewrite

Preceding release: `landing-20261028T010315Z-system-requirements-rewrite`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T005748Z-vps-terminology`.
Rewrote all ten System requirements sections in clearer Russian with 22 focused
subheadings, shorter explanations, lists, steps and 95 bold fragments. Explained
server sizing, actual free disk space, the separate package/source-build checks,
Docker architecture and modes, dependencies, container services, DNS, ports and
user-device requirements. Kept the VDS/VPS terminology and short hyphens.
All existing section anchors, source links, exact diagnostic commands, resource
values, versions, network ports and technical limits remain unchanged. The disk
recommendation explanation now sits immediately before its table. Independent
editorial/technical review found no lost facts. Only the System requirements
source object changed in `frontend/src/landing/docs-install.ts`.

The scoped release replaces one active client article, its matching SSR root
and description metadata under `system-requirements-rewrite-20261028t010315z`.
Source Vite SSR matched the published baseline using React renderToString and
preserving existing related-link markup. Other source articles, active client
articles, public pages and CSS are unchanged after namespace normalization.
Prettier, whitespace checks, Node24 bundle syntax, source SSR, exact published
HTML/client/CSS comparisons and active-data checks passed. All 28,788 prepared
public-file checksums and parent checksums passed. Unchanged files are hard-linked;
changed files were privately replaced. No full application rebuild. Free space
after preparation: 80,486,400 bytes.

Browser checks confirmed ten sections, 22 subheadings, 95 rendered bold fragments,
no empty list items, unrendered Markdown or long dashes, correct resources and
port rows, working client navigation and no runtime errors. No page overflow at
desktop or 390px mobile width; mobile body text remains 16px. Evidence:
`frontend/test-results/landing/system-requirements-rewrite-source-verification.json`,
`system-requirements-rewrite-browser-qa.json`,
`system-requirements-rewrite-published-verification.json`,
`system-requirements-rewrite-client-verification.json`,
`published-system-requirements-rewrite.png`,
`published-system-requirements-rewrite-mobile.png` and
`verify-system-requirements-rewrite.mjs`. Draft, snapshots and publication script:
`/private/tmp/meetspace-system-requirements-rewrite-20261028`.

## Previous VDS/VPS server terminology update

Preceding release: `landing-20261028T005748Z-vps-terminology`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T005143Z-docs-polish`.
Replaced 17 source references to Linux servers (including three inverse
“сервер Linux” phrases) with VDS/VPS server wording, preserving grammatical
endings. Collapsed three source “Linux-сервере или VDS/VPS-сервере” phrases
into “VDS/VPS-сервере”. Sources: `docs.ts`, `docs-product.ts`, `docs-install.ts`,
`content.ts` and `SelfHostedPage.tsx`. Linux OS/client requirements, executable
commands, URLs and layout remain unchanged.

The scoped release makes exact terminology substitutions in the active client
entry and 11 public HTML pages, including description metadata. All 13 docs
SSR renderings match the direct substitution output, and the active data
matches source for all 11 articles. Prettier, diff whitespace, Node24 syntax,
all 16 HTTP page comparisons, active client/CSS byte comparisons and browser
navigation checks passed. No old server wording, duplicated alternative or
page overflow was found in the inspected pages; no browser errors occurred.
All 28,410 prepared public-file checksums and parent checksums passed. Changed
files were privately replaced; unchanged files remain hard-linked. No full
application rebuild. Free space after preparation: 96,088,064 bytes.
Evidence: `frontend/test-results/landing/published-vps-terminology.png`,
`vps-terminology-browser-qa.json`, `vps-terminology-ssr-verification.json`,
`vps-terminology-published-verification.json`, `vps-terminology-client-verification.json`
and `verify-vps-terminology.mjs`. Snapshots and publication script:
`/private/tmp/meetspace-vps-terminology-20261028`.

## Previous documentation editorial and layout polish

Preceding release: `landing-20261028T005143Z-docs-polish`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T003719Z-troubleshooting-rewrite`.
Reviewed all 11 documentation articles. Corrected repeated and awkward wording,
standardized meeting terminology and spelling, replaced 38 long dashes with
short hyphens, and added 78 bold keyword/UI-label/limit fragments. Preserved
all existing article/section identifiers, links, screenshots and executable
commands. The Interface guest-session note now agrees with the previously
requested and already published three-hour limit in Features. The hotkey
introduction now precedes its list.

Tables render bold text using the shared rich-text formatter. Documentation
styles give bold terms a consistent weight, separate adjacent subsections,
keep whole words together where possible, enlarge the code toolbar and copy
button, and keep search results within the viewport on medium screens. Search
also indexes screenshots moved into subsections. Existing body font sizes,
responsive tables, horizontally scrolling code and the nine Interface
screenshots are retained.

The scoped release replaces the active documentation data initializer, three
equivalent renderer/search expressions, matching documentation SSR roots and
scoped CSS under `docs-polish-20261028t005143z`. Source Vite SSR matched all
12 current page baselines after equivalent quote-entity normalization and
preservation of existing related-link markup. The legacy `/docs/self-hosted`
SSR copy was stale and is synchronized with the current Installation article.
Unrelated marketing/download HTML is byte-identical after namespace
normalization. No full application rebuild was performed. Changed files were
privately replaced; unchanged files remain hard-linked. All 28,032 prepared
public-file checksums passed, as did parent checksums after activation.
Free space after preparation: 113,152,000 bytes.

Prettier, diff whitespace checks, isolated source SSR, Node24 bundle syntax,
exact comparisons for all 13 published documentation routes, active client
data and CSS passed. Browser checks confirmed rendered table keywords,
caption-based search and client navigation, nine Interface screenshots, no
page overflow at desktop/640px/390px widths, mobile 16px body text and 12px
copy controls. Copying the installer command returns the unchanged wget
command, and no browser runtime errors were observed. Evidence:
`frontend/test-results/landing/docs-polish-browser-qa.json`,
`published-docs-polish.png`, `published-docs-polish-mobile.png`,
`verify-docs-polish.mjs`, `docs-polish-ssr-verification.json`,
`docs-polish-published-verification.json`, `docs-polish-client-verification.json`.
Drafts, snapshots and publication script:
`/private/tmp/meetspace-docs-polish-20261028`.

## Previous Troubleshooting plain-language rewrite

Preceding release: `landing-20261028T003719Z-troubleshooting-rewrite`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T003003Z-remove-troubleshooting-live`.
Rewrote all nine Troubleshooting sections in clearer Russian with 30 focused
subheadings, shorter explanations, lists, sequential checks and bold actions.
Explained rootful Docker, recording finalization, drain, production evidence
and project locks. Preserved every existing section anchor, command block,
link and documented technical limit; the live-worker bullet remains removed.
The description and its metadata copies are updated; reading time is eight
minutes. Only the Troubleshooting source object changed in
`frontend/src/landing/docs-install.ts`; other installation articles are unchanged.

The scoped release replaces the Troubleshooting client article, matching SSR
root and description metadata under `troubleshooting-rewrite-20261028t003719z`.
Source rendering was checked against the published parent using React
renderToString. Existing related-link markup was retained verbatim to avoid
publishing unrelated discovery-attribute or arrow-icon differences between
the local renderer and the parent SSR artifact. The resulting baseline matches
the parent exactly. All 27654 public-file checksums passed before activation;
parent release checksums also passed afterwards. Unchanged files are hard-linked
and modifications privately replaced. Free space after preparation: 130,252,800
bytes. No full application rebuild.

Source formatting/diff, isolated Vite SSR, Node24 bundle syntax, exact live
HTML/client comparisons, article JSON and hydrated browser checks passed.
The page has nine sections, 30 subheadings, 89 bold fragments, no empty list
items, no unrendered bold markers or page overflow, and no browser runtime
errors. Both displayed command blocks match the published parent exactly.
Evidence: `frontend/test-results/landing/troubleshooting-rewrite-source-verification.json`,
`troubleshooting-rewrite-verification.json`, `troubleshooting-rewrite-qa.json`,
`published-troubleshooting-rewrite.jpg`, `published-troubleshooting-recording.jpg`
and `verify-troubleshooting-rewrite.mjs`. Reviewed draft, snapshots and script:
`/private/tmp/meetspace-troubleshooting-rewrite-20261028`.

## Previous Troubleshooting live-worker bullet removal

Preceding release: `landing-20261028T003003Z-remove-troubleshooting-live`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T002645Z-interface-order`.
Removed only `**live-worker:** распознавание речи в реальном времени.` from
Troubleshooting > Optional integrations > Task status and provider access.
The product-worker and provider bullets remain adjacent, without an empty
list item. Source: `frontend/src/landing/docs-install.ts`.

The scoped removal deletes one complete JSON array item and corresponding
SSR list item under `remove-troubleshooting-live-20261028t003003z`.
All 27276 public-file checksums passed before activation; parent release
checksums also passed afterwards. Unchanged files are hard-linked and changes
privately replaced. Free space after preparation: 142,848,000 bytes. Source
formatting/diff, Node24 bundle syntax, exact live HTML/client comparisons,
client JSON and hydrated browser checks passed. Two non-empty bullets remain,
with no page overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/remove-troubleshooting-live-verification.json`,
`remove-troubleshooting-live-qa.json` and `published-remove-troubleshooting-live.jpg`.
Script: `/private/tmp/meetspace-remove-troubleshooting-live-20261028/publish.py`.

## Previous Interface quick actions and screenshot order update

Preceding release: `landing-20261028T002645Z-interface-order`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T000947Z-linux-vps-copy`.
In Interface > Your workspace the order is now quick actions, the existing
home screenshot, browser navigation, then smartphone/tablet navigation. The
home screenshot belongs to the quick-actions subsection; its content, caption,
full-size link and dimensions are preserved. Other article content is unchanged.
Sources: `frontend/src/landing/docs-interface.ts`, screenshot types in
`frontend/src/landing/docs.ts` and the shared screenshot component in
`frontend/src/pages/DocumentationPage.tsx`. No CSS changes.

The scoped release updates only the Interface article data, optional subsection
screenshot rendering in the active entry, and matching Interface SSR markup.
All 26898 public-file checksums passed before activation; parent release
checksums also passed afterwards. Namespace: `interface-order-20261028t002645z`.
Unchanged files are hard-linked and changes privately replaced. Free space
after preparation: 155,160,576 bytes. Source formatting/diff, isolated Vite SSR,
Node24 bundle syntax, exact live HTML/client comparisons, hydrated desktop
(1280px) and mobile (390px) checks passed. All nine screenshots remain; the home
screenshot appears once, before browser navigation, without page overflow.
Client navigation through Features and back also preserves this order, with
no runtime errors. The temporary viewport override was reset.

No full application rebuild. The full typecheck reports existing issues:
missing `qrcode.react` in `MeetingInvitationLink.tsx` and unused `sourceLink`
in `docs-install.ts`; no errors are reported in the changed layout files.
The scoped deployment retains the published parent article's unrelated guest
duration wording rather than publishing a pending local copy change.
Evidence: `frontend/test-results/landing/interface-order-verification.json`,
`interface-order-qa.json`, `published-interface-order.jpg` and
`verify-interface-order.mjs`. Publication inputs and script:
`/private/tmp/meetspace-interface-order-20261028`.

## Previous Linux server and VDS/VPS wording update

Preceding release: `landing-20261028T000947Z-linux-vps-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T000658Z-folders-organization-copy`.
Replaced `Linux-сервере` with `Linux-сервере или VDS/VPS-сервере` in Features >
Administration and own server and Architecture > Deployment model. Existing
bold formatting and surrounding content are preserved. Source:
`frontend/src/landing/docs-product.ts`. The same exact wording was synchronized
in the legacy `frontend/src/pages/SelfHostedPage.tsx` source; that component is
not used by the current public marketing routes.

The scoped replacement updates two active client strings and the two matching
SSR pages under `linux-vps-copy-20261028t000947z`. All 26520 public-file
checksums passed before activation; parent release checksums also passed
afterwards. Unchanged files are hard-linked and modifications privately
replaced. Free space after preparation: 167,866,368 bytes. Source
formatting/diff, Node24 syntax, exact live HTML/client comparisons and hydrated
browser checks on both pages passed, with no page overflow. No full application
rebuild. Evidence: `frontend/test-results/landing/linux-vps-copy-verification.json`,
`linux-vps-copy-qa.json`, `published-linux-vps-copy.jpg` and
`published-linux-vps-architecture.jpg`.
Script: `/private/tmp/meetspace-linux-vps-20261028/publish.py`.

## Previous personal folders organization paragraph replacement

Preceding release: `landing-20261028T000658Z-folders-organization-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T000539Z-folders-conferences-copy`.
Replaced the second paragraph in Features > Personal folders with the supplied
explanation of adding one item to multiple folders, retaining related content
when a folder or association is removed, and preserving access rights. The
first paragraph and other article content are unchanged. Source:
`frontend/src/landing/docs-product.ts`.

The scoped replacement updates active client data and SSR HTML under
`folders-organization-copy-20261028t000658z`. All 26142 public-file checksums
passed before activation; parent release checksums also passed afterwards.
Unchanged files are hard-linked and modifications privately replaced. Free
space after preparation: 179,818,496 bytes. Source formatting/diff, Node24
syntax, exact live HTML/client comparisons and hydrated browser checks passed,
with no page overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/folders-organization-copy-verification.json`,
`folders-organization-copy-qa.json` and `published-folders-organization-copy.jpg`.
Script: `/private/tmp/meetspace-folders-organization-20261028/publish.py`.

## Previous personal folders conferences wording update

Preceding release: `landing-20261028T000539Z-folders-conferences-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T000406Z-recording-modes-copy`.
The Features > Personal folders paragraph now starts with
`Организуйте конференции, личные и групповые чаты в папки,`.
The source and published parent used `совещания` in this same phrase; replaced
that single noun with the requested `конференции`. All surrounding content
is unchanged. Source: `frontend/src/landing/docs-product.ts`.

The scoped replacement updates active client data and SSR HTML under
`folders-conferences-copy-20261028t000539z`. All 25764 public-file checksums
passed before activation; parent release checksums also passed afterwards.
Unchanged files are hard-linked and modifications privately replaced. Free
space after preparation: 191,516,672 bytes. Source formatting/diff, Node24
syntax, exact live HTML/client comparisons and hydrated browser checks passed,
with no page overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/folders-conferences-copy-verification.json`,
`folders-conferences-copy-qa.json` and `published-folders-conferences-copy.jpg`.
Script: `/private/tmp/meetspace-folders-conferences-20261028/publish.py`.

## Previous recording modes sentence removal

Preceding release: `landing-20261028T000406Z-recording-modes-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T000251Z-direct-call-user-copy`.
Removed the sentence listing recording modes from Features > Recording,
playback and download. The requested two sentences about server processing
and a single active or processing recording remain verbatim. Other article
content is unchanged. Source: `frontend/src/landing/docs-product.ts`.

The scoped replacement updates active client data and SSR HTML under
`recording-modes-copy-20261028t000406z`. All 25386 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modifications privately replaced. Free space after
preparation: 203,063,296 bytes. Source formatting/diff, Node24 syntax, exact
live HTML/client comparisons and hydrated browser checks passed, with no page
overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/recording-modes-copy-verification.json`,
`recording-modes-copy-qa.json` and `published-recording-modes-copy.jpg`.
Script: `/private/tmp/meetspace-recording-modes-20261028/publish.py`.

## Previous direct call registered user wording update

Preceding release: `landing-20261028T000251Z-direct-call-user-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261028T000137Z-group-admin-copy`.
Replaced `Позвоните коллеге из личного диалогового окна` with
`Позвоните пользователю из личного диалогового окна` in Features > Direct
audio and video calls, preserving the profile link wording and the rest of
the paragraph. Source: `frontend/src/landing/docs-product.ts`.

The scoped replacement updates active client data and SSR HTML under
`direct-call-user-copy-20261028t000251z`. All 25008 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modifications privately replaced. Free space after
preparation: 214,458,368 bytes. Source formatting/diff, Node24 syntax, exact
live HTML/client comparisons and hydrated browser checks passed, with no page
overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/direct-call-user-copy-verification.json`,
`direct-call-user-copy-qa.json` and `published-direct-call-user-copy.jpg`.
Script: `/private/tmp/meetspace-direct-call-user-20261028/publish.py`.

## Previous group administrator permissions wording update

Preceding release: `landing-20261028T000137Z-group-admin-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T235950Z-registered-user-copy`.
Replaced only the final group-administrator sentence in Features > Personal
and group chats with the requested explanation of changing group settings and
adding or removing users with ordinary access rights. Other paragraph content
is unchanged. Source: `frontend/src/landing/docs-product.ts`.

The scoped replacement updates active client data and SSR HTML under
`group-admin-copy-20261028t000137z`. All 24630 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modifications privately replaced. Free space after
preparation: 225,746,944 bytes. Source formatting/diff, Node24 syntax, exact
live HTML/client comparisons and hydrated browser checks passed, with no page
overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/group-admin-copy-verification.json`,
`group-admin-copy-qa.json` and `published-group-admin-copy.jpg`.
Script: `/private/tmp/meetspace-group-admin-20261028/publish.py`.

## Previous personal chat registered user wording update

Preceding release: `landing-20261027T235950Z-registered-user-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T235646Z-guest-session-copy`.
Replaced `Найдите зарегистрированного коллегу по имени` with
`Найдите зарегистрированного пользователя по имени` in Features > Personal
and group chats. The remaining paragraph and article content are unchanged.
Source: `frontend/src/landing/docs-product.ts`.

The scoped replacement updates active client data and SSR HTML under
`registered-user-copy-20261027t235950z`. All 24252 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modifications privately replaced. Free space after
preparation: 236,847,104 bytes. Source formatting/diff, Node24 syntax, exact
live HTML/client comparisons and hydrated browser checks passed, with no page
overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/registered-user-copy-verification.json`,
`registered-user-copy-qa.json` and `published-registered-user-copy.jpg`.
Script: `/private/tmp/meetspace-registered-user-20261028/publish.py`.

## Previous guest session documentation duration update

Preceding release: `landing-20261027T235646Z-guest-session-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T235424Z-remove-reactions`.
Changed the guest-session duration in Features > Invitations, waiting room and
moderation from `**один час**` to `**3 часа**`. The surrounding paragraph and
other article content are unchanged. Source:
`frontend/src/landing/docs-product.ts`. This updates documentation only.

The scoped replacement updates client data and SSR HTML under
`guest-session-copy-20261027t235646z`. All 23874 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modifications privately replaced. Free space after
preparation: 247,418,880 bytes. Source formatting/diff, Node24 syntax, exact
live HTML/client comparisons and hydrated browser checks passed. The duration
is bold (weight 700), with no page overflow. No full application rebuild.
Evidence: `frontend/test-results/landing/guest-session-copy-verification.json`,
`guest-session-copy-qa.json` and `published-guest-session-copy.jpg`.
Script: `/private/tmp/meetspace-guest-session-20261028/publish.py`.

## Previous Features Reactions bullet removal

Preceding release: `landing-20261027T235424Z-remove-reactions`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T235109Z-guide-description`.
Removed the requested Reactions bullet from Features > Conference capabilities.
The five remaining capability bullets are preserved, with Meeting chat and
Participant management now adjacent. Source:
`frontend/src/landing/docs-product.ts`. Other article content is unchanged.

The scoped removal deletes the complete JSON array item and rendered SSR list
item under `remove-reactions-20261027t235424z`. All 23496 public-file checksums
passed before activation; parent release checksums also passed afterwards.
Unchanged files are hard-linked and modifications privately replaced. Free
space after preparation: 258,297,856 bytes. Source formatting/diff, Node24
syntax, exact live HTML/client comparisons, client JSON and hydrated browser
checks passed, confirming five non-empty bullets and no page overflow.
No full application rebuild. Evidence:
`frontend/test-results/landing/remove-reactions-verification.json`,
`remove-reactions-qa.json` and `published-remove-reactions.jpg`.
Script: `/private/tmp/meetspace-remove-reactions-20261028/publish.py`.

## Previous documentation guide description replacement

Preceding release: `landing-20261027T235109Z-guide-description`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T234727Z-before-begin-copy`.
The documentation overview description now reads
`Руководство по использованию MeetSpace, развёртыванию, настройке и администрированию собственного сервера.`
The visible introduction and its metadata copies are updated on both `/docs`
and `/docs/overview`. Other article content is unchanged. Source:
`frontend/src/landing/docs.ts`.

The scoped replacement updates active client data and SSR HTML under
`guide-description-20261027t235109z`. Preparation initially stopped at a route
scope assertion because the overview has two SSR routes; the same prepared
release was completed after verifying both routes and every changed HTML page
against its parent. All 23118 public-file checksums passed before activation;
parent release checksums also passed afterwards. Unchanged files are hard-linked
and modifications privately replaced. Free space after preparation: 269,078,528
bytes. Source formatting/diff, Node24 syntax, exact live HTML/client comparison,
overview alias and hydrated browser checks passed. Reading time remains two
minutes; no page overflow or full application rebuild. Evidence:
`frontend/test-results/landing/guide-description-verification.json`,
`guide-description-qa.json` and `published-guide-description.jpg`.
Scripts: `/private/tmp/meetspace-guide-description-20261028/publish.py` and
`resume.py` in the same directory.

## Previous Before you begin sentence update

Preceding release: `landing-20261027T234727Z-before-begin-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T234121Z-record-worker-bold`.
Removed only «секреты, » from the requested sentence in the repository-tools
bullet: `HTTPS, сеть, хранилища и проверки в этом случае настраиваются самостоятельно.`
Other sentences, list formatting and article content remain unchanged. Source:
`frontend/src/landing/docs-install.ts`.

The scoped replacement updates SSR HTML and active client data under
`before-begin-copy-20261027t234727z`. All 22740 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and changes privately replaced. Source formatting/diff,
Node24 syntax, exact live HTML/client comparison and hydrated browser checks
passed with no page overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/before-begin-copy-verification.json`,
`before-begin-copy-qa.json` and `published-before-begin-copy.jpg`.
Inputs/scripts: `/private/tmp/meetspace-before-begin-copy-20261028`.

Before preparation, only 19,714,048 bytes were free. To restore publication
headroom without removing rollback contents, 6090 byte-identical same-path
regular files with one link in `landing-20261027T091404Z-calendar-copy/site`
were atomically replaced with hardlinks to matching files in its parent,
`landing-20261027T090236Z-personal-copy/site`. Device, size, owner/group,
permissions, SHA-256 and unchanged inode/mtime were checked. Both complete
rollback releases passed checksums before and after deduplication. Released
270,295,040 allocated bytes; free space then reached 290,009,088 bytes and was
279,674,880 bytes after preparing the new release. All release files and bytes
are retained. Future publishers must privately replace linked files before
editing them. Script: `deduplicate.py` in the input directory above.

## Previous complete Record Worker name emphasis

Preceding release: `landing-20261027T234121Z-record-worker-bold`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T233922Z-server-checklist-copy`.
Changed `Record **Worker**` to `**Record Worker**` in Architecture application
services so the full name is bold. The following description and other content
remain unchanged. Source: `frontend/src/landing/docs-product.ts`.

The scoped replacement updates client data and rendered SSR markup under
`record-worker-bold-20261027t234121z`. All 22362 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modified files privately replaced. Free space after
preparation: 19,828,736 bytes. Source formatting/diff, Node24 syntax, exact live
HTML/client comparisons and hydrated browser checks passed. Computed name
weight is 700 and description weight is 400; no page overflow. No full
application rebuild. Evidence:
`frontend/test-results/landing/record-worker-bold-verification.json`,
`record-worker-bold-qa.json` and `published-record-worker-bold.jpg`.
Script: `/private/tmp/meetspace-record-worker-bold-20261028/publish.py`.

## Previous introductory server checklist wording update

Preceding release: `landing-20261027T233922Z-server-checklist-copy`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T233515Z-media-worker-label`.
Shortened the Before you begin description to the requested phrase
`Linux-сервер, доступ по SSH и домен`. Surrounding sentences and section content
remain unchanged. Source: `frontend/src/landing/docs-install.ts`.

The scoped replacement updates active client data, visible SSR description and
its three metadata copies under `server-checklist-copy-20261027t233922z`.
All 21984 public-file checksums passed before activation; parent release
checksums also passed afterwards. Unchanged files are hard-linked and modified
files privately replaced. Free space after preparation: 30,081,024 bytes.
Source formatting/diff, Node24 syntax, exact live HTML/client comparisons and
hydrated browser checks passed. Reading time remains four minutes; no page
overflow or full application rebuild. Evidence:
`frontend/test-results/landing/server-checklist-copy-verification.json`,
`server-checklist-copy-qa.json` and `published-server-checklist-copy.jpg`.
Script: `/private/tmp/meetspace-server-checklist-copy-20261028/publish.py`.

## Previous Media Worker diagram label update

Preceding release: `landing-20261027T233515Z-media-worker-label`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T233340Z-remove-live-worker`.
Changed only the WebRTC media-server label to
`→ Media Worker - Медиасервер (SFU / Pion WebRTC)`. Other diagram lines,
indentation and article content remain unchanged. Source:
`frontend/src/landing/docs-product.ts`.

The scoped replacement updates SSR HTML and active client data under
`media-worker-label-20261027t233515z`. All 21606 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modified files privately replaced. Free space after
preparation: 40,218,624 bytes. Source formatting/diff, Node24 syntax, exact live
HTML/client comparisons and hydrated browser checks passed, confirming the
requested label and no page overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/media-worker-label-verification.json`,
`media-worker-label-qa.json` and `published-media-worker-label.jpg`.
Script: `/private/tmp/meetspace-media-worker-label-20261028/publish.py`.

## Previous Live Worker bullet removal

Preceding release: `landing-20261027T233340Z-remove-live-worker`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T233157Z-record-worker-name`.
Removed the complete Live Worker service bullet from Architecture > Application
services. The six remaining services and their descriptions are preserved,
including Record Worker, Product Worker and Migrate. Source:
`frontend/src/landing/docs-product.ts`.

The scoped removal deletes the complete client array item and rendered SSR
list item under `remove-live-worker-20261027t233340z`. All 21228 public-file
checksums passed before activation; parent release checksums also passed
afterwards. Unchanged files are hard-linked and modified files privately
replaced. Free space after preparation: 50,147,328 bytes. Source formatting/diff,
Node24 syntax, exact live HTML/client comparisons and hydrated browser checks
passed, confirming six non-empty bullets and no page overflow. No full
application rebuild. Evidence:
`frontend/test-results/landing/remove-live-worker-verification.json`,
`remove-live-worker-qa.json` and `published-remove-live-worker.jpg`.
Script: `/private/tmp/meetspace-remove-live-worker-20261028/publish.py`.

## Previous recording service name update

Preceding release: `landing-20261027T233157Z-record-worker-name`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T233009Z-record-worker`.
Changed the Architecture application-service bullet from `**Worker**` to
`Record **Worker**`, exactly preserving the supplied formatting and description.
Source: `frontend/src/landing/docs-product.ts`. Other article content is unchanged.

The scoped replacement updates active client data and rendered SSR HTML under
`record-worker-name-20261027t233157z`. All 20850 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modified files privately replaced. Free space after
preparation: 59,924,480 bytes. Source formatting/diff, Node24 syntax, exact live
HTML/client comparisons and hydrated browser text/formatting checks passed.
No page overflow or full application rebuild. Evidence:
`frontend/test-results/landing/record-worker-name-verification.json`,
`record-worker-name-qa.json` and `published-record-worker-name.jpg`.
Script: `/private/tmp/meetspace-record-worker-name-20261028/publish.py`.

## Previous Architecture recording branch replacement

Preceding release: `landing-20261027T233009Z-record-worker`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T231855Z-architecture-topology`.
Updated the diagram to the exact requested tree: RabbitMQ now leads to
`Record Worker`, then `Сервис записи конференций (FFmpeg)`. Removed both separate
flow lines below the tree, which now ends at the TURN relay description.
Other tree labels, indentation and article content remain unchanged. Source:
`frontend/src/landing/docs-product.ts`.

The scoped diagram replacement updates SSR HTML and active client data under
`record-worker-20261027t233009z`. All 20472 public-file checksums passed before
activation; parent release checksums also passed afterwards. Unchanged files
are hard-linked and modified files privately replaced. Free space after
preparation: 69,566,464 bytes. Source formatting/diff, Node24 syntax, exact live
HTML/client comparisons and hydrated browser checks passed. No page overflow;
existing internal code-block scrolling is preserved. No full application rebuild.
Evidence: `frontend/test-results/landing/record-worker-verification.json`,
`record-worker-qa.json` and `published-record-worker.jpg`.
Script: `/private/tmp/meetspace-architecture-record-worker-20261028/publish.py`.

## Previous Architecture topology tree replacement

Preceding release: `landing-20261027T231855Z-architecture-topology`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T230653Z-product-worker-label`.
Replaced the initial eight-line deployment tree with the requested expanded
MeetSpace topology: reverse proxy, Frontend, API, PostgreSQL, Redis, Product
Worker, RabbitMQ, recording service with FFmpeg, S3 storage with MinIO/SeaweedFS,
and WebRTC SFU/Pion with TURN. Tree indentation and connector rows are preserved.
The two separate flow lines below the tree remain unchanged. Source:
`frontend/src/landing/docs-product.ts`.

The scoped replacement updates SSR HTML and active client data under
`architecture-topology-20261027t231855z`. All 20094 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and changed files privately replaced. Free space after
preparation: 79,515,648 bytes. Source formatting/diff, Node24 syntax, exact live
HTML/client comparison and hydrated browser checks passed. No page overflow;
existing internal code-block scrolling is preserved. No full application rebuild.
Evidence: `frontend/test-results/landing/architecture-topology-verification.json`,
`architecture-topology-qa.json` and `published-architecture-topology.jpg`.
Script: `/private/tmp/meetspace-architecture-topology-20261028/publish.py`.

## Previous Architecture Product Worker label update

Preceding release: `landing-20261027T230653Z-product-worker-label`, published on
28 October 2026 (Europe/Moscow) from `landing-20261027T191848Z-architecture-flow`.
Replaced the background-worker diagram label with the exact requested wording:
`Сервис фоновой обработки (Product Worker) ↔ База данных PostgreSQL → Подключённые внешние сервисы и интеграции`.
Source: `frontend/src/landing/docs-product.ts`. Other content is unchanged.

The scoped replacement updates SSR HTML and active client data under
`product-worker-label-20261027t230653z`. All 19716 public-file checksums passed
before activation; parent release checksums also passed afterwards. Unchanged
files are hard-linked and modified files privately replaced. Free space after
preparation: 89,292,800 bytes. Source formatting/diff, Node24 syntax, exact live
HTML/client comparison and hydrated browser checks passed. The long code line
uses the existing internal horizontal scroll, without page overflow.
No full application rebuild. Evidence:
`frontend/test-results/landing/product-worker-label-verification.json`,
`product-worker-label-qa.json` and `published-product-worker-label.jpg`.
Script: `/private/tmp/meetspace-architecture-product-worker-20261028/publish.py`.

## Previous Architecture diagram line removal

Preceding release: `landing-20261027T191848Z-architecture-flow`, published on
27 October 2026 from `landing-20261027T191701Z-meeting-terms`. Removed only
`SFU → Обработчик речи → Настроенный речевой сервис` and its trailing newline
from the deployment model text diagram. Adjacent recording and background-job
flows remain unchanged. Source: `frontend/src/landing/docs-product.ts`.

The scoped removal updates SSR HTML and active client data under
`architecture-flow-20261027t191848z`. All 19338 public-file checksums passed
before activation, and parent release checksums were verified afterwards.
Unchanged files are hard-linked; modified files were privately replaced.
Free space after preparation: 112,902,144 bytes. Source formatting/diff,
Node24 syntax, exact public HTML/client comparisons and hydrated browser checks
passed; no page overflow. No full application rebuild. Evidence:
`frontend/test-results/landing/architecture-flow-verification.json`,
`architecture-flow-qa.json` and `published-architecture-flow-removal.jpg`.
Script: `/private/tmp/meetspace-architecture-speech-20261027/publish.py`.

## Previous Features meeting terminology update

Preceding release: `landing-20261027T191701Z-meeting-terms`, published on
27 October 2026 from `landing-20261027T191111Z-calendar-copy`. Replaced both
inflections of «собрание» in the Features recording paragraph with «встреча»:
«к активной встрече» and «организатор встречи». Source:
`frontend/src/landing/docs-product.ts`. Other article content is preserved.

The scoped paragraph patch updates SSR HTML and active client data under
`meeting-terms-20261027t191701z`. All 18960 public-file checksums passed before
activation; the parent release checksums were also verified. Unchanged files
are hard-linked and modified files were privately replaced. Free space after
preparation: 122,036,224 bytes. Source formatting/diff, Node24 syntax, public
HTML/client article comparison and hydrated browser checks passed. No remaining
forms of «собрание» occur in the Features article, and no page overflow was found.
No full application rebuild. Evidence:
`frontend/test-results/landing/meeting-terms-verification.json`,
`meeting-terms-qa.json` and `published-meeting-terms.jpg`. Script:
`/private/tmp/meetspace-meeting-terms-20261027/publish.py`.

## Previous Features calendar paragraph update

Preceding release: `landing-20261027T191111Z-calendar-copy`, published on
27 October 2026 from `landing-20261027T190642Z-calendar-title`. Calendar now uses
the exact requested meeting terminology and database-search wording in its
second and third paragraphs. The first paragraph, three-paragraph layout and
current heading are preserved. Source: `frontend/src/landing/docs-product.ts`.

The scoped public article data and SSR paragraph edits use the active entry in
`calendar-copy-20261027t191111z`. Other feature sections and downloads are
preserved. All 18582 public-file checksums passed before activation. Unchanged
files are hard-linked; changes were privately replaced. Free space after
preparation: 131,100,672 bytes. Reading time remains ten minutes. Source
formatting/diff, Node24 syntax, live HTML/client and hydrated browser checks
confirm all three paragraphs and no page overflow. No renderer change or full
application rebuild. Evidence:
`frontend/test-results/landing/calendar-copy-verification.json`,
`calendar-copy-qa.json` and `published-calendar-copy.jpg`. Inputs/scripts:
`/private/tmp/meetspace-calendar-copy-20261027`.

## Previous Features calendar heading update

Preceding release: `landing-20261027T190642Z-calendar-title`, published on
27 October 2026 from `landing-20261027T190307Z-direct-calls-note`. The Features
calendar heading is now `Планирование встреч и управление календарём`, including
both contents labels. The existing `calendar` anchor is preserved. Source:
`frontend/src/landing/docs-product.ts`.

The single-string patch is applied in SSR HTML and active client data under
`calendar-title-20261027t190642z`. Other content and downloads are preserved;
reading time remains ten minutes. All 18204 public-file checksums passed before
activation. Unchanged files are hard-linked; modifications were privately
replaced. Free space after preparation: 140,017,664 bytes. Source formatting/diff,
Node24 syntax, live HTML/client and hydrated browser checks confirm all three
heading/contents labels and no page overflow. No full application rebuild.
Evidence: `frontend/test-results/landing/calendar-title-verification.json`,
`calendar-title-qa.json` and `published-calendar-title.jpg`. Script:
`/private/tmp/meetspace-calendar-title-20261027/publish.py`.

## Previous direct-call note replacement

Preceding release: `landing-20261027T190307Z-direct-calls-note`, published on
27 October 2026 from `landing-20261027T185615Z-file-formats`. The Direct calls note
now uses the exact requested two-user wording, replacing the previous restrictions
and closed-mobile-app sentence. Source: `frontend/src/landing/docs-product.ts`.

The single-string patch is applied in SSR HTML and active client data under
`direct-calls-note-20261027t190307z`. Other feature content, including the expanded
file-format list, and downloads are preserved. Reading time remains ten minutes.
All 17826 public-file checksums passed before activation. Unchanged files are
hard-linked; changes were privately replaced. Free space after preparation:
148,791,296 bytes. Source formatting/diff, Node24 syntax, live HTML/client and
hydrated browser checks passed. No renderer change or full application rebuild.
Evidence: `frontend/test-results/landing/direct-calls-note-verification.json`,
`direct-calls-note-qa.json` and `published-direct-calls-note.jpg`. Script:
`/private/tmp/meetspace-direct-calls-note-20261027/publish.py`.

## Previous supported attachment formats expansion

Preceding release: `landing-20261027T185615Z-file-formats`, published on
27 October 2026 from `landing-20261027T184439Z-feature-copy`. Features > Meeting
chat now has a dedicated Supported file formats subsection with all 75 accepted
extensions grouped into ten categories, replacing the old six-format sentence.
Office, OpenDocument, Apple iWork, EPUB and archive formats are included, with
compound-name examples and an explanation that the server does not unpack or
execute attachments. Existing message/file limits and other article data are
preserved. Source: `frontend/src/landing/docs-product.ts`.

Public application `/version` and `/version.json` both declared
`v0.4.0-betab`, commit `5398cb14e9e92587c6ff549999ed5ddecb6b0517`. Backend type
policy, frontend attachment policy and content-validation files at that commit
were fetched from GitHub and were byte-identical to local source. Client/server
allowlists contain the same 75 extensions. An independent audit confirmed exact
coverage and scope; no file was uploaded and no application message was created.

The scoped public Features data and matching SSR block use active entry namespace
`file-formats-20261027t185615z`. Other feature sections and downloads are preserved.
All 17448 public-file checksums passed before activation. Unchanged files are
hard-linked; modifications were privately replaced. Free space after preparation:
157,417,472 bytes. Complete rollback releases remain present.

Source formatting/diff, Node24 syntax, live HTML/client data and hydrated UI
checks passed. All ten rendered categories match the verified list; existing
limits remain 4000 characters, five files and 10 MiB per file. Search finds XLSB.
At 390-pixel mobile width the list has no horizontal overflow. Reading time is
ten minutes. No renderer change or full application rebuild. Evidence:
`frontend/test-results/landing/file-formats-verification.json`,
`file-formats-qa.json` and `published-file-formats.jpg`. Inputs/scripts:
`/private/tmp/meetspace-file-formats-20261027`.

## Previous Features screen-layout and moderation copy edits

Preceding release: `landing-20261027T184439Z-feature-copy`, published on
27 October 2026 from `landing-20261027T183939Z-features-lead`. The already-updated
capabilities lead is retained. In Features, the paragraph about screen sharing
occupying the main viewing area is removed, and the co-organizer sentence now
uses `права` instead of `полномочия`. Source:
`frontend/src/landing/docs-product.ts`.

The current public Features object was extracted as data and changed only in
those two places; an independent JSON comparison confirmed the scope. This
excludes unrelated pending local-only content edits. Matching SSR HTML and the
active client entry use `feature-copy-20261027t184439z`. Twelve feature sections
remain; reading time remains nine minutes. Other page content and downloads are
preserved. All 17070 public-file checksums passed before activation. Unchanged
files are hard-linked; modifications were privately replaced. Free space after
preparation: 166,096,896 bytes.

Source formatting/diff, Node24 syntax, live HTML/client data and hydrated browser
checks confirm the retained lead, removed paragraph and revised sentence, with
no page overflow. No renderer change or full application rebuild. Evidence:
`frontend/test-results/landing/feature-copy-verification.json`,
`feature-copy-qa.json`, `published-feature-copy.jpg` and
`published-feature-copy-video.jpg`. Draft/scripts:
`/private/tmp/meetspace-feature-copy-20261027`.

## Previous Features lead replacement

Preceding release: `landing-20261027T183939Z-features-lead`, published on
27 October 2026 from `landing-20261027T183559Z-overview-cleanup`. Features now uses
the requested capabilities description as its lead and metadata description.
Pasted indentation was omitted. Source: `frontend/src/landing/docs-product.ts`.

The single-string patch is applied to Features SSR HTML and active client data
under `features-lead-20261027t183939z`. Other article content and downloads are
preserved. All 16692 public-file checksums passed before activation. Unchanged
files are hard-linked; changes were privately replaced. Free space after
preparation: 174,399,488 bytes. Source formatting/diff, Node24 syntax, live
HTML/metadata/client data and hydrated browser checks passed. No full application
rebuild. Evidence: `frontend/test-results/landing/features-lead-verification.json`,
`features-lead-qa.json` and `published-features-lead.jpg`. Script:
`/private/tmp/meetspace-features-lead-20261027/publish.py`.

## Previous Overview wording and beta-note cleanup

Preceding release: `landing-20261027T183559Z-overview-cleanup`, published on
27 October 2026 from `landing-20261027T182907Z-troubleshooting-copy`. Overview >
What this guide covers now uses the requested single-sentence introduction
(with normal spacing after the comma), has no beta-status note and labels the
installation bullet `Установка и обновление`. Source:
`frontend/src/landing/docs.ts`. An independent read-only scope review passed.

Matching changes apply to `/docs` and `/docs/overview` SSR HTML and the active
client data under `overview-cleanup-20261027t183559z`. Other articles, quick-start
card layout and downloads are preserved. All 16314 public-file checksums passed
before activation. Unchanged files are hard-linked; modified files were privately
replaced. Free space after preparation: 182,472,704 bytes. Complete rollback
releases remain present.

Source formatting/diff, Node24 syntax, live HTTP on both Overview routes and
hydrated browser checks confirm all three edits and no horizontal page overflow.
No renderer change or full application rebuild. Evidence:
`frontend/test-results/landing/overview-cleanup-verification.json`,
`overview-cleanup-qa.json` and `published-overview-cleanup.jpg`. Script:
`/private/tmp/meetspace-overview-cleanup-20261027/publish.py`.

## Previous Troubleshooting documentation rewrite

Preceding release: `landing-20261027T182907Z-troubleshooting-copy`, published on
27 October 2026 from `landing-20261027T155958Z-updates-copy`. Troubleshooting now
uses symptom-based Russian guidance across nine preserved anchors, with
25 subheadings, 25 bullets, five ordered checks and 61 bold spans. Nine links and
both diagnostic code blocks are preserved; code values are byte-identical to the
previous article. Source: `frontend/src/landing/docs-install.ts`.

The rewrite clarifies ready-image/source disk recommendations and Buildx scope,
saved-wizard continuation, browser origin/WebSocket checks, recording finalization
and private S3 access, production evidence and stale project lock directories.
Source references now use verified tools revision
`3ea9a4e83e82484c25402963b2e68a0642d986f6`. An independent accuracy/clarity review
passed. No diagnostic commands were executed against the application.

Only article data and matching SSR HTML changed in the active entry under
`troubleshooting-copy-20261027t182907z`. Other public content and downloads are
preserved. All 15936 public-file checksums passed before activation. Unchanged
files are hard-linked; changes were privately replaced. Free space after
preparation: 190,525,440 bytes. Complete rollback releases remain present.

Source import/draft equality, formatting/diff, Node24 syntax, live HTML/client
and hydrated browser checks passed, including both rendered code values and copy
controls. Search finds new subheadings. At 390-pixel mobile width, text and code
stay within the content area without page overflow. Reading time is six minutes.
No renderer change or full application rebuild. Evidence:
`frontend/test-results/landing/troubleshooting-copy-verification.json`,
`troubleshooting-copy-qa.json` and `published-troubleshooting-copy.jpg`.
Draft/scripts: `/private/tmp/meetspace-troubleshooting-copy-20261027`.

## Previous Updates and backups documentation rewrite

Preceding release: `landing-20261027T155958Z-updates-copy`, published on
27 October 2026 from `landing-20261027T155202Z-feature-profile`. Updates and backups
now uses clear Russian across six preserved anchors, with fifteen subheadings,
eleven bullets, fifteen ordered steps and 43 bold spans. Eight links, three code
blocks and one six-row coverage table remain. Executable command lines are
unchanged; only certificate comments were clarified. Source:
`frontend/src/landing/docs-install.ts`.

The article distinguishes installer/release Compose contexts, exact-image update
validation/pull, write-freeze/drain ordering and the meaning of the quiesce flag.
It explains backup coverage, separate secrets/configuration, encryption/storage
and recovery objectives, isolated restore checking and report creation only on
success. A related link now targets the dedicated installer backup procedure.
Public release helpers and relevant runbooks on verified revision
`3ea9a4e83e82484c25402963b2e68a0642d986f6` were independently compared with local
source; the draft passed review. Source references use that revision.

Only article data and matching SSR HTML changed in the active entry under
`updates-copy-20261027t155958z`; other public content and downloads are preserved.
All 15558 public-file checksums passed before activation. Unchanged files are
hard-linked; modifications were privately replaced. Free space after preparation:
211,734,528 bytes. Complete rollback releases remain present.

Source import/draft equality, formatting/diff, Node24 syntax, live HTML/client
and hydrated browser checks passed, including all three command values and copy
controls. Search finds the new subheadings. Code and table stay within the
content area at 390-pixel mobile width without page overflow. Reading time is six
minutes. No renderer change or full application rebuild. Evidence:
`frontend/test-results/landing/updates-copy-verification.json`,
`updates-copy-qa.json` and `published-updates-copy.jpg`. Draft/scripts:
`/private/tmp/meetspace-updates-copy-20261027`.

## Previous Features profile wording update

Preceding release: `landing-20261027T155202Z-feature-profile`, published on
27 October 2026 from `landing-20261027T154613Z-installation-copy`. In Features >
Direct calls, the first sentence now ends `или с профиля пользователя.` instead
of `или с карточки пользователя.`. The rest of the paragraph is preserved.
Source: `frontend/src/landing/docs-product.ts`.

The single-string patch is applied in SSR HTML and the active client entry under
`feature-profile-20261027t155202z`. All 15180 public-file checksums passed before
activation. Unchanged files are hard-linked; changes were privately replaced.
Free space after preparation: 219,860,992 bytes. Formatting/diff, Node24 syntax,
live HTML/client data and hydrated browser checks passed. No full application
rebuild; unrelated pending source edits are excluded. Evidence:
`frontend/test-results/landing/feature-profile-verification.json`,
`feature-profile-qa.json` and `published-feature-profile.jpg`. Script:
`/private/tmp/meetspace-feature-profile-20261027/publish.py`.

## Previous Installation documentation rewrite

Preceding release: `landing-20261027T154613Z-installation-copy`, published on
27 October 2026 from `landing-20261027T153127Z-requirements-next`. Installation now
uses clear Russian across eleven preserved anchors, with 24 subheadings,
22 bullets, nineteen ordered steps and 75 bold spans. Eighteen links, eight code
blocks and both tables remain. The default ready-image workflow is described in
five steps; interruption/resume, source builds and operator-managed releases are
separate sections. Source: `frontend/src/landing/docs-install.ts`.

The public `install.sh` was checked directly: its default release is `latest`
and tool revision is `3ea9a4e83e82484c25402963b2e68a0642d986f6`. The pinned
wizard/from-images/from-source scripts were independently verified as identical
to local source. The article no longer claims a fixed current application tag or
obsolete default tool SHA. Source links and the manual-checkout example use the
verified public tool revision; the release link points to the releases list.
Seven code blocks are byte-identical to the previous article; the eighth changes
only that checkout SHA. Simple wget, SMTP options, actual wizard resume command,
ordinary first-user privileges, storage constraints and production prerequisites
are retained. An independent draft review passed.

Only article data and matching SSR HTML changed in the active entry under
`installation-copy-20261027t154613z`. Other public content and latest downloads
are preserved. All 14802 public-file checksums passed before activation.
Unchanged files are hard-linked; modified files were privately replaced.
Free space after preparation: 227,446,784 bytes. Complete rollback releases remain.

Source import/draft equality, formatting/diff, Node24 syntax, live HTML/client
content and hydrated browser checks passed, including all eight rendered command
values and copy controls. Search finds new subheadings. Code and tables stay
within the content area at 390-pixel mobile width without page overflow.
Reading time is nine minutes. No renderer change or full application rebuild.
Evidence: `frontend/test-results/landing/installation-copy-verification.json`,
`installation-copy-qa.json` and `published-installation-copy.jpg`. Draft/scripts:
`/private/tmp/meetspace-installation-copy-20261027`.

## Previous System requirements editorial revision

Preceding release: `landing-20261027T153127Z-requirements-next`, published on
27 October 2026 from `landing-20261027T152240Z-interface-communication`. System
requirements now separates the basic hardware recommendation from disk guidance
for ready images/source compilation, mandatory Docker prerequisites and local
monitoring. Ten anchors and ten links are preserved, with 22 subheadings,
21 bullets, four ordered diagnostic steps and 52 bold spans. Four tables remain:
three hardware rows, four disk rows, six utility rows and six public-port rows.
Prometheus/Grafana local endpoints are explained separately. Diagnostic code
is byte-identical to the preceding article. Source:
`frontend/src/landing/docs-install.ts`.

The draft passed an independent accuracy/clarity review against the installer.
No new capacity guarantees or unverified OS/browser minimums were introduced.
The scoped article-data and SSR HTML patch uses the active entry in
`requirements-next-20261027t153127z`; other public content and downloads are
preserved. All 14424 public-file checksums passed before activation. Unchanged
files are hard-linked and modified files were privately replaced. Free space
after preparation: 235,315,200 bytes. Complete rollback releases remain present.

Source import/draft equality, formatting/diff, active bundle Node24 syntax,
live HTML/client content and hydrated browser checks passed. Search finds new
subheadings. At 390-pixel width the tables and code stay within the content area
without horizontal page overflow. Reading time is seven minutes. No renderer
change or full application rebuild. Evidence:
`frontend/test-results/landing/requirements-next-verification.json`,
`requirements-next-qa.json` and `published-requirements-next.jpg`. Draft/scripts:
`/private/tmp/meetspace-requirements-next-20261027`.

## Previous Interface communication wording update

Preceding release: `landing-20261027T152240Z-interface-communication`, published
on 27 October 2026. In Interface > Creation and scheduling, the opening phrase is
now `Создайте встречу для коммуникации` instead of
`Создайте встречу для немедленного общения`. The remaining paragraph is unchanged.
Local source: `frontend/src/landing/docs-interface.ts`.

The server had independently advanced to `landing-20261027T151921Z-desktop-021`;
that release was used as parent to retain its latest downloads. The single-string
change is applied in matching SSR HTML and the active client entry under
`interface-communication-20261027t152240z`. All 14046 public files passed
checksums before activation. Unchanged files are hard-linked and changes were
privately replaced. Source formatting/diff, Node24 syntax, live HTML/client and
hydrated browser checks passed. No full application rebuild. Evidence:
`frontend/test-results/landing/interface-communication-verification.json`,
`interface-communication-qa.json` and `published-interface-communication.jpg`.
Script: `/private/tmp/meetspace-interface-communication-20261027/publish.py`.

Before preparation only about 6.9 MB was free. To preserve complete rollback
releases while recovering headroom, 5485 byte-identical same-path public files
with link count one in `landing-20261027T111540Z-overview-copy/site` were atomically
replaced with hardlinks to matching files in its parent
`landing-20261027T104804Z-desktop-0114/site`. Device, size, owner, group, mode,
hash and unchanged inode/mtime were checked before replacement; files already
having multiple links were excluded. Both releases passed checksums before and
afterwards. This released 243,007,488 allocated bytes without removing any files
or rollback contents. Free space after preparing the new release was
242,888,704 bytes. Future publishers must privately replace hard-linked files.

## Previous documentation language/date row removal

Preceding release: `landing-20261027T151824Z-docs-meta`, published on 27 October 2026
from `landing-20261027T151415Z-before-begin`. The shared language/checked-date row
is removed from every documentation page, including overview's language-only row
and installation's 23 October variant. Interface and Source/release pages already
had no row. The shared renderer block is deleted in
`frontend/src/pages/DocumentationPage.tsx`; unused CSS selectors are retained.

Eleven SSR pages were cleaned, with corresponding deletion in the single active
client entry under `docs-meta-20261027t151824z`. Article bodies, lead paragraphs,
reading times, metadata and downloads are preserved. All 13290 public files
passed checksums before activation. Unchanged files are hard-linked; modifications
were privately replaced. Free space after preparation was 14,381,056 bytes.
Future publications need to check disk headroom before preparing another release.
Complete rollback releases remain present.

Source formatting/diff checks, active bundle Node24 syntax, live HTML on all
thirteen documentation routes and hydrated navigation checks on Before you
begin, Installation and Overview passed. No full application rebuild was run.
Evidence: `frontend/test-results/landing/docs-meta-verification.json`,
`docs-meta-qa.json` and `published-docs-meta-removal.jpg`. Script:
`/private/tmp/meetspace-docs-meta-20261027/publish.py`.

## Previous Before you begin documentation rewrite

Preceding release: `landing-20261027T151415Z-before-begin`, published on
27 October 2026 from `landing-20261027T150920Z-interface-mobile`. Before you begin
now uses clear Russian across all five existing anchors, with thirteen
subheadings, fourteen bullets, four numbered checks and 35 bold spans. Seven
related links are preserved; the installer source reference now points to clean
current commit `b91cce2a089a8aa29d89d7ecab50ec168a02d84b`.

The rewrite distinguishes the default ready-image installation from explicit
source compilation and manual deployment. It clarifies a new/nonexistent
installation path, local rootful Docker, subnet, internet access, DNS/TLS,
firewall, backups and key preservation, optional SMTP setup and post-install
acceptance checks. The draft passed an independent review against the current
installer and installation guide. Source: `frontend/src/landing/docs-install.ts`.

Only article data and matching SSR HTML changed in the active entry under
`before-begin-20261027t151415z`; all other public page content and current
downloads are preserved. Unchanged files are hard-linked; changed files were
privately replaced. All 12912 public-file checksums passed before activation.
Free space after preparation: 28,471,296 bytes. Complete rollback releases remain.

Source import/draft equality, formatting/diff checks, active bundle Node24
syntax, live HTML/client content and hydrated browser checks passed. Search finds
the new subheadings. At 390-pixel mobile width there is no horizontal page
overflow. Reading time is four minutes. No renderer change or full application
rebuild was performed. Evidence:
`frontend/test-results/landing/before-begin-verification.json`,
`before-begin-qa.json` and `published-before-begin.jpg`. Draft and scripts:
`/private/tmp/meetspace-before-begin-20261027`. Future publications must start from
the current server artifact and privately replace hard-linked files.

## Previous Interface mobile subheading update

Preceding release: `landing-20261027T150920Z-interface-mobile`, published on
27 October 2026 from `landing-20261027T150745Z-interface-browser`. The Interface
workspace subheading is now `На смартфоне или планшете` instead of `На телефоне`.
Only this heading changed in source, SSR HTML and the active client entry under
`interface-mobile-20261027t150920z`. All other article content is preserved.
All 12534 public-file checksums passed before activation; modified hard-linked
files were privately replaced. Free space after preparation: 35,352,576 bytes.
Source formatting/diff, Node24 syntax, live HTML/client and hydrated browser
checks passed. No full application rebuild. Evidence:
`frontend/test-results/landing/interface-mobile-verification.json`,
`interface-mobile-qa.json` and `published-interface-mobile.jpg`. Script:
`/private/tmp/meetspace-interface-mobile-20261027/publish.py`.

## Previous Interface browser subheading update

The preceding release is `landing-20261027T150745Z-interface-browser`, published
on 27 October 2026. The workspace subheading in Interface is now `В браузере`
instead of `На компьютере`. The desktop/phone explanation elsewhere in the
article is unchanged. Local source: `frontend/src/landing/docs-interface.ts`.

Parent: `landing-20261027T150551Z-interface-url`. The targeted heading was patched
in SSR HTML and the active client entry in `interface-browser-20261027t150745z`.
All 12156 public files passed checksums before activation. Unchanged files are
hard-linked; modified files were privately replaced. Free space after preparation
was 41,852,928 bytes. Source formatting, diff checks, client Node24 syntax, live
HTML/client data and hydrated browser checks passed. Eight sections and nine
screenshots remain. No full application rebuild was performed. Evidence:
`frontend/test-results/landing/interface-browser-verification.json`,
`interface-browser-qa.json` and `published-interface-browser.jpg`. Script:
`/private/tmp/meetspace-interface-browser-20261027/publish.py`.

## Previous Interface installation-URL wording update

The preceding server release is `landing-20261027T150551Z-interface-url`, published
on 27 October 2026. The opening Interface phrase now uses the exact requested
wording, `Откройте MeetSpace по адресу url адресу установки`. The rest of the
paragraph, article, hidden lead/meta and current downloads are preserved.
Local source: `frontend/src/landing/docs-interface.ts`.

Parent: `landing-20261027T150221Z-requirements-copy`. One string was replaced in
Interface SSR HTML and the single active entry under
`interface-url-20261027t150551z`. Unchanged files are hard-linked and modifications
were privately replaced. All 11778 public-file checksums passed before
activation. Free space afterwards was 48,144,384 bytes. The active bundle passed
Node24 syntax checking; live HTML and hydrated browser checks confirm the new
phrase and absence of the old one. Formatting and diff checks passed. No full
application rebuild was performed; unrelated pending source edits were excluded.
Evidence: `frontend/test-results/landing/interface-url-verification.json`,
`interface-url-qa.json` and `published-interface-url.jpg`. Script:
`/private/tmp/meetspace-interface-url-20261027/publish.py`.

## Previous system requirements rewrite and Interface header cleanup

The preceding server release is `landing-20261027T150221Z-requirements-copy`,
published on 27 October 2026. The Interface page no longer shows the lead
paragraph or the language/checked-date row below its title. SEO descriptions,
the article body, eight anchors and nine actual screenshots remain unchanged.
The condition is scoped to Interface in `frontend/src/pages/DocumentationPage.tsx`.

System requirements now uses clear Russian across all ten existing anchors,
with sixteen subheadings, lists, 44 bold spans and four resource/software/port
tables. Ready-image installation and source compilation are explained separately.
The text retains the 4 vCPU / 8 GiB / 60 GiB starting recommendation, source-build
35 GiB recommendation, ready-package copy plus 15 GiB guidance, Docker/Compose
requirements and network ports. Disk-space warnings are correctly described as
advisory; Buildx is only required for source builds. Source-reference links point
to clean current commit `b91cce2a089a8aa29d89d7ecab50ec168a02d84b`.
Local article: `frontend/src/landing/docs-install.ts`.

The parent is `landing-20261027T144939Z-interface-copy`. The targeted article data,
renderer condition and matching SSR HTML were patched in the active entry under
`requirements-copy-20261027t150221z`. Other public page content and separately
published downloads are preserved. Unchanged files are hard-linked; modified
files were privately replaced. All 11400 public-file checksums passed, and both
complete current and parent releases passed checksums after activation. Free
space after preparation was 54,386,688 bytes. Complete rollback releases remain.

An independent copy review checked the draft against the current installer and
installation guide. Source import/equality, formatting, diff whitespace, active
bundle Node24 syntax, live HTML/client data and hydrated browser checks passed.
Interface summary/meta are absent after hydration; other pages retain theirs.
Search finds the new subheadings. At 390-pixel mobile width the page has no
horizontal overflow and tables/code stay within the content area. No full
application rebuild was performed.

Evidence: `frontend/test-results/landing/requirements-copy-verification.json`,
`requirements-copy-qa.json`, `published-requirements-copy.jpg` and
`published-interface-header.jpg`. Inputs and publication scripts:
`/private/tmp/meetspace-requirements-copy-20261027`. Future publications must
start from the current server artifact and privately replace hard-linked files.

## Previous interface documentation rewrite

The preceding server release is `landing-20261027T144939Z-interface-copy`, published
on 27 October 2026. The Interface article now uses clear Russian with 24
subheadings, practical numbered instructions, lists and bold emphasis across
eight sections. All eight anchors, nine actual screenshots with their original
captions/dimensions, related links and the two-column workspace table are
preserved. Labels were checked against the application source. The description,
contents labels and eight-minute reading time match the rewritten article.
Local source: `frontend/src/landing/docs-interface.ts`.

The parent is `landing-20261027T142813Z-admin-copy`. Matching article HTML and the
single active client entry were patched under the fresh namespace
`interface-copy-20261027t144939z`. Other page content and independently published
download updates are retained. Unchanged files are hard-linked; changed files
were privately replaced. All 11022 public files passed checksum validation,
and both parent and current release checksums passed again after activation.
Free space after preparation was 60,964,864 bytes. Complete rollback releases
and historical assets remain present.

Live HTTP checks matched all 157 article strings and the complete active client
article. The client bundle passed Node24 syntax validation. Hydrated browser
checks confirmed all sections, 78 bold spans, working search for a new
subheading, and no horizontal overflow at desktop or 390-pixel mobile width.
The source passed formatting/import checks and an independent read-only copy
audit. No renderer change or full application rebuild was performed. The
separate pending three-hour guest-duration edit in local source was excluded;
the public article retains the existing one-hour duration.

Evidence: `frontend/test-results/landing/interface-copy-verification.json`,
`interface-copy-qa.json` and `published-interface-copy.jpg`. Draft and scripts:
`/private/tmp/meetspace-interface-copy.4efy5c73`. Future publications must start
from the current server artifact and privately replace hard-linked files.

## Previous administration documentation rewrite

The preceding server release is `landing-20261027T142813Z-admin-copy`, published
on 27 October 2026. Features > Administration and own server now uses plain
Russian with three subheadings, eight bullets and twelve bold spans. It explains
Linux/Docker Compose deployment, domain/HTTPS and media setup, storage,
updates/backups, system status and the separate system-administrator permission.
Both Architecture and Interface links and the original anchor are preserved.
Local source: `frontend/src/landing/docs-product.ts`.

The current server had advanced through a separate publication to
`landing-20261027T114712Z-desktop-020`, which was used as the parent to preserve
the latest desktop packages and all preceding content. Only the targeted section changes
in HTML and the single active entry JS; all public HTML pages were verified to
use that entry. The fresh namespace is `admin-copy-20261027t142813z`; historical
entry files remain unchanged. All10644 public files passed checksum validation.
Unchanged assets are hard-linked; modified files are privately replaced.
Manifest JSON is compact to avoid unnecessary disk use.

Space was constrained during publication. To restore headroom without removing
rollback data, 1741 byte-identical same-path files in the complete
`landing-20261027T111750Z-docs-cards` release were atomically replaced with hard
links to identical files from `landing-20261027T111540Z-overview-copy`. Hash,
size, ownership and mode were checked before each replacement. All files and
complete rollback releases remain present. Afterwards free space was67,710,976
bytes. Checksums for the deduplicated release and current parent were rechecked.
Future publishers must privately replace hard-linked files before editing them.

Live HTTP and hydrated browser checks confirm content, headings, lists, bold
spans, related links and no page overflow; active JS passed Node24 syntax.
No full application rebuild was performed. Evidence:
`frontend/test-results/landing/admin-copy-verification.json`, `admin-copy-qa.json`
and `published-admin-copy.jpg`. Script/inputs:
`/private/tmp/meetspace-admin-copy.i5kuw5b_`. Reconcile future publications with
the current server artifact to retain work published separately from local source.

## Previous optional processing section removal

The preceding server release is `landing-20261027T114256Z-remove-processing`,
published on 27 October 2026. The entire optional-processing section is removed
from Features, including its heading, three paragraphs, four bullets and two
contents links. Current client data and documentation search no longer include
it. Other sections are preserved; twelve remain. Local source:
`frontend/src/landing/docs-product.ts`.

The current notifications release is the parent. The scoped deletion is applied
to feature-page HTML and four current bundles in the fresh immutable namespace
`remove-processing-20261027t114256z`. All other HTML content is byte-identical
after resource URL normalization. Unchanged files are hard-linked; changes are
written privately and atomically replaced. All9888 public-file checksums
passed before activation; parent integrity was checked again afterwards.
Previous complete releases and immutable assets remain available for rollback.

Live browser and HTTP checks confirm absence of the section and its contents
links after hydration, twelve remaining sections and no search result for
removed text. The active client bundle passed a Node24 syntax check.
Evidence: `frontend/test-results/landing/remove-processing-verification.json`,
`remove-processing-qa.json` and `published-remove-processing.jpg`. Script:
`/private/tmp/meetspace-remove-processing.9pp6b2na`. No renderer change or full
application rebuild was required. Future publishers must preserve separately
published server changes and use private replacement for hard-linked files.

## Previous notifications documentation copy update

The preceding server release is `landing-20261027T114054Z-notifications`, published
on 27 October 2026. Features > Notifications and reminders now uses the exact
user-supplied copy, with three subheadings, six bullets in two lists and eight
bold spans. It covers built-in notifications, per-chat settings, external
delivery and one-way calendar integration. Local source:
`frontend/src/landing/docs-product.ts`.

The account-copy release is the parent. Only the notifications section changes
in HTML with matching data in four current JS bundles under
`notifications-20261027t114054z`; all other HTML text is preserved after resource
URL normalization. No renderer changes were required. Both unchanged site
files and namespace assets are hard-linked; modified files use private atomic
replacements to preserve parent bytes. All9510 public-file checksums passed
before activation. Parent release integrity was checked afterwards. Previous
assets and complete releases remain available for rollback.

Live browser and HTTP checks confirmed exact paragraphs, heading/list order,
bold spans, no page overflow and matching client data. The active bundle passed
a Node24 syntax check; no full application rebuild was run. Evidence:
`frontend/test-results/landing/notifications-copy-verification.json`,
`notifications-copy-qa.json` and `published-notifications-copy.jpg`. Inputs and
patch script: `/private/tmp/meetspace-notifications.p_2nqs2y`. Future publishing
must preserve separately published server changes and replace hard-linked
files privately before modification.

## Previous account and preferences copy update

The preceding server release is `landing-20261027T113854Z-account-copy`, published
on 27 October 2026. Features > Accounts, devices and appearance now uses the
exact user-supplied registration/session, preferences, audio/video, appearance
and deletion copy. Four subheadings, six settings bullets, three numbered
removal steps and ten bold spans render in the supplied order. The previous
password/email limitations note was removed as part of the requested replacement.

Sources: `frontend/src/landing/docs-product.ts`, `docs.ts` and
`frontend/src/pages/DocumentationPage.tsx`. Subsections now support optional
steps rendered as ordered lists after any bullets and before closing paragraphs;
steps are included in search. Existing subsection behavior is preserved.

The current chat-materials release is the parent. Only the targeted section
changes in HTML and four current bundles receive matching data, ordered-list
rendering and search updates in `account-copy-20261027t113854z`. Other HTML text
is unchanged apart from resource URLs. Unchanged files are hard-linked; changed
HTML uses private atomic replacement and the namespace has private copies.
All9132 public-file checksums passed before activation. The active bundle
passed a Node24 syntax check before activation. No full app build was run.

Browser hydration and live HTTPS checks verified exact content, four headings,
six bullets, three numbered steps, ten bold spans, no page overflow and search
matching words from a deletion step. Evidence:
`frontend/test-results/landing/account-copy-verification.json`,
`account-copy-qa.json`, `published-account-copy.jpg`. Inputs/scripts:
`/private/tmp/meetspace-account-copy.l_8g8h5g`. Parent rollback files were checked
after publication. Preserve separately published updates and use private
replacement before modifying hard-linked files in future releases.

## Previous meeting chat documentation rewrite

The preceding server release is `landing-20261027T113621Z-chat-materials`, published
on 27 October 2026. Features > Meeting chat and shared materials now uses clear
Russian with four subheadings: communication, finding materials, messages and
attachments, and access after the meeting. Nine bullets and fifteen bold spans
highlight actions, private bookmarks, limits and access checks. The existing
4000-character, five-file and10MiB limits, six published formats, moderation
scope and read-only post-meeting access are preserved.

Local source: `frontend/src/landing/docs-product.ts`. A separate pending local
claim about expanded attachment formats is retained in local source but not
published; this rewrite preserves the six formats already documented publicly.
The current invitations-copy release is the parent. Only the meeting-chat
section changes in HTML and matching data in four client bundles under
`chat-materials-20261027t113621z`. Other HTML content is byte-identical after
resource URL normalization. No renderer or application behavior changed.

The first staging attempt ran out of disk space. Only this task's incomplete
stage and the four incomplete, unpublished conference-copy attempts created
earlier in this chat were removed. Active and complete rollback releases were
retained. The successful release hard-links unchanged files from its parent;
changed HTML is written to private temporary inodes and atomically replaced.
The new namespace contains private copies. All8754 public-file checksums
passed before activation; the parent release was checked again afterwards to
confirm its bytes remain unchanged. Future publishers must use atomic private
replacements for any hard-linked files and avoid repeatedly copying retained
historical assets into new physical copies.

Live browser and HTTP checks confirm content, structure, no page overflow and
matching client data. The active bundle passed a Node24 syntax check. No full
application rebuild was run. Evidence:
`frontend/test-results/landing/chat-materials-verification.json`,
`chat-materials-qa.json` and `published-chat-materials.jpg`. Patch inputs:
`/private/tmp/meetspace-chat-materials.2bp0wq9c`. Future publications must retain
separately published server changes beyond local source.

## Previous invitations and moderation copy update

The preceding server release is `landing-20261027T113201Z-invitations-copy`,
published on 27 October 2026. Features > Invitations, waiting room and moderation
uses the exact user-supplied text: three guest-access paragraphs, participant
management with four bullets, role/device explanations and invitation delivery
paragraphs. Guest session copy reads «один час» as requested. Two subheadings
and eight bold spans are rendered. Local source:
`frontend/src/landing/docs-product.ts`.

The current conference-copy release is preserved as the parent. Only the
access-and-moderation section changes in HTML, with matching serialized section
data in four current client bundles under `invitations-copy-20261027t113201z`.
No renderer change was required. All other HTML content remains byte-identical
after resource URL normalization. All 8376 public files passed checksums before
activation; preceding releases and immutable assets remain available.

Live browser and HTTP verification confirmed every requested paragraph, bullet,
bold span and heading, absence of horizontal overflow and matching client data.
The active bundle passed a Node24 syntax check. Evidence:
`frontend/test-results/landing/invitations-copy-verification.json`,
`invitations-copy-qa.json` and `published-invitations-copy.jpg`. Patch files:
`/private/tmp/meetspace-invitations-copy.kq3axcdu`. No full application rebuild
or backend behavior change was performed. Future publishing must reconcile
with current server artifacts to retain separately published work.

## Previous conference capabilities copy update

The preceding server release is `landing-20261027T112854Z-conference-copy`,
published on 27 October 2026. Features > Video meetings now has the supplied
«Возможности во время конференции» and «Особенности работы» subsections,
six feature bullets, the screen-share explanation and three keyboard shortcuts.
The preceding room-preparation paragraph and existing anchor are preserved.
The old controls paragraph and four bullets are replaced as requested.

Sources: `frontend/src/landing/docs-product.ts`, `docs.ts` and
`frontend/src/pages/DocumentationPage.tsx`. Subsections gain optional
introParagraphs rendered before bullets and included in documentation search;
existing subsection paragraphs keep their order after bullets.

The update uses current server artifact `landing-20261027T111750Z-docs-cards`.
Only the requested section changes in HTML; all other HTML text is unchanged
apart from resource URLs. Four current client bundles have matching section,
rendering and search updates in `conference-copy-20261027t112854z`. Historical
bundles remain unchanged. All four changed bundles passed syntax checks with
local Node24; the server Node12 parser is too old for the existing bundle
syntax. No full application build was run. All 7998 public files passed remote
checksum verification before activation; current documentation card CSS and
separately published package updates are retained.

Live HTTP and browser checks confirm exact content order, two subheadings,
lists of six and three items, ten bold spans, no page overflow and successful
search for text in a leading paragraph. Evidence:
`frontend/test-results/landing/conference-copy-verification.json`,
`conference-copy-qa.json`, and `published-conference-copy.jpg`. Patch inputs
and checked bundles are in `/private/tmp/meetspace-conference-copy.bwrgwpr5`.
Previous releases and assets are retained for rollback.

## Previous documentation quick-start layout update

The preceding server release is `landing-20261027T111750Z-docs-cards`, published
on 27 October 2026. The overview's four quick-start cards now align icon,
heading and description rows using CSS subgrid. Headings are 17px/1.45 and
descriptions 15px/1.65, with consistent spacing and a single column below640px.
The scoped rules fix body-text cascade overriding small card headings and keep
wrapped headings from shifting adjacent descriptions. Local source:
`frontend/src/pages/documentation.css`.

The current published artifact `landing-20261027T111540Z-overview-copy` was
copied and supplemented with `assets/docs-cards-1067040871c5.css`. The stylesheet
is linked last on all 13 documentation pages so direct access and client-side
navigation both receive the same styling. All HTML text, JS, existing assets,
download versions and verification files are preserved byte for byte. Only the
supplemental stylesheet links are added to HTML. All 7620 public files passed
checksums before activation; previous release remains available for rollback.

Live browser checks at1280px confirmed paired headings and body rows have
identical vertical positions, equal card heights within each row, and17/15px
fonts. At390px cards form a single342px column without clipping or horizontal
overflow. Screenshot: `frontend/test-results/landing/published-doc-cards.jpg`.
QA data: `frontend/test-results/landing/doc-cards-layout-qa.json`. The patch and
CSS are in `/private/tmp/meetspace-doc-cards.d00l8a9q`. No JS rebuild was required.
Future publications must preserve server artifact changes not yet in local source.

## Previous documentation overview copy update

The preceding server release is `landing-20261027T111540Z-overview-copy`, published
on 27 October 2026. The three About MeetSpace overview paragraphs use the exact
text supplied by the user. The MeetSpace word retains existing bold emphasis;
other paragraph text is plain. Local source: `frontend/src/landing/docs.ts`.

The current server artifact `landing-20261027T104804Z-desktop-0114` was copied
into a new release, preserving separate publications including Desktop0.1.14-beta.
Only the about-meetspace section on `/docs` and `/docs/overview` changes in HTML,
plus current resource URLs. Four copied JS bundles receive matching source
paragraph replacements in a fresh namespace `overview-copy-20261027t111540z`.
All other page text remains byte-identical after URL normalization. All 7619
public files passed checksum validation before the atomic activation. Parent
release and previous immutable assets remain available for rollback.

Live HTTPS verification confirms all three exact paragraphs, absence of old
copy, the overview alias and matching client bundle. Evidence:
`frontend/test-results/landing/overview-copy-verification.json`. Patch scripts:
`/private/tmp/meetspace-overview-copy.wmuq78zb`. No full application rebuild was
performed. Future publication must reconcile with the current server artifact
to retain separately published updates.

## Previous self-hosted wording update

The preceding server release is `landing-20261027T095029Z-server-copy`, published
on 27 October 2026. The shared self-hosted description now says
«Запускайте MeetSpace на собственном сервере» using the exact replacement
supplied by the user. Local source: `frontend/src/landing/content.ts`.

The current artifact `landing-20261027T091404Z-calendar-copy` was copied into a
new release. The requested string changes on `/`, `/welcome` and `/self-hosted`
and in fifteen copied JS bundles under the fresh immutable namespace
`server-copy-20261027t095029z`. Other HTML remains byte-identical after resource
URL normalization. The release contains 6863 public files; checksums passed
before activation. Previous releases and cached assets remain available.

Live HTTPS checks confirmed exact new copy and absence of the old copy on all
three routes and in the active client bundle. Android 0.3.12-beta remains
published. Evidence: `frontend/test-results/landing/server-copy-verification.json`.
Scripts: `/private/tmp/meetspace-server-copy.5k5nb7hj`. Future releases must
reconcile with the current server artifact to retain separately published work.

## Previous calendar description update

The preceding server release is `landing-20261027T091404Z-calendar-copy`, published
on 27 October 2026. The Calendar feature card description uses the exact text
supplied by the user, including its em dash and trailing full stop. The title
«Календарь встреч» is unchanged. Local source: `frontend/src/landing/content.ts`.

The current artifact `landing-20261027T090236Z-personal-copy` was copied into
a new release. The single requested string was changed on `/` and `/welcome`
and in fifteen client bundles under the fresh immutable namespace
`calendar-copy-20261027t091404z`. Other HTML is byte-identical after normalizing
resource URLs. The preceding personal messages change, latest Android package,
verification files and other content are preserved. The release contains 6485
public files; all passed checksum validation before activation. Previous
releases and immutable assets are retained for rollback.

Live HTTPS checks confirmed exact copy on both landing routes and the active
client bundle, absence of old copy, preserved personal messages text and Android
0.3.12-beta. Evidence: `frontend/test-results/landing/calendar-copy-verification.json`.
The patch scripts are in `/private/tmp/meetspace-calendar-copy.g17e9nls`.
Future publications must reconcile with current server artifacts because local
source does not include all separately published download updates.

## Previous personal messages description update

The preceding server release is `landing-20261027T090236Z-personal-copy`, published
on 27 October 2026. The Personal messages feature card uses the exact replacement
text supplied by the user, including its em dash and no trailing full stop.
Local source: `frontend/src/landing/content.ts`.

The patch uses current server release
`landing-20261027T022342228438Z-android-0.3.12-beta` as its parent, preserving
separately published download updates, verification files and documentation.
HTML copy changes are limited to `/` and `/welcome`; all pages use a fresh
immutable asset namespace `personal-copy-20261027t090236z`. Fifteen copied JS
bundles receive the same exact string replacement. All other HTML text is
byte-identical after normalizing resource URLs. Previous assets and the parent
release are retained for rollback. The package contains 6107 public files;
all passed checksums before activation. No full application rebuild was run.

Live HTTPS checks confirmed the exact replacement on both pages and in the
active client bundle, absence of the old text, and preserved Android 0.3.12-beta.
Evidence: `frontend/test-results/landing/personal-messages-copy-verification.json`.
The script and supplied text are in `/private/tmp/meetspace-personal-copy.u9q3evcv`.
Current production contains additional publications beyond local source; future
changes must reconcile against the current server artifact before deployment.

## Previous desktop download update

The preceding release is `landing-20261025T095000Z-desktop-016`, published on 25 October 2026 (Moscow time).
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
