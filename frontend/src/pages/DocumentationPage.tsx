import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  BookOpen,
  Check,
  ChevronRight,
  Code2,
  Copy,
  FileText,
  Globe2,
  Info,
  Mail,
  Menu,
  Search,
  Send,
  Server,
  Terminal,
  X,
} from "lucide-react";
import { BRAND_ASSET_VERSION } from "../brand";
import { applicationPath, MARKETING_ONLY } from "../landing/application";
import {
  documentationArticles,
  documentationGroups,
  documentationPath,
  type DocumentationSection,
} from "../landing/docs";
import "./documentation.css";

function renderParagraphText(text: string) {
  return text
    .split(/(\*\*[^*]+\*\*)/g)
    .map((part, index) =>
      index % 2 === 1 ? <strong key={index}>{part.slice(2, -2)}</strong> : part,
    );
}

function CodeBlock({
  code,
}: {
  code: NonNullable<DocumentationSection["code"]>;
}) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  async function copy() {
    try {
      await navigator.clipboard.writeText(code.value);
      setStatus("copied");
    } catch {
      setStatus("failed");
    }
  }
  return (
    <div className="docs-code">
      <div className="docs-code-bar">
        <span>
          <Terminal size={14} /> {code.language}
        </span>
        <button type="button" onClick={copy} aria-label="Скопировать код">
          {status === "copied" ? <Check size={14} /> : <Copy size={14} />}
          {status === "copied" ? "Скопировано" : "Скопировать"}
        </button>
      </div>
      <pre tabIndex={0}>
        <code>{code.value}</code>
      </pre>
      <span className="docs-sr" role="status">
        {status === "copied"
          ? "Код скопирован."
          : status === "failed"
            ? "Не удалось скопировать. Выделите код и скопируйте его вручную."
            : ""}
      </span>
    </div>
  );
}

function ArchitectureMap() {
  return (
    <figure
      className="docs-architecture"
      aria-label="Архитектура сервиса MeetSpace"
    >
      <div className="docs-diagram-top">
        <Globe2 size={19} />
        <strong> Веб-, десктопные и Android-клиенты </strong>
        <span> HTTPS · WebSocket · WebRTC </span>
      </div>
      <div className="docs-diagram-arrows" aria-hidden="true">
        <ArrowDown size={19} />
      </div>
      <div className="docs-diagram-row">
        <div>
          <strong> Веб-шлюз </strong>
          <span> Фронтенд и обратный прокси-сервер </span>
        </div>
        <div>
          <strong> Go API </strong>
          <span> Аккаунты, чаты и доступ к встречам </span>
        </div>
        <div>
          <strong> Медиа SFU + TURN </strong>
          <span> Аудио, видео и демонстрация экрана </span>
        </div>
      </div>
      <div className="docs-diagram-arrows" aria-hidden="true">
        <ArrowDown size={19} />
      </div>
      <div className="docs-diagram-row docs-diagram-storage">
        <div>
          <strong> PostgreSQL </strong>
          <span> Данные приложения </span>
        </div>
        <div>
          <strong> Redis </strong>
          <span> Присутствие и координация </span>
        </div>
        <div>
          <strong> RabbitMQ </strong>
          <span> Команды управления записью </span>
        </div>
        <div>
          <strong> Хранилище S3 </strong>
          <span> Записи и вложения </span>
        </div>
      </div>
      <figcaption>
        {" "}
        Рабочие процессы записи и дополнительные интеграции работают параллельно
        с основными сервисами. Медиатрафик и трафик приложений используют
        отдельные пути.{" "}
      </figcaption>
    </figure>
  );
}

