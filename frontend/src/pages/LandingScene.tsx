import { useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import {
  CalendarDays,
  Check,
  ChevronDown,
  FolderOpen,
  Maximize2,
  MessageCircle,
  Plus,
  Send,
  Settings,
  Smile,
  Users,
  Video,
} from "lucide-react";
import { LandingConferencePreview } from "./LandingConferencePreview";

const tabs = [
  { id: "meetings", label: "Встречи", Icon: Video },
  { id: "messages", label: "Сообщения", Icon: MessageCircle },
  { id: "calendar", label: "Календарь", Icon: CalendarDays },
] as const;
type PreviewTab = (typeof tabs)[number]["id"];

export function LandingScene() {
  const [active, setActive] = useState<PreviewTab>("meetings");
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  function onTabKey(event: KeyboardEvent, index: number) {
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft")
      next = (index + tabs.length - 1) % tabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = tabs.length - 1;
    else return;
    event.preventDefault();
    setActive(tabs[next].id);
    refs.current[next]?.focus();
  }
  return (
    <div id="product-preview" className="lp-preview">
      <div
        className="lp-preview-tabs"
        role="tablist"
        aria-label="Изучите интерфейс MeetSpace"
      >
        {tabs.map(({ id, label, Icon }, index) => (
          <button
            key={id}
            id={`preview-tab-${id}`}
            ref={(el) => {
              refs.current[index] = el;
            }}
            role="tab"
            aria-selected={id === active}
            aria-controls="preview-panel"
            tabIndex={active === id ? 0 : -1}
            onClick={() => setActive(id)}
            onKeyDown={(e) => onTabKey(e, index)}
          >
            <Icon size={14} />
            {label}
          </button>
        ))}
      </div>
      <div
        id="preview-panel"
        className="lp-preview-panel"
        role="tabpanel"
        aria-labelledby={`preview-tab-${active}`}
        tabIndex={0}
      >
        <p className="sr-only">
          {active === "meetings"
            ? "Интерфейс веб-конференции: четыре участника, чат встречи и панель управления микрофоном, камерой и демонстрацией экрана. Демонстрационные данные."
            : active === "messages"
              ? "Концепция группового чата: участники команды обсуждают проект и обмениваются PDF-файлами. Демонстрационный контент."
              : "Концепция календаря: командное совещание в 10:00 и мозговой штурм в 14:00. Демонстрационный контент."}
        </p>
        {active === "meetings" ? (
          <LandingConferencePreview />
        ) : (
          <div className="lp-app-window" aria-hidden="true">
            <div className="lp-window-top">
              <div className="lp-window-dots">
                <i />
                <i />
                <i />
              </div>
              <span>
                <img
                  src="/branding/meetspace-symbol-color.svg"
                  width="12"
                  height="13"
                  alt=""
                />{" "}
                meetspace
              </span>
              <Maximize2 size={11} />
            </div>
            <div className="lp-window-content">
              <div className="lp-window-sidebar">
                <img
                  src="/branding/meetspace-symbol-color.svg"
                  width="24"
                  height="25"
                  alt=""
                />
                <span>
                  <Video size={18} />
                </span>
                <span className={active === "messages" ? "selected" : ""}>
                  <MessageCircle size={18} />
                </span>
                <span className={active === "calendar" ? "selected" : ""}>
                  <CalendarDays size={18} />
                </span>
                <span>
                  <FolderOpen size={18} />
                </span>
                <span className="lp-sidebar-bottom">
                  <Settings size={17} />
                </span>
                <i>А</i>
              </div>
              <div className="lp-window-main">
                {active === "messages" && (
                  <div className="lp-demo-messages">
                    <div className="lp-meeting-title">
                      <div>
                        <strong>
                          Команда по продукту <ChevronDown size={12} />
                        </strong>
                        <span>
                          <span className="lp-live-dot" /> 4 участника
                        </span>
                      </div>
                      <Users size={18} />
                    </div>
                    <p className="lp-demo-day">Сегодня</p>
                    <div className="lp-chat-message">
                      <span className="lp-avatar lp-avatar-peach">А</span>
                      <div>
                        <strong>
                          Анна <small>12:50</small>
                        </strong>
                        <p>
                          Отличное совещание! Все наши идеи в одном месте. ✨
                        </p>
                        <div className="lp-file-chip">
                          <FolderOpen size={18} />
                          Идеи проекта.pdf
                        </div>
                      </div>
                    </div>
                    <div className="lp-demo-reply">
                      Мне нравится. Смотрю - скоро появятся мысли!
                      <small>
                        12:51 <Check size={11} />
                      </small>
                    </div>
                    <div className="lp-chat-message">
                      <span className="lp-avatar">М</span>
                      <div>
                        <strong>
                          Михаил <small>12:52</small>
                        </strong>
                        <p>Вот за что я люблю эту команду. 🙌</p>
                      </div>
                    </div>
                    <div className="lp-demo-input">
                      <Plus size={15} />
                      <span>Напишите сообщение…</span>
                      <Smile size={15} />
                      <Send size={16} />
                    </div>
                  </div>
                )}
                {active === "calendar" && (
                  <div className="lp-demo-calendar">
                    <div className="lp-meeting-title">
                      <div>
                        <strong>Ваш день. Ваш ритм.</strong>
                        <span>
                          Октябрь 2026 <ChevronDown size={12} />
                        </span>
                      </div>
                      <CalendarDays size={18} />
                    </div>
                    <div className="lp-calendar-week">
                      {["Пн 19", "Вт 20", "Ср 21", "Чт 22", "Пт 23"].map(
                        (day, i) => (
                          <span className={i === 1 ? "current" : ""} key={day}>
                            {day.split(" ")[0]}
                            <strong>{day.split(" ")[1]}</strong>
                          </span>
                        ),
                      )}
                    </div>
                    <div className="lp-calendar-event">
                      <time>10:00</time>
                      <div>
                        <Video size={16} />
                        <strong>Встреча команды</strong>
                        <span>30 минут · 4 участника</span>
                      </div>
                    </div>
                    <div className="lp-calendar-event violet">
                      <time>14:00</time>
                      <div>
                        <Users size={16} />
                        <strong>Большие идеи - вместе</strong>
                        <span>60 минут · Продуктовая команда</span>
                      </div>
                    </div>
                    <div className="lp-calendar-free">
                      <span /> Место для вашей следующей идеи
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
