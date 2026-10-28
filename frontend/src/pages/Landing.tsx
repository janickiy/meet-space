import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import {
  ArrowDown,
  ArrowRight,
  ArrowUpRight,
  CalendarDays,
  Check,
  Code2,
  Download,
  FileText,
  FolderOpen,
  Globe2,
  Menu,
  MessageCircle,
  Monitor,
  MonitorUp,
  Plus,
  Server,
  ShieldCheck,
  Smartphone,
  Users,
  Video,
  X,
  Send,
  GraduationCap,
  Building2,
  Heart,
  Database,
  CirclePlay,
} from "lucide-react";
import { LandingScene } from "./LandingScene";
import { PlatformLogo } from "../landing/PlatformLogo";
import { applicationPath, MARKETING_ONLY } from "../landing/application";
import {
  DOWNLOADS,
  PROJECT_URL,
  HOSTED_APP_URL,
  DESKTOP_PROJECT_URL,
  ANDROID_PROJECT_URL,
  WINDOWS_OS_VERSIONS,
  LINUX_DISTRIBUTIONS,
} from "../landing/downloads";
import {
  HOME,
  FEATURES,
  FAQ,
  AUDIENCES,
  INSTALL_STEPS,
} from "../landing/content";
import "./landing.css";

const navigation = [
  ["Возможности", "/welcome#features"],
  ["Загрузить", "/download"],
  ["Для команд", "/welcome#business"],
  ["На своём сервере", "/self-hosted"],
] as const;

export function LandingBrand({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <Link
      to={MARKETING_ONLY ? "/" : "/welcome"}
      className="lp-brand"
      aria-label="Главная страница MeetSpace"
      onClick={() => {
        window.scrollTo(0, 0);
        onNavigate?.();
      }}
    >
      <img
        src="/branding/meetspace-horizontal-light-no-tagline.svg"
        width="180"
        height="46"
        alt="MeetSpace"
      />
    </Link>
  );
}

export function LandingHeader() {
  const [open, setOpen] = useState(false);
  const menu = useRef<HTMLDialogElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const dialog = menu.current;
    if (open) dialog?.showModal();
    else if (dialog?.open) dialog.close();
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [open]);
  function close() {
    setOpen(false);
    trigger.current?.focus();
  }
  return (
    <header className="lp-header">
      <div className="lp-container lp-header-inner">
        <LandingBrand />
        <nav className="lp-desktop-nav" aria-label="Основная навигация">
          {navigation.map(([label, href]) => (
            <a key={href} href={href}>
              {label}
            </a>
          ))}
          <Link to="/docs">
            Документация <ArrowUpRight size={12} />
          </Link>
        </nav>
        <div className="lp-account">
          <Link className="lp-login" to={applicationPath("/login")}>
            Вход
          </Link>
          <Link
            className="lp-button lp-button-small"
            to={applicationPath("/register")}
          >
            Начать работу <ArrowUpRight size={16} />
          </Link>
        </div>
        <button
          ref={trigger}
          className="lp-menu-button"
          aria-label="Открыть меню"
          aria-expanded={open}
          aria-controls="lp-mobile-menu"
          onClick={() => setOpen(true)}
        >
          <Menu />
        </button>
        <dialog
          ref={menu}
          id="lp-mobile-menu"
          className="lp-mobile-menu"
          aria-label="Меню навигации"
          onCancel={close}
          onClose={() => setOpen(false)}
          onClick={(e) => {
            if (e.target === e.currentTarget) close();
          }}
        >
          <div className="lp-mobile-menu-top">
            <LandingBrand onNavigate={close} />
            <button aria-label="Закрыть меню" onClick={close}>
              <X />
            </button>
          </div>
          <nav aria-label="Мобильная навигация">
            {navigation.map(([label, href]) => (
              <a key={href} href={href} onClick={close}>
                {label}
                <ArrowUpRight size={18} />
              </a>
            ))}
            <Link to="/docs" onClick={close}>
              Документация <ArrowUpRight size={18} />
            </Link>
            <Link to={applicationPath("/login")} onClick={close}>
              Вход <ArrowUpRight size={18} />
            </Link>
            <Link
              className="lp-button"
              to={applicationPath("/register")}
              onClick={close}
            >
              Начать работу <ArrowRight size={18} />
            </Link>
          </nav>
        </dialog>
      </div>
    </header>
  );
}

