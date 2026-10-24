# Public documentation

Reviewed on 20 October 2026.

## Scope

The landing links to `/docs`. It contains an overview and ten Russian articles:
features, architecture, interface, support, preparation, system requirements,
source and releases, installation, updates and backups, and troubleshooting.
The existing Russian marketing pages retain their language.

The layout follows the requested documentation pattern: grouped chapter
navigation, an article column, section contents, previous/next navigation and
full-text section search. Search covers prose, lists, commands and tables.
The interface includes mobile chapter navigation, responsive tables, copyable
commands, print styles and a service architecture diagram.

## Evidence and source boundaries

- Product and architecture content was audited against local source revision
  `893566bb90f07bdcec4542eec5e00116c423e852`, including registered HTTP routes,
  permissions, UI components, `USER_GUIDE.md`, `ARCHITECTURE.md`, `DATA_FLOWS.md`,
  direct-call, group-chat, account and folder implementations.
- The public repository and GitHub issue/release destinations were inspected.
  The ispmanager page was used for information hierarchy, not copied content.
- The live web application was inspected using authorized sign-in. Observations
  covered Home, Meetings, Personal, Calendar, Recordings, Folders, account
  settings and the create-meeting form. No meeting, chat, account setting or
  production deployment was created or modified. No private conversation,
  credential or screenshot from the signed-in account is included in the docs.
- Active meeting behavior is documented from the implementation and user guide.
  No live call or recording was started for this documentation task.
- The published `downloads/install.sh`, checksum and source metadata were
  inspected. The script selected
  `12ba606bb98c10638fa6512d970f57c4ebe70de0`. Its bootstrap, source builder, wizard,
  deployment configuration and release/backup tools were reviewed read-only.
  Installation source links are pinned to that revision.
- Installation, update, backup and restore commands were not executed against
  a server. Thirteen Bash examples were checked with `bash -n`.

Important distinctions are explicit in the articles: valid invitation links
bypass waiting admission but not removal/rejection; recordings expire after
seven days; personal/group chat has no shared-media gallery; optional providers
must be configured; classic object storage and the hardened installer differ.
The installer has no update command, and its generated project/layout cannot
use the operator release and backup commands unchanged.

## Implementation

- `frontend/src/landing/docs.ts`: types, article ordering and chapter groups.
- `docs-product.ts`, `docs-interface.ts`, `docs-install.ts`: content.
- `frontend/src/pages/DocumentationPage.tsx` and `documentation.css`: reader,
  search, code copying, navigation and diagram.
- Public routing, metadata, SSR, Vite preview and Nginx serve every article
  directly without authentication or API providers.
- `/docs/overview` is an alias of `/docs`; `/docs/self-hosted` displays the
  installation article with canonical `/docs/installation`. The old article's
  fragment names are not preserved.
- Each article has Russian document language, social metadata, structured data
  and a canonical URL. The sitemap excludes aliases.

## Validation

- Production TypeScript/Vite build succeeds.
- `check-public-build.mjs` passes for all 17 public documents, including one H1,
  document language, canonical metadata, sitemap, local assets, protected
  external links, article links and cross-page fragment targets.
- Sitemap contains 14 canonical pages: eleven documentation pages and three
  marketing pages.
- Alias article content, selected sidebar entry and section ID uniqueness were
  independently checked.
- Browser checks at desktop and 390 px mobile width confirm no horizontal page
  overflow, working mobile chapter navigation and search for a table-only port
  number. Search navigates to the correct section below the sticky header.
- Copying a command reports success. Browser console inspection found no
  warning or error for the checked documentation pages.
- The existing large JavaScript chunk warning remains a build advisory.

## Russian translation

On 20 October 2026, the complete text was translated through the English-to-Russian
interface at https://translate.yandex.ru/. Nine text batches covered 637 unique
article and interface strings; two additional batches covered command comments
and architecture labels. The machine translation was reviewed and corrected for
technical meaning, service names, configuration variables, units, admission rules
and the release drain procedure. Navigation, search, accessibility labels and
metadata are also Russian.

An AST comparison verified that 141 technical identifiers and links and all
11 command bodies remain unchanged. Six command comments and the explanatory
architecture diagram were translated. The source-and-releases article still
contains only the requested repository link.

The local preview runs at `http://127.0.0.1:4183/docs`. The documentation is also
published at `https://meet-space.janickiy.com/docs`.


## Real interface screenshots

On 20 October 2026, nine JPEG screenshots were captured directly from the
running web application at `https://meeting.janickiy.com/` through its browser UI.
The interface article now illustrates workspace navigation, the scheduling form
and calendar, audio settings, the conference room, an empty group chat, the
recording library, folder creation and appearance settings.

The images in `frontend/public/media/docs/` are original browser captures;
framing excludes account email and private conversation content. The scheduling
and folder forms were captured as unsaved drafts and cancelled. The room image
shows the existing interface before connecting media, with camera and microphone
off; its caption states that condition. No meeting, folder, message, recording
or account setting was created or changed to produce these screenshots.

Each image has Russian alternative text, a factual caption and a link to its
full size. The reader reserves the source aspect ratio before lazy loading,
limits tall captures to 640px, and preserves the complete image on narrow screens.
The public-build checker validates both screenshot images and their local
full-size links. Production build evidence is in
`frontend/test-results/landing/interface-screenshots-build.log`.


## Simplified installer download

At the user's request, the first installation step now uses only
`wget https://meeting.janickiy.com/downloads/install.sh`. The previous working
directory setup, curl flags and inline checksum/inspection sequence were removed
from this step. Direct installer and SHA-256 links remain available.
The download was checked for HTTPS 200; the installer was not executed.
