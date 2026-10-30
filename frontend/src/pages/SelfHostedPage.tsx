import { useState } from "react";
import { Link } from "react-router";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  ChevronRight,
  Copy,
  ExternalLink,
  Server,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import { BRAND_ASSET_VERSION } from "../brand";
import "./self-hosted.css";

const sourceCommit = "893566bb90f07bdcec4542eec5e00116c423e852";
const sourceRoot = `https://github.com/janickiy/meet-space/blob/${sourceCommit}`;
const sections = [
  ["before-start", "Перед началом работы"],
  ["requirements", "Сервер и инструменты"],
  ["network", "Домен, TLS и порты"],
  ["source", "Исходный код и релизы"],
  ["configuration", "Настройка"],
  ["deployment", "Развертывание и миграция"],
  ["verification", "Проверка и доступ"],
  ["maintenance", "Обновления и резервное копирование"],
  ["troubleshooting", "Устранение неполадок"],
] as const;

function SourceLink({
  path,
  children,
}: {
  path: string;
  children: React.ReactNode;
}) {
  return (
    <a href={`${sourceRoot}/${path}`} target="_blank" rel="noopener noreferrer">
      {children} <ExternalLink size={13} aria-hidden="true" />
    </a>
  );
}

function Command({ title, children }: { title: string; children: string }) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  async function copy() {
    try {
      await navigator.clipboard.writeText(children);
      setStatus("copied");
    } catch {
      setStatus("failed");
    }
  }
  return (
    <div className="self-code">
      <div className="self-code-header">
        <span>
          <Terminal size={14} aria-hidden="true" />
          {title}
        </span>
        <button
          type="button"
          onClick={copy}
          aria-label={`Скопировать: ${title}`}
        >
          {status === "copied" ? (
            <Check size={15} aria-hidden="true" />
          ) : (
            <Copy size={15} aria-hidden="true" />
          )}
          {status === "copied" ? "Скопировано" : "Скопировать"}
        </button>
      </div>
      <pre tabIndex={0} aria-label={title}>
        <code>{children}</code>
      </pre>
      <span
        className={status === "failed" ? "self-copy-error" : "sr-only"}
        role="status"
      >
        {status === "copied"
          ? "Команды скопированы в буфер обмена."
          : status === "failed"
            ? "Буфер обмена недоступен. Выделите и скопируйте команды вручную."
            : ""}
      </span>
    </div>
  );
}

