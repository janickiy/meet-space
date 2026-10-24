import {
  ArrowLeft,
  Circle,
  Download,
  FileText,
  Link,
  LogOut,
  MessageCircle,
  Mic,
  MonitorUp,
  Paperclip,
  RefreshCw,
  SendHorizontal,
  Settings,
  ShieldCheck,
  Smile,
  Users,
  Video,
  X,
} from "lucide-react";
import "./landing-conference-preview.css";

const participants = ["Анна", "Михаил", "Елена", "Александр · вы"];
const controls = [
  { Icon: Mic, label: "Микрофон", active: true },
  { Icon: Video, label: "Камера", active: true },
  { Icon: MonitorUp, label: "Экран" },
  { Icon: Settings, label: "Настройки" },
  { Icon: Users, label: "Участники" },
  { Icon: MessageCircle, label: "Чат" },
  { Icon: LogOut, label: "Выйти", danger: true },
];

/** Passive illustration of ActiveConferenceRoom; never starts media or joins a room. */
export function LandingConferencePreview() {
  return (
    <div className="lp-conference-preview" aria-hidden="true">
      <div className="lp-conf-room">
        <div className="lp-conf-header">
          <ArrowLeft className="lp-conf-back" size={19} />
          <div className="lp-conf-title">
            <strong>Начинаем неделю вместе.</strong>
            <span>Встреча в эфире</span>
          </div>
          <span className="lp-conf-clock">00:12:48</span>
          <div className="lp-conf-header-actions">
            <span className="lp-conf-header-action">
              <Circle size={17} /> Запись
            </span>
            <span className="lp-conf-header-action">
              <Link size={17} /> Пригласить
            </span>
            <span className="lp-conf-header-action">
              <RefreshCw size={20} />
            </span>
          </div>
        </div>

        <div className="lp-conf-stage">
          <div className="lp-conf-grid">
            {participants.map((name, index) => (
              <div className="lp-conf-tile" key={name}>
                <div className={`lp-conf-photo lp-conf-photo-${index}`}>
                  <img
                    src="/media/meetspace-group-chat-v6.webp"
                    width="1536"
                    height="1024"
                    alt=""
                    fetchPriority={index === 0 ? "high" : "auto"}
                  />
                </div>
                <span className="lp-conf-person">
                  <span>{name}</span>
                  {index === 3 && <ShieldCheck size={15} />}
                  <span className="lp-conf-mic">
                    <Mic size={15} />
                  </span>
                </span>
              </div>
            ))}
          </div>

          <div className="lp-conf-chat">
            <div className="lp-conf-chat-header">
              <strong>Чат встречи</strong>
              <X size={18} />
            </div>
            <div className="lp-conf-chat-tabs">
              <span className="selected">Чат</span>
              <span>Участники (4)</span>
            </div>
            <div className="lp-conf-thread">
              <div className="lp-conf-day">
                <span>Сегодня</span>
              </div>
              <div className="lp-conf-message">
                <span className="lp-conf-avatar">А</span>
                <div className="lp-conf-message-content">
                  <strong>Анна</strong>
                  <div className="lp-conf-bubble">
                    <p>Доброе утро, команда! Вот план на сегодня.</p>
                    <div className="lp-conf-file">
                      <span className="lp-conf-file-icon">
                        <FileText size={18} />
                      </span>
                      <span>
                        <strong>Бриф проекта.pdf</strong>
                        <small>PDF · 240 КБ</small>
                      </span>
                      <Download size={14} />
                    </div>
                    <span className="lp-conf-message-time">10:42</span>
                  </div>
                </div>
              </div>
              <div className="lp-conf-message">
                <span className="lp-conf-avatar">М</span>
                <div className="lp-conf-message-content">
                  <strong>Михаил</strong>
                  <div className="lp-conf-bubble">
                    <p>Выглядит неплохо. Давайте разберём его вместе.</p>
                    <span className="lp-conf-message-time">10:43</span>
                  </div>
                </div>
              </div>
            </div>
            <div className="lp-conf-composer">
              <div className="lp-conf-compose-row">
                <span className="lp-conf-placeholder">Напишите сообщение…</span>
                <span className="lp-conf-compose-action">
                  <Paperclip size={18} />
                </span>
                <span className="lp-conf-compose-action">
                  <Smile size={18} />
                </span>
                <span className="lp-conf-compose-action lp-conf-send">
                  <SendHorizontal size={18} />
                </span>
              </div>
            </div>
          </div>
        </div>

        <div className="lp-conf-footer">
          <div className="lp-conf-controls">
            {controls.map(({ Icon, label, active, danger }) => (
              <span
                className={`lp-conf-control${active ? " is-active" : ""}${danger ? " is-danger" : ""}`}
                key={label}
              >
                <Icon size={20} />
                <span>{label}</span>
              </span>
            ))}
          </div>
          <div className="lp-conf-footer-meta">
            <ShieldCheck size={14} /> Доступно по приглашению
          </div>
        </div>
      </div>
    </div>
  );
}