function DownloadCard({ platform }: { platform: (typeof DOWNLOADS)[number] }) {
  const architectures = [
    ...new Set(platform.assets.map((asset) => asset.architecture)),
  ];
  const [architecture, setArchitecture] = useState(architectures[0] || "");
  const [distribution, setDistribution] = useState<string>(
    LINUX_DISTRIBUTIONS[0].name,
  );
  const linuxDistribution =
    LINUX_DISTRIBUTIONS.find((item) => item.name === distribution) ||
    LINUX_DISTRIBUTIONS[0];
  const selectedFormat =
    platform.id === "linux"
      ? linuxDistribution.format
      : platform.assets[0]?.format;
  const selectedAssets = platform.assets.filter(
    (asset) =>
      asset.architecture === architecture && asset.format === selectedFormat,
  );
  const unverifiedLinuxArm64 =
    platform.id === "linux" &&
    architecture === "ARM64" &&
    !linuxDistribution.arm64Versions;
  const osVersions =
    platform.id === "windows"
      ? WINDOWS_OS_VERSIONS[architecture]
      : platform.id === "linux"
        ? architecture === "ARM64" && linuxDistribution.arm64Versions
          ? linuxDistribution.arm64Versions
          : `${linuxDistribution.versions}${unverifiedLinuxArm64 ? " (x64)" : ""}`
        : platform.supportedSystems.join(", ") || null;
  return (
    <article className={`lp-download-card lp-download-${platform.id}`}>
      <div className="lp-download-card-top">
        <div className="lp-platform-icon">
          {platform.id === "android" ? (
            <PlatformLogo platform="android" size={30} />
          ) : platform.id === "windows" ? (
            <PlatformLogo platform="windows" size={30} />
          ) : (
            <PlatformLogo platform="linux" size={30} />
          )}
        </div>
        <span className="lp-platform-kind">
          {platform.id === "android" ? "В ПУТИ" : "ЗА РАБОЧИМ СТОЛОМ"}
        </span>
      </div>
      <h3>{platform.label}</h3>
      <p className="lp-download-description">
        {platform.id === "android"
          ? "Всегда рядом, куда бы вы ни направлялись."
          : platform.id === "windows"
            ? "Ваше пространство для командной работы."
            : "Ваше рабочее пространство. Ваш любимый дистрибутив."}
      </p>
      <p className="lp-build-note">{platform.buildNote}</p>
      {platform.status === "available" && (
        <div
          className={`lp-download-options${platform.id !== "linux" ? " lp-download-options-single" : ""}`}
        >
          {architectures.length > 1 ? (
            <label
              className="lp-architecture-select"
              htmlFor={`architecture-${platform.id}`}
            >
              Архитектура
              <select
                id={`architecture-${platform.id}`}
                value={architecture}
                onChange={(event) => setArchitecture(event.target.value)}
              >
                {architectures.map((arch) => (
                  <option key={arch} value={arch}>
                    {arch === "x64" ? "x64 (Intel / AMD)" : arch}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <div className="lp-download-single">
              <span>Архитектура</span>
              <strong>
                {architectures[0] === "Universal"
                  ? "Универсальная"
                  : architectures[0]}
              </strong>
            </div>
          )}
          {platform.id === "linux" && (
            <label
              className="lp-architecture-select"
              htmlFor="distribution-linux"
            >
              Дистрибутив
              <select
                id="distribution-linux"
                value={distribution}
                onChange={(event) => setDistribution(event.target.value)}
              >
                {LINUX_DISTRIBUTIONS.map((item) => (
                  <option key={item.name} value={item.name}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          )}
        </div>
      )}
      {osVersions && (
        <div className="lp-os-versions" aria-live="polite">
          <span>Версии ОС</span>
          <strong>{osVersions}</strong>
          {unverifiedLinuxArm64 && (
            <small>
              Совместимость ARM64 с этим дистрибутивом пока не подтверждена.
            </small>
          )}
        </div>
      )}
      {platform.status === "available" && platform.assets.length > 0 ? (
        <div className="lp-download-assets">
          {selectedAssets.map((asset) => (
            <div className="lp-download-asset" key={asset.url}>
              <a
                className="lp-button lp-button-outline"
                href={asset.url}
                target="_blank"
                rel="noopener noreferrer"
              >
                <Download size={16} />
                <span>
                  Скачать{" "}
                  {platform.id === "linux"
                    ? linuxDistribution.name
                    : asset.format}{" "}
                  <span className="lp-asset-arch">
                    ·{" "}
                    {asset.architecture === "Universal"
                      ? "Универсальная"
                      : asset.architecture}
                  </span>
                </span>
                <ArrowUpRight size={15} />
              </a>
              <small>
                v{asset.version}
                {platform.id === "linux" ? ` · ${asset.format}` : ""}
                {asset.sizeBytes !== null
                  ? ` · ${(asset.sizeBytes / 1048576).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} МБ`
                  : ""}
              </small>
              {asset.sha256 && (
                <details>
                  <summary>
                    Проверить SHA-256 <Plus size={12} />
                  </summary>
                  <code>{asset.sha256}</code>
                </details>
              )}
            </div>
          ))}
        </div>
      ) : (
        <div className="lp-unavailable">
          <span>Сборка для настольных компьютеров недоступна</span>
          <small>Используйте веб-приложение на этой платформе.</small>
          <a
            className="lp-text-link"
            href={HOSTED_APP_URL}
            target="_blank"
            rel="noopener noreferrer"
          >
            Открыть в браузере <ArrowUpRight size={15} />
          </a>
        </div>
      )}
      <p className="lp-platform-description">{platform.description}</p>
      {platform.repositoryUrl !== DESKTOP_PROJECT_URL && (
        <a
          className="lp-repository-link"
          href={platform.repositoryUrl}
          target="_blank"
          rel="noopener noreferrer"
        >
          Посмотреть на GitHub <ArrowUpRight size={14} />
        </a>
      )}
    </article>
  );
}

const featureIcons = [
  Video,
  MessageCircle,
  Users,
  CirclePlay,
  CalendarDays,
  FolderOpen,
  FileText,
];
const audienceIcons = [Code2, Building2, GraduationCap, Heart, Server];

function FeatureSection() {
  return (
    <section
      id="features"
      className="lp-section lp-container"
      aria-labelledby="features-title"
    >
      <div className="lp-section-heading lp-centered">
        <p className="lp-kicker">Одно пространство. Больше способов общения.</p>
        <h2 id="features-title">{HOME.featuresTitle}</h2>
        <p>{HOME.featuresDescription}</p>
      </div>
      <div className="lp-feature-grid">
        {FEATURES.map((feature, index) => {
          const Icon = featureIcons[index];
          return (
            <article
              key={feature.id}
              className={`lp-feature-card lp-feature-${feature.id}`}
            >
              <div className="lp-feature-heading">
                <span className="lp-feature-icon" aria-hidden="true">
                  <Icon size={23} />
                </span>
                <h3>{feature.title}</h3>
              </div>
              <p>{feature.description}</p>
              {index === 0 && (
                <div className="lp-feature-pills" aria-hidden="true">
                  <span>
                    <MonitorUp size={14} /> Демонстрация экрана
                  </span>
                  <span>
                    <MessageCircle size={14} /> Чат встречи
                  </span>
                </div>
              )}
            </article>
          );
        })}
      </div>
    </section>
  );
}

function MeetingVisual() {
  return (
    <div
      className="lp-detail-visual lp-meeting-visual"
      role="img"
      aria-label="Современный MacBook на столе в светлом офисе, под углом с видимой клавиатурой и трекпадом. На экране - видеовстреча MeetSpace с четырьмя участниками."
    >
      <img
        className="lp-macbook-image"
        src="/media/meetspace-macbook-office.webp"
        width="1536"
        height="1024"
        alt=""
        loading="lazy"
        decoding="async"
        aria-hidden="true"
      />
      <div className="lp-float-note" aria-hidden="true">
        <span>
          <MonitorUp size={20} />
        </span>
        <div>
          <strong>Представляйте свои идеи</strong>
          <small>Делитесь экраном с командой</small>
        </div>
      </div>
    </div>
  );
}

function ChatVisual() {
  return (
    <div
      className="lp-detail-visual lp-chat-visual"
      role="img"
      aria-label="Наглядный проектный чат с PDF-вложением и ответом."
    >
      <div className="lp-message-card" aria-hidden="true">
        <div className="lp-message-heading">
          <span className="lp-avatar lp-avatar-violet">
            <Users size={21} />
          </span>
          <div>
            <strong>Команда по продукту</strong>
            <small>4 участника</small>
          </div>
          <Video size={20} />
        </div>
        <span className="lp-message-date">Сегодня</span>
        <div className="lp-message-item">
          <span className="lp-avatar">А</span>
          <div>
            <strong>
              Анна <small>10:42</small>
            </strong>
            <p>Вот итоги нашей встречи. Готовы перейти к следующему шагу?</p>
            <div className="lp-file-preview">
              <FileText size={24} />
              <div>
                <b>Бриф проекта.pdf</b>
                <small>PDF-документ · 2,4 МБ</small>
              </div>
              <ArrowDown size={17} />
            </div>
          </div>
        </div>
        <div className="lp-message-bubble">
          Выглядит отлично! Давайте воплотим это в жизнь.
          <small>
            10:43 <Check size={12} />
          </small>
        </div>
        <div className="lp-message-compose">
          <Plus size={18} />
          <span>Отправьте сообщение своей команде…</span>
          <Send size={18} />
        </div>
      </div>
    </div>
  );
}

function FolderVisual() {
  return (
    <div
      className="lp-detail-visual lp-folders-visual"
      role="img"
      aria-label="Наглядная личная папка, в которой можно организовать совещание, групповой чат и личное сообщение."
    >
      <div className="lp-folder-board" aria-hidden="true">
        <div className="lp-folder-rail">
          <strong>Ваши папки</strong>
          <span className="active">
            <FolderOpen size={17} /> Запуск продукта
          </span>
          <span>
            <FolderOpen size={17} /> Команда дизайнеров
          </span>
          <span>
            <FolderOpen size={17} /> Каждый день
          </span>
          <span className="lp-new-folder">
            <Plus size={15} /> Новая папка
          </span>
        </div>
        <div className="lp-folder-content">
          <span className="lp-folder-large">
            <FolderOpen size={30} />
          </span>
          <h3>Запуск продукта</h3>
          <p>Все на своих местах.</p>
          {[
            { Icon: Video, name: "Еженедельная встреча", kind: "Встреча" },
            { Icon: Users, name: "Команда запуска", kind: "Групповой чат" },
            {
              Icon: MessageCircle,
              name: "Анна Смирнова",
              kind: "Личное сообщение",
            },
          ].map(({ Icon, name, kind }) => (
            <div className="lp-folder-row" key={name}>
              <span>
                <Icon size={19} />
              </span>
              <div>
                <strong>{name}</strong>
                <small>{kind}</small>
              </div>
              <ArrowUpRight size={15} />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function ProductDetails() {
  return (
    <div className="lp-product-details lp-container">
      <section className="lp-detail-row" aria-labelledby="video-title">
        <div className="lp-detail-copy">
          <p className="lp-kicker">
            <Video size={16} /> Видеоконференции
          </p>
          <h2 id="video-title">{HOME.videoTitle}</h2>
          <p>{HOME.videoDescription}</p>
          <ul className="lp-checks">
            <li>
              <Check size={16} /> Покажите свою работу с помощью демонстрации
              экрана
            </li>
            <li>
              <Check size={16} /> Храните сообщения вместе с материалами встречи
            </li>
            <li>
              <Check size={16} /> Управляйте участниками встречи
            </li>
          </ul>
          <Link className="lp-text-link" to={applicationPath("/meetings/new")}>
            Создать встречу <ArrowRight size={18} />
          </Link>
        </div>
        <MeetingVisual />
      </section>
      <section
        className="lp-detail-row lp-detail-reverse"
        aria-labelledby="messages-title"
      >
        <div className="lp-detail-copy">
          <p className="lp-kicker">
            <MessageCircle size={16} /> Сообщения и файлы
          </p>
          <h2 id="messages-title">{HOME.messengerTitle}</h2>
          <p>{HOME.messengerDescription}</p>
          <Link className="lp-text-link" to={applicationPath("/personal")}>
            Открыть чаты <ArrowRight size={18} />
          </Link>
        </div>
        <ChatVisual />
      </section>
      <section className="lp-detail-row" aria-labelledby="folders-title">
        <div className="lp-detail-copy">
          <p className="lp-kicker">
            <FolderOpen size={16} /> Личные папки
          </p>
          <h2 id="folders-title">{HOME.foldersTitle}</h2>
          <p>{HOME.foldersDescription}</p>
          <div className="lp-topic-chips">
            <span>По проектам</span>
            <span>По командам</span>
            <span>По темам</span>
          </div>
        </div>
        <FolderVisual />
      </section>
    </div>
  );
}

export function SelfHostedSection({
  standalone = false,
}: {
  standalone?: boolean;
}) {
  const Heading = standalone ? "h1" : "h2";
  return (
    <section
      id="self-hosted"
      className={`lp-self-wrap ${standalone ? "lp-self-standalone" : ""}`}
      aria-labelledby="self-title"
    >
      <div className="lp-container lp-self-grid">
        <div className="lp-self-copy">
          <p className="lp-kicker">
            <Server size={16} /> Создано для вашей инфраструктуры
          </p>
          <Heading id="self-title">{HOME.selfHostedTitle}</Heading>
          <p>{HOME.selfHostedDescription}</p>
          <ul className="lp-checks">
            <li>
              <Check size={16} /> Ваш домен и развертывание
            </li>
            <li>
              <Check size={16} /> Ваши политики хранения и резервного
              копирования
            </li>
            <li>
              <Check size={16} /> Ваша конфигурация и обновления
            </li>
          </ul>
          <div className="lp-actions">
            <Link className="lp-button lp-button-white" to="/docs/installation">
              Установка на ваш сервер <ArrowRight size={17} />
            </Link>
            <Link
              className="lp-text-link"
              to={standalone ? "/docs/installation" : "/self-hosted"}
            >
              {standalone
                ? "Руководство по установке"
                : "Подробнее о своём сервере"}{" "}
              <ArrowUpRight size={16} />
            </Link>
          </div>
        </div>
        <div
          className="lp-architecture"
          role="img"
          aria-label="Веб-клиенты, настольные и мобильные клиенты подключаются к вашему серверу MeetSpace по протоколам HTTPS, WSS и WebRTC. Вы управляете его базой данных и хранилищем файлов."
        >
          <div className="lp-architecture-top">
            <span>ВАШЕ ПРОСТРАНСТВО</span>
            <ShieldCheck size={18} />
          </div>
          <div className="lp-clients">
            <span>
              <Globe2 size={23} />
              Браузер
            </span>
            <span>
              <Monitor size={23} />
              Компьютер
            </span>
            <span>
              <Smartphone size={23} />
              Мобильный
            </span>
          </div>
          <div className="lp-connection">
            <i />
            <span>HTTPS · WSS · WebRTC</span>
            <ArrowDown size={17} />
          </div>
          <div className="lp-server-box">
            <img
              src="/branding/meetspace-symbol-color.svg"
              width="38"
              height="40"
              alt=""
            />
            <div>
              <strong>MeetSpace</strong>
              <span>meet.your-company.com</span>
            </div>
            <Server size={25} />
          </div>
          <div className="lp-storage">
            <span>
              <Database size={17} /> База данных
            </span>
            <span>
              <FolderOpen size={17} /> Хранилище файлов
            </span>
          </div>
          <p>Вы сами выбираете, где все это будет храниться.</p>
        </div>
      </div>
    </section>
  );
}

export function InstallSection() {
  return (
    <section
      className="lp-section lp-install lp-container"
      aria-labelledby="install-title"
    >
      <div className="lp-section-heading">
        <p className="lp-kicker">От вашего сервера к вашей команде</p>
        <h2 id="install-title">{HOME.installTitle}</h2>
      </div>
      <div className="lp-install-grid">
        {INSTALL_STEPS.map((step, index) => (
          <article key={step.title}>
            <span className="lp-step-number">0{index + 1}</span>
            <h3>{step.title}</h3>
            <p>{step.description}</p>
          </article>
        ))}
      </div>
      <Link className="lp-text-link" to="/docs/installation">
        Следуйте инструкциям по установке <ArrowRight size={17} />
      </Link>
    </section>
  );
}

function InstallationHelp() {
  return (
    <section className="lp-install-help" aria-labelledby="client-install-title">
      <div className="lp-section-heading">
        <p className="lp-kicker">С чего начать</p>
        <h2 id="client-install-title">Установите. Подключитесь. Общайтесь.</h2>
        <p>
          Выберите версию для своего устройства, затем подключитесь к серверу
          MeetSpace.
        </p>
      </div>
      <div className="lp-client-help-grid">
        <article>
          <PlatformLogo platform="android" size={25} />
          <h3>Android</h3>
          <p>
            Загрузите APK на устройство с Android 8.0 или более поздней версией.
            Откройте файл и следуйте инструкциям по установке Android. Запустите
            MeetSpace и введите HTTPS-адрес своего сервера.
          </p>
        </article>
        <article>
          <PlatformLogo platform="windows" size={25} />
          <h3>Windows</h3>
          <p>
            Выберите x64 или ARM64 в соответствии с архитектурой вашего
            компьютера, затем откройте EXE-файл установщика. Сборки
            предназначены для Windows 10 22H2 x64 и Windows 11 x64 или ARM64.
            Эти бета-версии установщиков не подписаны.
          </p>
        </article>
        <article>
          <PlatformLogo platform="linux" size={25} />
          <h3>Linux</h3>
          <p>
            Выберите Ubuntu, Debian или Fedora и архитектуру x64 (Intel / AMD)
            или ARM64. Для Ubuntu и Debian предлагается DEB, для Fedora - RPM.
            Нужен графический рабочий стол с разблокированным хранилищем ключей
            GNOME Keyring или KWallet. Для демонстрации экрана в Wayland
            требуются PipeWire и XDG Desktop Portal.
          </p>
        </article>
      </div>
      <div className="lp-download-faq">
        <h3>Перед первым разговором</h3>
        <details>
          <summary>
            Какую архитектуру выбрать?
            <Plus size={18} />
          </summary>
          <p>
            Для большинства компьютеров Intel и AMD выбирайте x64. Выберите
            ARM64 для компьютера с операционной системой на базе ARM. Если вы не
            уверены, проверьте системную информацию своего устройства. APK-файл
            для Android включает в себя несколько архитектур.
          </p>
        </details>
        <details>
          <summary>
            Как подключиться к собственному серверу?
            <Plus size={18} />
          </summary>
          <p>
            Используйте HTTPS-адрес, предоставленный вашим администратором.
            Следуйте инструкциям по подключению к серверу, а затем войдите в
            систему или зарегистрируйтесь, если ваш сервер это позволяет.
            Веб-приложение доступно по адресу вашего сервера.
          </p>
        </details>
        <details>
          <summary>
            Что значит «бета-версия»?
            <Plus size={18} />
          </summary>
          <p>
            Эти пакеты проходят тестирование. Перед установкой ознакомьтесь с
            примечаниями к репозиторию, в которых указаны требования и известные
            проблемы. Пакеты для настольных компьютеров - это тестовые сборки, а
            установщики для Windows не подписаны. Наличие пакета не гарантирует
            его совместимость со всеми системами.
          </p>
        </details>
        <details>
          <summary>
            Как обновить приложение или сообщить о проблеме?
            <Plus size={18} />
          </summary>
          <p>
            В репозитории Android или десктопных приложений можно найти более
            новую версию пакета и примечания по установке. Чтобы сообщить о
            проблеме, укажите версию приложения и операционную систему в
            описании проблемы на GitHub. Не публикуйте пароли, ссылки для
            приглашения и личные сообщения.
          </p>
        </details>
      </div>
    </section>
  );
}

export function DownloadSection({
  standalone = false,
}: {
  standalone?: boolean;
}) {
  const Heading = standalone ? "h1" : "h2";
  return (
    <section
      id="apps"
      className={`lp-section lp-download-section ${standalone ? "lp-download-standalone" : ""}`}
      aria-labelledby="apps-title"
    >
      <div className="lp-container">
        <div className="lp-section-heading lp-centered">
          <p className="lp-kicker">
            <Download size={16} /> Приложения MeetSpace
          </p>
          <Heading id="apps-title">{HOME.downloadsTitle}</Heading>
          <p>{HOME.downloadsDescription}</p>
        </div>
        <p className="lp-beta-note">
          Бета-версии · Приложения для компьютера - тестовые сборки. Установщики
          Windows не подписаны.
        </p>
        <div className="lp-download-grid">
          {[...DOWNLOADS]
            .sort(
              (a, b) =>
                ["windows", "android", "linux"].indexOf(a.id) -
                ["windows", "android", "linux"].indexOf(b.id),
            )
            .map((platform) => (
              <DownloadCard platform={platform} key={platform.id} />
            ))}
        </div>
        <div className="lp-browser-banner">
          <span className="lp-browser-icon">
            <Globe2 size={29} />
          </span>
          <div>
            <h3>MeetSpace уже в вашем браузере.</h3>
            <p>
              Установка не требуется. Откройте веб-приложение и подключитесь.
            </p>
          </div>
          <a
            className="lp-button lp-button-outline"
            href={HOSTED_APP_URL}
            target="_blank"
            rel="noopener noreferrer"
          >
            Откройте веб-приложение <ArrowUpRight size={17} />
          </a>
        </div>
        {standalone ? (
          <InstallationHelp />
        ) : (
          <Link className="lp-text-link lp-download-help-link" to="/download">
            Все приложения и помощь с установкой <ArrowRight size={17} />
          </Link>
        )}
      </div>
    </section>
  );
}

function AudienceSection() {
  return (
    <section
      id="business"
      className="lp-section lp-container"
      aria-labelledby="audiences-title"
    >
      <div className="lp-section-heading lp-centered">
        <p className="lp-kicker">Пространство для ваших людей</p>
        <h2 id="audiences-title">{HOME.audienceTitle}</h2>
      </div>
      <div className="lp-audience-grid">
        {AUDIENCES.map((audience, index) => {
          const Icon = audienceIcons[index];
          return (
            <article key={audience.title}>
              <Icon size={24} />
              <h3>{audience.title}</h3>
              <p>{audience.description}</p>
            </article>
          );
        })}
      </div>
    </section>
  );
}

function FaqSection() {
  return (
    <section
      id="faq"
      className="lp-section lp-container lp-faq-section"
      aria-labelledby="faq-title"
    >
      <div className="lp-section-heading">
        <p className="lp-kicker">Хорошие вопросы. Четкие ответы.</p>
        <h2 id="faq-title">
          Вопросы,
          <br />
          которые могут возникнуть.
        </h2>
        <p>Начало работы, выбор приложения или запуск собственного сервера.</p>
        <Link className="lp-text-link" to="/docs/installation">
          Изучите документацию <ArrowUpRight size={17} />
        </Link>
      </div>
      <div className="lp-faq-list">
        {FAQ.map(([question, answer], index) => (
          <details key={question}>
            <summary>
              <span className="lp-faq-number">
                {String(index + 1).padStart(2, "0")}
              </span>
              <span>{question}</span>
              <Plus size={18} />
            </summary>
            <p>{answer}</p>
          </details>
        ))}
      </div>
    </section>
  );
}

function FinalCta() {
  return (
    <section className="lp-cta-wrap">
      <div className="lp-container lp-final-cta">
        <div className="lp-cta-symbol" aria-hidden="true">
          <img
            src="/branding/meetspace-symbol-color.svg"
            width="52"
            height="56"
            alt=""
          />
        </div>
        <h2>{HOME.ctaTitle}</h2>
        <p>{HOME.ctaDescription}</p>
        <div className="lp-actions">
          <Link className="lp-button" to={applicationPath("/register")}>
            Начать работу <ArrowRight size={18} />
          </Link>
          <Link className="lp-button lp-button-outline" to="/download">
            <Download size={17} /> Скачать MeetSpace
          </Link>
        </div>
      </div>
    </section>
  );
}

export function LandingFooter() {
  return (
    <footer className="lp-footer">
      <div className="lp-container">
        <div className="lp-footer-main">
          <div className="lp-footer-brand">
            <LandingBrand />
            <p>
              <strong>MeetSpace</strong> - {HOME.title}. {HOME.description}
            </p>
          </div>
          <div>
            <h3>Продукт</h3>
            <a href="/welcome#features">Возможности</a>
            <Link to="/download">Скачать приложения</Link>
            <a href="/welcome#faq">Вопросы и ответы</a>
          </div>
          <div>
            <h3>Ваша инфраструктура</h3>
            <Link to="/self-hosted">На своём сервере</Link>
            <Link to="/docs/installation">Руководство по установке</Link>
            <a href={PROJECT_URL} target="_blank" rel="noopener noreferrer">
              Исходный код <ArrowUpRight size={12} />
            </a>
          </div>
          <div>
            <h3>Получить приложения</h3>
            <a
              href={DESKTOP_PROJECT_URL}
              target="_blank"
              rel="noopener noreferrer"
            >
              Приложения для компьютера на GitHub <ArrowUpRight size={12} />
            </a>
            <a
              href={ANDROID_PROJECT_URL}
              target="_blank"
              rel="noopener noreferrer"
            >
              Android на GitHub <ArrowUpRight size={12} />
            </a>
            <a href={HOSTED_APP_URL} target="_blank" rel="noopener noreferrer">
              Открыть веб-приложение <ArrowUpRight size={12} />
            </a>
          </div>
        </div>
        <div className="lp-footer-bottom">
          <span>
            © {new Date().getFullYear()} MeetSpace · Яницкий Александр
          </span>
          <a href="#main">
            Наверх <ArrowUpRight size={13} />
          </a>
        </div>
        <p className="lp-platform-credits">
          Android robot - Google, используется по лицензии{" "}
          <a
            href="https://creativecommons.org/licenses/by/3.0/"
            target="_blank"
            rel="noopener noreferrer"
          >
            CC BY 3.0
          </a>
          . Android - товарный знак Google LLC. Tux -{" "}
          <a
            href="https://commons.wikimedia.org/wiki/File:Tux.png"
            target="_blank"
            rel="noopener noreferrer"
          >
            Larry Ewing и The GIMP
          </a>
          .
        </p>
      </div>
    </footer>
  );
}

export function Landing() {
  return (
    <div className="lp-site">
      <a className="lp-skip" href="#main">
        Перейти к содержимому
      </a>
      <LandingHeader />
      <main id="main" tabIndex={-1}>
        <section className="lp-hero" aria-labelledby="hero-title">
          <div className="lp-container">
            <div className="lp-hero-copy">
              <p className="lp-hero-eyebrow">
                <span />
                MeetSpace · Встречи. Общение. Работа.
              </p>
              <h1 id="hero-title">
                <span className="lp-hero-name">
                  Meet<span>Space</span>
                </span>{" "}
                - <span className="lp-hero-title-copy">{HOME.title}</span>
              </h1>
              <p className="lp-hero-description">{HOME.description}</p>
              <div className="lp-hero-actions">
                <Link className="lp-button" to={applicationPath("/register")}>
                  Начать работу <ArrowRight size={18} />
                </Link>
                <Link className="lp-button lp-button-outline" to="/download">
                  <Download size={17} /> Скачать MeetSpace
                </Link>
              </div>
              <div className="lp-hero-platforms">
                <span>
                  <Globe2 size={15} /> Веб
                </span>
                <span>
                  <PlatformLogo platform="android" size={15} /> Android
                </span>
                <span>
                  <PlatformLogo platform="windows" size={15} /> Windows
                </span>
                <span>
                  <PlatformLogo platform="linux" size={15} /> Linux
                </span>
              </div>
            </div>
            <div className="lp-hero-product">
              <LandingScene />
              <div className="lp-product-note">
                <span>
                  <Video size={17} /> Лицом к лицу - где бы вы ни были.
                </span>
                <span>
                  <MessageCircle size={17} /> Продолжайте общение.
                </span>
                <span>
                  <Server size={17} /> Пространство, которым управляете вы.
                </span>
              </div>
            </div>
          </div>
        </section>
        <FeatureSection />
        <ProductDetails />
        <SelfHostedSection />
        <InstallSection />
        <DownloadSection />
        <AudienceSection />
        <FaqSection />
        <FinalCta />
      </main>
      <LandingFooter />
    </div>
  );
}