export function SelfHostedPage() {
  return (
    <div className="self-page">
      <a className="self-skip" href="#self-main">
        Перейти к руководству
      </a>
      <header className="self-header self-container">
        <Link
          className="brand brand-meetspace"
          to="/"
          aria-label="MeetSpace — главная страница"
        >
          <span className="brand-lockup">
            <img
              className="brand-logo-light"
              src={`/branding/meetspace-horizontal-light-no-tagline.svg?v=${BRAND_ASSET_VERSION}`}
              width="900"
              height="230"
              alt="MeetSpace"
            />
          </span>
        </Link>
        <nav aria-label="Навигация по документации">
          <Link className="self-back" to="/">
            <ArrowLeft size={16} aria-hidden="true" /> Вернуться на главную
            страницу
          </Link>
          <Link className="self-app-link" to="/login">
            Открыть приложение <ArrowRight size={16} aria-hidden="true" />
          </Link>
        </nav>
      </header>

      <div className="self-intro">
        <div className="self-container">
          <div className="self-breadcrumb">
            <Link to="/">MeetSpace</Link>
            <ChevronRight size={14} aria-hidden="true" />
            <span>Документация</span>
          </div>
          <div className="self-eyebrow">
            <Server size={16} aria-hidden="true" /> НА СВОЁМ СЕРВЕРЕ
          </div>
          <h1>
            Ваше пространство.
            <br />
            <span>На вашем сервере.</span>
          </h1>
          <p className="self-lead">
            Настройте, разверните и поддерживайте MeetSpace на собственном
            VDS/VPS-сервере. Подробные инструкции со ссылками на исходные
            материалы.
          </p>
          <div className="self-meta">
            <span>
              <ShieldCheck size={15} aria-hidden="true" /> Руководство с
              исходным кодом
            </span>
            <span>
              Снимок экрана: <code>{sourceCommit.slice(0, 12)}</code>
            </span>
            <span>18 октября 2026 г.</span>
          </div>
        </div>
      </div>

      <div className="self-container self-layout">
        <aside className="self-sidebar">
          <nav aria-label="Содержание">
            <p>В ЭТОМ РУКОВОДСТВЕ</p>
            <ol>
              {sections.map(([id, title], index) => (
                <li key={id}>
                  <a href={`#${id}`}>
                    <span>{String(index + 1).padStart(2, "0")}</span>
                    {title}
                  </a>
                </li>
              ))}
            </ol>
          </nav>
          <div className="self-sidebar-note">
            <Server size={22} aria-hidden="true" />
            <strong>Один сервер. Ваша инфраструктура.</strong>
            <p>
              Docker Compose, Go и Pion WebRTC. Данные хранятся в вашем
              хранилище.
            </p>
            <SourceLink path="docs/ARCHITECTURE.md">
              Архитектура проекта
            </SourceLink>
          </div>
        </aside>

        <main id="self-main" className="self-content" tabIndex={-1}>
          <section id="before-start" aria-labelledby="before-start-title">
            <div className="self-section-label">01 / ПЕРЕД НАЧАЛОМ РАБОТЫ</div>
            <h2 id="before-start-title">Что нужно знать перед установкой</h2>
            <p>
              MeetSpace работает на одном хосте Docker Compose. Вы управляете
              доменом, сертификатами, доступом и резервными копиями. Обновления
              могут прерывать сеансы связи: высокая доступность и динамическая
              миграция комнат между серверами не реализованы.
            </p>
            <div className="self-notice">
              <strong>
                Эта процедура развертывания не была протестирована на удаленном
                хосте
              </strong>
              <p>
                Руководство сверено с исходным кодом указанного коммита. В
                репозитории описано пилотное развёртывание, но оно не
                подтверждает работоспособность этого снимка при вашей нагрузке и
                настройках сети. Перед запуском в эксплуатацию проверьте связь и
                восстановление данных в отдельной тестовой среде.
              </p>
            </div>
            <p>
              <SourceLink path="docs/operations/meeting-host-deployment.md">
                История пилотного развертывания
              </SourceLink>
              <span className="self-link-separator"> · </span>
              <SourceLink path="docs/PRODUCTION_LAUNCH_CHECKLIST.md">
                Проверка готовности к запуску в эксплуатацию
              </SourceLink>
            </p>
          </section>

          <section id="requirements" aria-labelledby="requirements-title">
            <div className="self-section-label">02 / СЕРВЕР</div>
            <h2 id="requirements-title">Подготовка хоста на базе Linux</h2>
            <p>
              В документации проекта описан ограниченный пилотный проект на базе
              Ubuntu 22.04 amd64. Общие минимальные требования к процессору,
              оперативной памяти и дисковому пространству для запуска в
              эксплуатацию не установлены. Ограничения по памяти и
              вычислительной мощности в Compose применяются к контейнерам, а не
              к хосту. Измеряйте объем записываемых данных, количество
              участников и рабочую нагрузку отдельно.
            </p>
            <div className="self-mini-grid">
              <div>
                <strong>Система</strong>
                <p>
                  Linux, совместимая версия Docker Engine и архитектура,
                  поддерживаемая всеми образами в выбранном вами релизе.
                </p>
              </div>
              <div>
                <strong>Инструменты</strong>
                <p>
                  Docker Compose с <code>!override</code>, Bash, Git, jq,
                  OpenSSL, ripgrep и SHA-256. Для сборки используйте версию Go
                  из <code>go.mod</code>.
                </p>
              </div>
            </div>
            <h3>Подключение по SSH</h3>
            <p>
              Замените имя пользователя и IP-адрес. При первом подключении
              сверьте отпечаток ключа хоста с данными из надежного источника.
              Эти команды предназначены для выполнения вами, на этой странице
              они не выполняются.
            </p>
            <Command title="Подключитесь к серверу">{`ssh username@SERVER_IP\n\n# Либо используйте отдельный ключ\nssh -i ~/.ssh/meetspace_ed25519 username@SERVER_IP`}</Command>
            <h3>Установите Docker для своей операционной системы</h3>
            <p>
              Следуйте{" "}
              <a
                href="https://docs.docker.com/engine/install/"
                target="_blank"
                rel="noopener noreferrer"
              >
                официальному руководству по установке Docker Engine{" "}
                <ExternalLink size={13} aria-hidden="true" />
              </a>{" "}
              и установите плагин Compose. Поддержка Docker в дистрибутиве не
              означает, что MeetSpace протестирован на нем.{" "}
              <a
                href="https://docs.docker.com/reference/compose-file/merge/#replace-value"
                target="_blank"
                rel="noopener noreferrer"
              >
                тег <code>!override</code>
              </a>{" "}
              требует Compose 2.24.4 или более поздней версии.
            </p>
            <Command title="Проверьте свои инструменты">{`docker version\ndocker compose version\nbash --version\njq --version\nopenssl version\nrg --version\nsha256sum --version`}</Command>
          </section>

          <section id="network" aria-labelledby="network-title">
            <div className="self-section-label">03 / СЕТЬ</div>
            <h2 id="network-title">Домен, HTTPS и медиасоединения</h2>
            <p>
              Укажите свой домен, например <code>meet.example.com</code>, на
              публичный IP-адрес сервера. Подготовьте доверенные сертификаты для
              HTTPS и TURN TLS и настройте их продление. Compose не выпускает
              TLS-сертификаты автоматически; в этом руководстве не
              рассматривается получение публичных сертификатов для IP-адреса без
              домена.
            </p>
            <div className="self-table-wrap">
              <table>
                <caption>
                  Входящие порты по умолчанию для классической производственной
                  схемы
                </caption>
                <thead>
                  <tr>
                    <th scope="col">Порт</th>
                    <th scope="col">Назначение</th>
                    <th scope="col">Доступ</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>443 / TCP</td>
                    <td>HTTPS и WSS</td>
                    <td>Пользователи</td>
                  </tr>
                  <tr>
                    <td>50010 / UDP и TCP</td>
                    <td>SFU / WebRTC</td>
                    <td>Пользователи</td>
                  </tr>
                  <tr>
                    <td>3478 / UDP и TCP</td>
                    <td>TURN</td>
                    <td>Пользователи</td>
                  </tr>
                  <tr>
                    <td>5349 / TCP</td>
                    <td>TURNS</td>
                    <td>Пользователи</td>
                  </tr>
                  <tr>
                    <td>49160–49259 / UDP</td>
                    <td>TURN-ретранслятор</td>
                    <td>Пользователи</td>
                  </tr>
                  <tr>
                    <td>Ваш SSH-порт</td>
                    <td>Администрирование</td>
                    <td>Ограниченные IP-адреса</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p>
              Согласуйте настройки брандмауэра и NAT с{" "}
              <code>MEDIA_NAT_IPS</code>, <code>MEDIA_UDP_PORT</code>,{" "}
              <code>MEDIA_TCP_PORT</code>, <code>TURN_PUBLIC_IP</code> и
              диапазоном <code>TURN_MIN_PORT</code>–<code>TURN_MAX_PORT</code>.
              Порт 80 нужен только при получении сертификата через HTTP
              challenge; production-конфигурация Compose его не открывает.
            </p>
            <div className="self-note">
              <strong>Обеспечьте конфиденциальность внутренних сервисов</strong>
              <p>
                Не открывайте из интернета доступ к PostgreSQL, Redis, RabbitMQ,
                консоли S3, внутренним HTTP API и метрикам. Используйте
                production-конфигурацию Compose: базовый{" "}
                <code>docker-compose.yml</code> предназначен для разработки и
                открывает дополнительные порты.
              </p>
            </div>
            <SourceLink path="docker-compose.production.yml">
              Проверьте порты и сервисы
            </SourceLink>
          </section>

          <section id="source" aria-labelledby="source-title">
            <div className="self-section-label">04 / ВЫПУСК</div>
            <h2 id="source-title">Закрепите исходные и релизные артефакты</h2>
            <p>
              Все ссылки на исходные коды на этой странице ведут на один и тот
              же коммит. Это делает руководство воспроизводимым, но не
              гарантирует, что релиз готов к запуску в продакшене.
            </p>
            <Command title="Получите снимок исходного кода">{`git clone https://github.com/janickiy/meet-space.git\ncd meet-space\ngit checkout --detach ${sourceCommit}\ngit rev-parse HEAD`}</Command>
            <p>
              Для развертывания требуется подготовленный{" "}
              <code>release.json</code> и соответствующие ему образы. Скрипты{" "}
              <code>scripts/release/build.sh</code> и{" "}
              <code>scripts/release/package.sh</code> связывают версию, коммит,
              хеш исходного кода, SBOM и результаты сканирования. Для production
              принимаются проверенные digest образов из реестра или архив с
              неизменяемыми идентификаторами образов. Режим <code>local</code>{" "}
              не подходит для production.
            </p>
            <div className="self-notice">
              <strong>Клонирование репозитория не подготавливает релиз</strong>
              <p>
                Сначала соберите и проверьте пакет, используя руководство по
                выпуску, с выбранной вами компоновкой и архитектурой. Не
                создавайте манифест вручную и не заменяйте digest на тег{" "}
                <code>latest</code>. Готовый к развертыванию серверный пакет для
                этого коммита не был собран в рамках данного руководства.
              </p>
            </div>
            <SourceLink path="docs/RELEASE_PROCESS.md">
              Сборка и упаковка релиза
            </SourceLink>
            <h3>Выберите согласованную конфигурацию</h3>
            <div className="self-mini-grid">
              <div>
                <strong>классическая</strong>
                <p>
                  <code>production</code> + <code>grafana</code> Compose. MinIO,
                  Nginx с TLS на порту 443, отдельные сертификаты и Grafana для
                  мониторинга.
                </p>
              </div>
              <div>
                <strong>с усиленной защитой общего хоста</strong>
                <p>
                  <code>production</code> + <code>hardened</code> +{" "}
                  <code>shared-host</code>. SeaweedFS, внешний TLS-прокси,
                  локальный порт 19080 и Coturn в сети хоста. Grafana не
                  включена.
                </p>
              </div>
            </div>
            <p>
              Конфигурацию определяет проверенный манифест. Вариант SeaweedFS
              сохраняет имя сервиса <code>minio</code> и переменные{" "}
              <code>MINIO_*</code>, но использует новый том <code>objects</code>
              . Существующий том MinIO несовместим с SeaweedFS. Для shared-host
              настройте внешний прокси и убедитесь, что подсеть{" "}
              <code>172.31.242.0/24</code> не конфликтует с вашей сетью.
            </p>
          </section>

          <section id="configuration" aria-labelledby="configuration-title">
            <div className="self-section-label">05 / КОНФИГУРАЦИЯ</div>
            <h2 id="configuration-title">Настройка окружения и секретов</h2>
            <p>
              Создайте отдельный защищённый файл окружения вне Git. Пути ниже
              приведены для новой тестовой среды; перед запуском выберите свой
              каталог. Шаблон намеренно содержит пустые секреты и
              домены-заглушки.
            </p>
            <Command title="Создайте новый файл конфигурации">{`umask 077\nmkdir -p /srv/meet/config\n# Только для нового файла: не перезаписывайте существующий файл окружения.\ncp -n .env.production.example /srv/meet/config/staging.env\nchmod 600 /srv/meet/config/staging.env`}</Command>
            <div className="self-table-wrap">
              <table>
                <caption>Ключевые группы переменных окружения</caption>
                <thead>
                  <tr>
                    <th scope="col">Переменные</th>
                    <th scope="col">Что нужно настроить</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <code>APP_ENV</code>, <code>AUTO_MIGRATE</code>
                    </td>
                    <td>
                      <code>production</code> и <code>false</code> для тестовой
                      и рабочей среды. Запускайте миграции отдельно.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <code>JWT_SECRET</code>, <code>MEDIA_TICKET_SECRET</code>,{" "}
                      <code>MEDIA_INTERNAL_SECRET</code>,{" "}
                      <code>METRICS_SECRET</code>,{" "}
                      <code>TURN_SHARED_SECRET</code>
                    </td>
                    <td>
                      Независимые случайные значения. Не используйте один и тот
                      же секрет для нескольких полей и не публикуйте секреты.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <code>POSTGRES_*</code>, <code>REDIS_PASSWORD</code>,{" "}
                      <code>RABBIT_MQ_*</code>, <code>MINIO_*</code>
                    </td>
                    <td>
                      Учетные данные для ваших зависимостей. Внутренние имена
                      хостов: <code>postgres</code>, <code>redis</code>,{" "}
                      <code>rabbitmq</code>, <code>minio</code>.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <code>PUBLIC_FRONTEND_URL</code>,{" "}
                      <code>MINIO_PUBLIC_ENDPOINT</code>,{" "}
                      <code>WS_ALLOWED_ORIGINS</code>,{" "}
                      <code>HTTP_TRUSTED_PROXIES</code>
                    </td>
                    <td>
                      Только реальные HTTPS-origin и адреса доверенных прокси.
                      Публичный адрес S3 должен соответствовать маршрутизации
                      выбранной конфигурации.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <code>MEDIA_NAT_IPS</code>, <code>TURN_PUBLIC_IP</code>,{" "}
                      <code>TURN_REALM</code>, <code>TURN_URLS</code>
                    </td>
                    <td>
                      Публичные адреса, область TURN и URL-адреса с
                      соответствующими портами и настройками TLS.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <code>TLS_CERT_DIR</code>, <code>TURN_TLS_CERT_DIR</code>,{" "}
                      <code>RECORDER_STORAGE_DIR</code>
                    </td>
                    <td>
                      Абсолютные пути. Сертификаты: <code>fullchain.pem</code> и{" "}
                      <code>privkey.pem</code>. Каталог для записи: UID/GID{" "}
                      <code>1000:1000</code>, права доступа <code>0750</code>.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p>
              Сгенерируйте один случайный секретный ключ с помощью{" "}
              <code>openssl rand -hex 32</code>; повторите для каждого поля и
              надежно сохраните каждое значение. Все параметры и требования см.
              в{" "}
              <SourceLink path=".env.production.example">
                производственном шаблоне
              </SourceLink>{" "}
              и{" "}
              <SourceLink path="docs/DEPLOYMENT.md">
                руководстве по развертыванию
              </SourceLink>
              .
            </p>
            <p>
              Для обеспечения наблюдаемости подготовьте{" "}
              <code>METRICS_TOKEN_FILE</code> и, в классическом режиме,{" "}
              <code>GRAFANA_PASSWORD_FILE</code>. Токен для доступа к метрикам
              должен совпадать с <code>METRICS_SECRET</code>. Все операторы на
              одном хосте должны использовать один и тот же абсолютный путь к{" "}
              <code>RELEASE_LOCK_DIR</code>.
            </p>
            <p>
              SMTP, push-уведомления и внешние календари по умолчанию отключены
              или используют <code>noop</code>. Включите их после отдельной
              настройки и тестирования. Для отправки писем нужны параметры SMTP.
              Запуск Compose сам по себе не настраивает эти интеграции.
            </p>
          </section>

          <section id="deployment" aria-labelledby="deployment-title">
            <div className="self-section-label">06 / РАЗВЕРТЫВАНИЕ</div>
            <h2 id="deployment-title">Начните с тестовой среды</h2>
            <p>
              После подготовки и передачи проверенного пакета укажите его
              абсолютные пути. <code>REVIEWED_VERSION</code> ниже — заглушка, а
              не существующий релиз. Замените её именем каталога своего пакета;
              коммит в манифесте должен совпадать с выбранным исходным кодом.
            </p>
            <Command title="Проверка подготовленного пакета">{`MEET_ENV=/srv/meet/config/staging.env\nMEET_MANIFEST=/srv/meet/releases/REVIEWED_VERSION/release.json\nexport RELEASE_LOCK_DIR=/srv/meet/locks\n\nbash scripts/release/release.sh validate \\\n  --environment staging --project recorder-staging \\\n  --env-file "$MEET_ENV" --manifest "$MEET_MANIFEST"`}</Command>
            <p>
              <code>validate</code> проверяет манифест и работу Compose. Не
              проверяет DNS, TLS, доступность хранилища и медиатрафик. Следующие
              команды изменяют выбранную среду. Сначала подготовьте конфигурацию
              и сделайте резервную копию всех существующих данных.
            </p>
            <Command title="Загрузка образов, миграция и развертывание в промежуточной среде">{`bash scripts/release/release.sh pull \\\n  --environment staging --project recorder-staging \\\n  --env-file "$MEET_ENV" --manifest "$MEET_MANIFEST"\n\nbash scripts/release/release.sh migrate \\\n  --environment staging --project recorder-staging \\\n  --env-file "$MEET_ENV" --manifest "$MEET_MANIFEST"\n\nbash scripts/release/release.sh deploy \\\n  --environment staging --project recorder-staging \\\n  --env-file "$MEET_ENV" --manifest "$MEET_MANIFEST"`}</Command>
            <p>
              <code>migrate</code> запускает зависимости и выполняет одноразовую
              миграцию из образа API. <code>deploy</code> не создает образы и не
              запускает SQL-запросы: последовательно обновляет приложения и
              ожидает изменения их статуса. PostgreSQL, RabbitMQ и объектное
              хранилище используют постоянные тома; локальные записи используют{" "}
              <code>RECORDER_STORAGE_DIR</code>. Redis хранит временное
              состояние. Не удаляйте тома во время обновления.
            </p>
            <div className="self-notice">
              <strong>Переходите в рабочую среду после проверки</strong>
              <p>
                Для операций, изменяющих рабочую среду, скрипту требуется{" "}
                <code>--evidence</code> с реальными результатами и{" "}
                <code>RELEASE_PRODUCTION_APPROVAL</code> со значением,
                соответствующим версии манифеста. Не отмечайте невыполненные
                проверки как пройденные и не переключайте среду на{" "}
                <code>local</code> для обхода проверки.
              </p>
            </div>
            <SourceLink path="docs/RELEASE_PROCESS.md#approval-production">
              Требования к доказательствам и развертыванию в рабочей среде
            </SourceLink>
          </section>

          <section id="verification" aria-labelledby="verification-title">
            <div className="self-section-label">07 / ПРОВЕРКА</div>
            <h2 id="verification-title">
              Протестируйте весь путь пользователя
            </h2>
            <Command title="Публичные проверки — используйте собственный домен">{`curl --fail --silent --show-error https://meet.example.com/version\ncurl --fail --silent --show-error https://meet.example.com/version.json\ncurl --fail --silent --show-error --output /dev/null https://meet.example.com/`}</Command>
            <ol className="self-check-list">
              <li>
                Сравните версию/коммит API и фронтенда с манифестом. Проверьте
                цепочку сертификатов TLS и загрузку ресурсов.
              </li>
              <li>
                Через закрытый доступ проверьте <code>/health/live</code> и{" "}
                <code>/health/ready</code> на изменённых сервисах. Публичный
                прокси намеренно блокирует конечные точки проверки
                работоспособности, сбора метрик и управления операциями.
              </li>
              <li>
                Используйте авторизованную существующую учетную запись для
                проверки входа, прав доступа, приглашений и подключений по
                протоколу WebSocket. Рабочая домашняя страница не доказывает,
                что аутентификация работает.
              </li>
              <li>
                На реальных устройствах и в разных сетях проверьте камеру,
                микрофон, демонстрацию экрана и переподключение. Для TURN
                проверьте фактически выбранную пару ICE с ретрансляцией, включая
                TURNS; одного открытого порта недостаточно.
              </li>
              <li>
                С согласия участников проверьте завершение записи,
                воспроизведение, превью и права доступа к файлам. Отдельно
                проверьте очереди и свободное место на диске.
              </li>
            </ol>
            <div className="self-note">
              <strong>Подключитесь к своему рабочему пространству</strong>
              <p>
                Откройте свой адрес <code>https://meet.example.com</code>, в
                браузере и войдите в систему. Замените домен из примера своим.
                Настройка произвольного адреса сервера в приложениях для
                телефона и компьютера в этом руководстве не проверялась.
              </p>
            </div>
            <h3>Развертывание с телефона</h3>
            <p>
              Мастер установки по SSH в Android-приложении не подтверждён
              проверенным исходным кодом. Руководство не описывает установку
              сервера из приложения и предназначено для администратора с
              SSH-доступом.
            </p>
          </section>

          <section id="maintenance" aria-labelledby="maintenance-title">
            <div className="self-section-label">08 / ОБСЛУЖИВАНИЕ</div>
            <h2 id="maintenance-title">
              Обновления, резервное копирование и восстановление
            </h2>
            <h3>Обновления</h3>
            <p>
              Сохраните предыдущий пакет и подготовьте новый на основе чистого,
              закрепленного снимка исходного кода. Протестируйте миграцию на
              копии данных и проверьте, поддерживает ли предыдущий код новую
              схему. Перед обновлением остановите все новые операции и дождитесь
              завершения встреч, записей и ожидающих обработки команд.
              Развертывайте в согласованный период технического обслуживания.
            </p>
            <h3>Резервные копии</h3>
            <p>
              <code>scripts/release/backup.sh</code> сохраняет дамп PostgreSQL и
              текущие объекты бакета. Для согласованной резервной копии на время
              копирования нужно остановить все процессы записи, включая прямые
              загрузки и внешние задания. Одного перевода приложения в режим
              завершения операций недостаточно.
            </p>
            <p>
              Помощник не сохраняет историю версий S3, снимок состояния
              RabbitMQ, незавершенные записи, TLS-сертификаты, конфигурацию и
              секретные данные. Сохраните их отдельно в своем плане
              восстановления, в том числе{" "}
              <code>PROVIDER_TOKEN_ENCRYPTION_KEY</code>, если вы используете
              зашифрованные токены. Защитите свои резервные копии и храните их
              отдельно от этого сервера.
            </p>
            <h3>Восстановление и откат</h3>
            <p>
              <code>scripts/release/restore-check.sh</code> проверяет резервную
              копию в отдельной локальной или тестовой среде. Скрипт не
              предназначен для восстановления поверх production. Затем проверьте
              данные и связи с файлами через приложение.{" "}
              <code>release.sh rollback</code> восстанавливает образы
              предыдущего пакета, но не откатывает SQL. Совместимость схемы
              нужно проверить отдельно.
            </p>
            <div className="self-resource-links">
              <SourceLink path="docs/operations/backup-restore.md">
                Пошаговое руководство по резервному копированию и восстановлению
              </SourceLink>
              <SourceLink path="docs/ROLLBACK.md">Процедура отката</SourceLink>
              <SourceLink path="docs/operations/README.md">
                Руководство по эксплуатации
              </SourceLink>
            </div>
          </section>

          <section id="troubleshooting" aria-labelledby="troubleshooting-title">
            <div className="self-section-label">
              09 / УСТРАНЕНИЕ НЕИСПРАВНОСТЕЙ
            </div>
            <h2 id="troubleshooting-title">Устранение неполадок</h2>
            <div className="self-faq">
              <details>
                <summary>
                  Compose сообщает о неизвестном теге или отсутствующем образе
                </summary>
                <p>
                  Проверьте поддержку <code>!override</code>, путь к манифесту и
                  его структуру. В рабочей среде Compose получает закрепленные
                  переменные образа через <code>release.sh</code>. Запуск файла
                  напрямую без этих переменных не является полноценным процессом
                  развертывания.
                </p>
              </details>
              <details>
                <summary>
                  Страница загружается, но при вызове не удается установить
                  соединение
                </summary>
                <p>
                  Проверьте публичный IP-адрес SFU, брандмауэр/NAT, диапазон
                  ретрансляции TURN, <code>TURN_URLS</code>, общий секретный
                  ключ и сертификат TURNS. Проверьте выбранную пару ICE в
                  браузере; проверка готовности HTTP не тестирует RTP или
                  ретрансляцию трафика.
                </p>
              </details>
              <details>
                <summary>
                  Соединения через WebSocket или вход в систему отклонены
                </summary>
                <p>
                  Проверьте HTTPS-origin, <code>WS_ALLOWED_ORIGINS</code>,{" "}
                  доверенные прокси и заголовки Upgrade. Проверьте совместимость
                  API и фронтенда и изучите приватные логи без публикации файлов
                  cookie, JWT и секретов.
                </p>
              </details>
              <details>
                <summary>
                  Сервис не проходит проверку готовности после обновления
                </summary>
                <p>
                  Проверьте статус зависимостей и ошибки миграции в приватных
                  логах. При <code>AUTO_MIGRATE=false</code> ожидающие
                  выполнения миграции блокируют проверку готовности. Не
                  редактируйте SQL-файлы, которые уже были применены: их
                  контрольные суммы проверены.
                </p>
              </details>
              <details>
                <summary>Запись не удается сохранить или воспроизвести</summary>
                <p>
                  Проверьте права доступа <code>1000:1000</code> и свободное
                  место в <code>RECORDER_STORAGE_DIR</code>, очередь RabbitMQ,
                  воркер и доступ к S3. Проверьте бакет и публичную
                  маршрутизацию для подписанных URL-адресов. Не заменяйте MinIO
                  на SeaweedFS, используя тот же том.
                </p>
              </details>
              <details>
                <summary>
                  Релиз останавливается на завершении операций или проверке
                  допуска в production
                </summary>
                <p>
                  Завершите активные операции и проверьте ожидающие команды
                  запуска записи. Тайм-аут завершения операций не вызывает
                  принудительный перезапуск. Для production сверьте версию,
                  коммит и SHA-256 манифеста с результатами проверок. Добавляйте
                  отсутствующий результат только после фактической проверки.
                </p>
              </details>
            </div>
          </section>
          <div className="self-endnote">
            <ShieldCheck size={22} aria-hidden="true" />
            <div>
              <strong>Руководство, которое вы можете проверить</strong>
              <p>
                Источники привязаны к <code>{sourceCommit.slice(0, 12)}</code>.
                Для другой версии перепроверьте Compose, переменные среды,
                миграции и процесс выпуска.
              </p>
            </div>
          </div>
        </main>
      </div>
      <footer className="self-footer self-container">
        <span>
          MeetSpace — единое пространство для встреч, общения и совместной
          работы
        </span>
        <a
          href="https://github.com/janickiy/meet-space"
          target="_blank"
          rel="noopener noreferrer"
        >
          Исходный код <ExternalLink size={14} aria-hidden="true" />
        </a>
      </footer>
    </div>
  );
}