const quickStarts = [
  {
    slug: "interface",
    title: "Познакомьтесь со своим рабочим пространством",
    description: "Найдите встречи, чаты, записи и настройки аккаунта.",
    icon: BookOpen,
  },
  {
    slug: "installation",
    title: "Разверните собственный сервер",
    description:
      "Следуйте инструкциям установщика или настройте развертывание вручную.",
    icon: Server,
  },
  {
    slug: "features",
    title: "Изучите возможности",
    description: "Узнайте, что доступно и что требует настройки.",
    icon: Globe2,
  },
  {
    slug: "troubleshooting",
    title: "Решите проблему",
    description:
      "Устраните проблемы со входом, медиафайлами, записями и развертыванием.",
    icon: Terminal,
  },
];

function DocumentationSearch() {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const wrapper = useRef<HTMLDivElement>(null);
  const results = useMemo(() => {
    const terms = query.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
    if (!terms.length) return [];
    return documentationArticles
      .flatMap((article) => {
        const sections = article.sections.filter((section) => {
          const text = [
            article.title,
            section.title,
            ...(section.paragraphs || []),
            ...(section.subsections?.flatMap(
              ({ title, bullets, paragraphs }) => [
                title,
                ...(bullets || []),
                ...paragraphs,
              ],
            ) || []),
            ...(section.bullets || []),
            ...(section.steps || []),
            ...(section.actions?.map(({ label }) => label) || []),
            section.note || "",
            section.code?.value || "",
            ...(section.table?.headings || []),
            ...(section.table?.rows.flat() || []),
            ...(section.screenshots?.flatMap(({ alt, caption }) => [
              alt,
              caption,
            ]) || []),
          ]
            .join(" ")
            .replace(/\*\*/g, "")
            .toLocaleLowerCase();
          return terms.every((term) => text.includes(term));
        });
        return sections.map((section) => ({ article, section }));
      })
      .slice(0, 8);
  }, [query]);
  useEffect(() => {
    function outside(event: PointerEvent) {
      if (!wrapper.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, []);
  return (
    <div
      className="docs-search"
      ref={wrapper}
      onKeyDown={(event) => {
        if (event.key === "Escape") setOpen(false);
      }}
    >
      <Search size={18} aria-hidden="true" />
      <input
        aria-label="Поиск по документации"
        placeholder="Поиск по документации…"
        type="search"
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        aria-expanded={open && !!query.trim()}
        aria-controls="docs-search-results"
      />
      {open && !!query.trim() && (
        <div className="docs-search-results" id="docs-search-results">
          <p role="status">
            {results.length
              ? "Подходящие разделы"
              : "Нет подходящих разделов. Попробуйте ввести «запись», «Docker» или «чат»."}
          </p>
          {results.map(({ article, section }) => (
            <Link
              key={`${article.slug}-${section.id}`}
              to={`${documentationPath(article.slug)}#${section.id}`}
              onClick={() => {
                setOpen(false);
                setQuery("");
              }}
            >
              <FileText size={17} aria-hidden="true" />
              <span>
                <strong>{section.title}</strong>
                <small>{article.title}</small>
              </span>
              <ChevronRight size={15} aria-hidden="true" />
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

export function DocumentationPage({ slug = "overview" }: { slug?: string }) {
  const article =
    documentationArticles.find((item) => item.slug === slug) ||
    documentationArticles[0];
  const index = documentationArticles.indexOf(article);
  const group = documentationGroups.find((item) =>
    item.slugs.includes(article.slug),
  )!;
  const [menuOpen, setMenuOpen] = useState(false);
  const { hash } = useLocation();
  useEffect(() => {
    setMenuOpen(false);
    if (hash) {
      const id = decodeURIComponent(hash.slice(1));
      document.getElementById(id)?.scrollIntoView();
    } else window.scrollTo(0, 0);
  }, [article.slug, hash]);
  const contents = (
    <>
      {article.sections.map((section) => (
        <a key={section.id} href={`#${section.id}`}>
          {section.title}
        </a>
      ))}
    </>
  );
  const wordCount = JSON.stringify(article).split(/\s+/).length;
  const isLinkOnlyArticle = article.sections.every(
    ({
      paragraphs,
      subsections,
      bullets,
      steps,
      code,
      note,
      table,
      links,
      actions,
      screenshots,
    }) =>
      links?.length &&
      !paragraphs &&
      !subsections?.length &&
      !bullets &&
      !steps &&
      !code &&
      !note &&
      !table &&
      !actions?.length &&
      !screenshots?.length,
  );

  return (
    <div className="docs-site">
      <a className="docs-skip" href="#documentation-main">
        {" "}
        Перейти к содержимому{" "}
      </a>
      <header className="docs-header">
        <div className="docs-header-inner">
          <Link
            className="docs-brand"
            to={MARKETING_ONLY ? "/" : "/welcome"}
            aria-label="Главная страница MeetSpace"
          >
            <img
              src={`/branding/meetspace-symbol-color.svg?v=${BRAND_ASSET_VERSION}`}
              alt=""
              width="32"
              height="32"
            />
            <span>
              Meet<span>Space</span>
            </span>
          </Link>
          <span className="docs-header-divider" />
          <Link className="docs-header-label" to="/docs">
            {" "}
            Документация{" "}
          </Link>
          <DocumentationSearch />
          <a
            className="docs-source-link"
            href="https://github.com/janickiy/meet-space"
            target="_blank"
            rel="noopener noreferrer"
          >
            <Code2 size={17} /> <span> Исходный код </span>
            <ArrowUpRight size={14} />
          </a>
          <Link className="docs-open-app" to={applicationPath("/login")}>
            {" "}
            Открыть приложение <ArrowUpRight size={15} />
          </Link>
        </div>
      </header>
      <div className="docs-mobile-nav">
        <button
          type="button"
          aria-expanded={menuOpen}
          aria-controls="docs-sidebar"
          onClick={() => setMenuOpen(!menuOpen)}
        >
          {menuOpen ? <X size={18} /> : <Menu size={18} />} Разделы
          документации{" "}
        </button>
        <span>RU</span>
      </div>
      <div className="docs-layout">
        <aside
          className={`docs-sidebar ${menuOpen ? "is-open" : ""}`}
          id="docs-sidebar"
        >
          <Link className="docs-sidebar-home" to="/docs">
            <BookOpen size={18} /> Руководство MeetSpace <span>RU</span>
          </Link>
          <nav aria-label="Главы документации">
            {documentationGroups.map((item) => (
              <div className="docs-nav-group" key={item.title}>
                <p>{item.title}</p>
                {item.slugs.map((itemSlug) => {
                  const page = documentationArticles.find(
                    (value) => value.slug === itemSlug,
                  )!;
                  return (
                    <Link
                      key={itemSlug}
                      to={documentationPath(itemSlug)}
                      aria-current={
                        article.slug === itemSlug ? "page" : undefined
                      }
                    >
                      {itemSlug === "overview" ? "Обзор" : page.title}
                    </Link>
                  );
                })}
              </div>
            ))}
          </nav>
          <div className="docs-sidebar-help">
            <span> Нужна помощь? </span>
            <Link to="/docs/support">
              {" "}
              Техническая поддержка <ArrowRight size={15} />
            </Link>
          </div>
          <Link
            className="docs-back-site"
            to={MARKETING_ONLY ? "/" : "/welcome"}
          >
            <ArrowLeft size={14} /> Вернуться на сайт{" "}
          </Link>
        </aside>
        <main className="docs-main" id="documentation-main">
          <nav className="docs-breadcrumb" aria-label="Навигация">
            <Link to="/docs"> Документация </Link>
            <ChevronRight size={13} />
            <span>{group.title}</span>
          </nav>
          <article key={article.slug}>
            <header className="docs-article-header">
              <div className="docs-eyebrow">
                {article.slug === "overview"
                  ? "РУКОВОДСТВО ПО MEETSPACE"
                  : group.title}
                {!isLinkOnlyArticle && (
                  <span>
                    {Math.max(2, Math.ceil(wordCount / 220))} мин на чтение{" "}
                  </span>
                )}
              </div>
              <h1>{article.title}</h1>
              {!isLinkOnlyArticle && (
                <p className="docs-lead">{article.description}</p>
              )}
              {!isLinkOnlyArticle && (
                <div className="docs-article-meta">
                  <span className="docs-status-dot" /> Документация на русском{" "}
                  {article.slug !== "overview" &&
                    (article.slug === "installation" ? (
                      <>
                        <span>·</span> Проверено 23 октября 2026 г.{" "}
                      </>
                    ) : (
                      <>
                        <span>·</span> Проверено 20 октября 2026 г.{" "}
                      </>
                    ))}
                </div>
              )}
            </header>
            {!isLinkOnlyArticle && (
              <details className="docs-mobile-toc">
                <summary> На этой странице </summary>
                <nav aria-label="Содержание статьи">{contents}</nav>
              </details>
            )}
            {article.slug === "architecture" && <ArchitectureMap />}
            {article.sections.map((section) => (
              <section
                className="docs-section"
                id={section.id}
                key={section.id}
              >
                {!isLinkOnlyArticle && (
                  <h2 className={section.hideHeading ? "docs-sr" : undefined}>
                    {section.hideHeading ? (
                      section.title
                    ) : (
                      <a href={`#${section.id}`}>
                        {section.title}
                        <span aria-hidden="true">#</span>
                      </a>
                    )}
                  </h2>
                )}
                {section.paragraphs?.map((paragraph) => (
                  <p key={paragraph}>{renderParagraphText(paragraph)}</p>
                ))}
                {section.subsections?.map((subsection) => (
                  <div className="docs-subsection" key={subsection.title}>
                    <h3>{subsection.title}</h3>
                    {subsection.bullets && (
                      <ul>
                        {subsection.bullets.map((bullet) => (
                          <li key={bullet}>{renderParagraphText(bullet)}</li>
                        ))}
                      </ul>
                    )}
                    {subsection.paragraphs.map((paragraph) => (
                      <p key={paragraph}>{renderParagraphText(paragraph)}</p>
                    ))}
                  </div>
                ))}
                {section.screenshots?.map((screenshot) => (
                  <figure className="docs-screenshot" key={screenshot.src}>
                    <a
                      className="docs-screenshot-link"
                      href={screenshot.src}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={`Открыть скриншот «${screenshot.caption}» в полном размере в новой вкладке`}
                    >
                      <img
                        src={screenshot.src}
                        alt={screenshot.alt}
                        width={screenshot.width}
                        height={screenshot.height}
                        style={{
                          width: `min(100%, ${Math.min(screenshot.width, (640 * screenshot.width) / screenshot.height)}px)`,
                          aspectRatio: `${screenshot.width} / ${screenshot.height}`,
                        }}
                        loading="lazy"
                        decoding="async"
                      />
                      <span className="docs-screenshot-open" aria-hidden="true">
                        Открыть в полном размере <ArrowUpRight size={14} />
                      </span>
                    </a>
                    <figcaption>{screenshot.caption}</figcaption>
                  </figure>
                ))}
                {section.bullets && (
                  <ul>
                    {section.bullets.map((bullet) => (
                      <li key={bullet}>{renderParagraphText(bullet)}</li>
                    ))}
                  </ul>
                )}
                {section.steps && (
                  <ol>
                    {section.steps.map((step) => (
                      <li key={step}>{renderParagraphText(step)}</li>
                    ))}
                  </ol>
                )}
                {section.table && (
                  <div
                    className="docs-table-wrap"
                    role="region"
                    aria-label={section.title}
                    tabIndex={0}
                  >
                    <table>
                      <thead>
                        <tr>
                          {section.table.headings.map((heading) => (
                            <th scope="col" key={heading}>
                              {heading}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {section.table.rows.map((row, rowIndex) => (
                          <tr key={rowIndex}>
                            {row.map((cell, cellIndex) => (
                              <td key={cellIndex}>{cell}</td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                {section.code && <CodeBlock code={section.code} />}
                {section.note && (
                  <aside className="docs-note">
                    <Info size={18} aria-hidden="true" />
                    <p>{renderParagraphText(section.note)}</p>
                  </aside>
                )}
                {section.actions && (
                  <div className="docs-contact-actions">
                    {section.actions.map((action) => (
                      <a
                        key={action.href}
                        href={action.href}
                        target={
                          action.icon === "telegram" ? "_blank" : undefined
                        }
                        rel={
                          action.icon === "telegram"
                            ? "noopener noreferrer"
                            : undefined
                        }
                      >
                        {action.icon === "email" ? (
                          <Mail size={18} aria-hidden="true" />
                        ) : (
                          <Send size={18} aria-hidden="true" />
                        )}
                        {action.label}
                      </a>
                    ))}
                  </div>
                )}
                {section.links && (
                  <div
                    className={`docs-related-links${isLinkOnlyArticle ? " docs-repository-link" : ""}`}
                  >
                    {section.links.map((link) =>
                      link.href.startsWith("/") ? (
                        <Link key={link.href} to={link.href}>
                          {link.label}
                          <ArrowRight size={14} />
                        </Link>
                      ) : (
                        <a
                          key={link.href}
                          href={link.href}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {link.label}
                          <ArrowUpRight size={14} />
                        </a>
                      ),
                    )}
                  </div>
                )}
                {article.slug === "overview" && section.id === "start-here" && (
                  <div className="docs-quick-starts">
                    {quickStarts.map((item) => (
                      <Link key={item.slug} to={documentationPath(item.slug)}>
                        <div className="docs-quick-icon">
                          <item.icon size={21} />
                        </div>
                        <ArrowUpRight className="docs-quick-arrow" size={19} />
                        <h3>{item.title}</h3>
                        <p>{item.description}</p>
                      </Link>
                    ))}
                  </div>
                )}
              </section>
            ))}
            <nav
              className="docs-pagination"
              aria-label="Предыдущая и следующая статьи"
            >
              {index > 0 ? (
                <Link
                  to={documentationPath(documentationArticles[index - 1].slug)}
                >
                  <ArrowLeft size={18} />
                  <span>
                    <small> Предыдущая </small>
                    {documentationArticles[index - 1].slug === "overview"
                      ? "Обзор"
                      : documentationArticles[index - 1].title}
                  </span>
                </Link>
              ) : (
                <span />
              )}
              {index < documentationArticles.length - 1 && (
                <Link
                  className="docs-next"
                  to={documentationPath(documentationArticles[index + 1].slug)}
                >
                  <span>
                    <small> Следующая </small>
                    {documentationArticles[index + 1].title}
                  </span>
                  <ArrowRight size={18} />
                </Link>
              )}
            </nav>
          </article>
          <footer className="docs-footer">
            <span> © 2026 MeetSpace · Яницкий Александр </span>
            <Link to="/docs/support">
              {" "}
              Сообщить о проблеме с документацией <ArrowUpRight size={13} />
            </Link>
          </footer>
        </main>
        {!isLinkOnlyArticle && (
          <aside className="docs-toc">
            <p> На этой странице </p>
            <nav aria-label="На этой странице">{contents}</nav>
            <div className="docs-toc-help">
              <BookOpen size={18} />
              <strong> Создано для вашей команды </strong>
              <span> Одно место для встреч, общения и совместной работы. </span>
              <Link to="/download">
                {" "}
                Скачать MeetSpace <ArrowUpRight size={14} />
              </Link>
            </div>
          </aside>
        )}
      </div>
    </div>
  );
}
