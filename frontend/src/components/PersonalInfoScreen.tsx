import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { ArrowLeft, MessageCircle } from "lucide-react";
import { api, ApiError } from "../api";
import { useAuth } from "../auth";
import type { PersonalConversation } from "../types";
import { personalInfoLink } from "../personalInfoNavigation";
import { Button, ErrorNotice, Loading } from "./ui";
import { UserAvatar } from "./UserAvatar";
import { DirectCallButton } from "./DirectCallButton";
import { PersonalPeerStatus } from "./PersonalPeerStatus";
import { GroupInfoModal } from "./GroupChats";
import { ConversationActions } from "./FolderPicker";
import { ConversationMaterials } from "./ConversationMaterials";
import "./personal-info-screen.css";

/** Информация занимает область переписки; список чатов и черновик остаются на месте. */
export function PersonalInfoScreen({
  conversation,
  profileId,
  onBack,
  onChat,
  onProfile,
  onMessage,
  onAccessDenied,
}: {
  conversation: PersonalConversation;
  profileId?: string;
  onBack: () => void;
  onChat: () => void;
  onProfile: (id: string) => void;
  onMessage: (id: string) => void;
  onAccessDenied: () => void;
}) {
  const heading = useRef<HTMLHeadingElement>(null);
  const group = conversation.type === "group" && !profileId;
  useEffect(() => {
    heading.current?.focus({ preventScroll: true });
  }, [conversation.id, profileId]);
  return (
    <section
      className="personal-info-screen"
      aria-labelledby="personal-info-heading"
    >
      <header className="personal-info-topbar">
        <button
          type="button"
          className="icon-button"
          aria-label={
            profileId ? "Назад к информации о группе" : "Назад к переписке"
          }
          onClick={onBack}
        >
          <ArrowLeft size={22} aria-hidden="true" />
        </button>
        <h2 id="personal-info-heading" ref={heading} tabIndex={-1}>
          {group ? "Информация о группе" : "Информация о пользователе"}
        </h2>
        {!profileId && <ConversationActions conversation={conversation} />}
      </header>
      <div className="personal-info-scroll">
        <div className="personal-info-body">
          {group && conversation.type === "group" ? (
            <GroupInfoModal
              embedded
              group={conversation}
              onClose={onChat}
              returnFocus={() => heading.current}
              onMemberProfile={(member) => onProfile(member.id)}
            >
              <ConversationMaterials
                conversation={conversation}
                onMessage={onMessage}
                onAccessDenied={onAccessDenied}
              />
            </GroupInfoModal>
          ) : (
            <ConversationProfile
              conversation={conversation}
              personId={
                conversation.type === "direct"
                  ? conversation.peer.id
                  : profileId!
              }
              onChat={onChat}
              onAccessDenied={onAccessDenied}
            />
          )}
          {!group && (
            <>
              {conversation.type === "group" && (
                <h3 className="personal-info-materials-heading">
                  Материалы группы «{conversation.name}»
                </h3>
              )}
              <ConversationMaterials
                conversation={conversation}
                onMessage={onMessage}
                onAccessDenied={onAccessDenied}
              />
            </>
          )}
        </div>
      </div>
    </section>
  );
}

function ConversationProfile({
  conversation,
  personId,
  onChat,
  onAccessDenied,
}: {
  conversation: PersonalConversation;
  personId: string;
  onChat: () => void;
  onAccessDenied: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const navigate = useNavigate();
  const alive = useRef(true);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      controller.current?.abort();
    };
  }, []);
  const query = useQuery({
    queryKey: ["personal-profile", conversation.id, user?.id, personId],
    queryFn: ({ signal }) =>
      api.conversationUserProfile(conversation.id, personId, signal),
    retry: false,
    refetchInterval: 15000,
  });
  const direct = useMutation({
    mutationFn: () => {
      controller.current = new AbortController();
      return api.createPersonalConversation(
        personId,
        controller.current.signal,
      );
    },
    onSuccess: ({ item }) => {
      if (!alive.current) return;
      client.setQueryData(["personal-detail", item.id, user?.id], {
        status: "success",
        item,
      });
      void client.invalidateQueries({ queryKey: ["personal-list", user?.id] });
      navigate(personalInfoLink(item.id));
    },
  });
  const denied =
    query.error instanceof ApiError && [403, 404].includes(query.error.status);
  const accessDenied = useRef(onAccessDenied);
  accessDenied.current = onAccessDenied;
  const callerDenied =
    query.error instanceof ApiError && query.error.status === 403;
  useEffect(() => {
    if (callerDenied) accessDenied.current();
  }, [callerDenied]);
  // Не оставляем старые контактные данные на экране после отзыва доступа к профилю.
  const profile = denied ? undefined : query.data?.item;
  return (
    <>
      <ErrorNotice error={query.error || direct.error} />
      {query.isPending && <Loading />}
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку профиля
        </Button>
      )}
      {profile && (
        <>
          <div className="conversation-profile-hero">
            <UserAvatar
              id={profile.id}
              name={profile.displayName}
              className="conversation-profile-avatar"
            />
            <h3>{profile.displayName}</h3>
            {conversation.type === "direct" && (
              <PersonalPeerStatus
                conversation={conversation}
                onAccessDenied={onAccessDenied}
              />
            )}
            {conversation.type === "group" && (
              <p className="muted">Участник группы «{conversation.name}»</p>
            )}
          </div>
          <div className="conversation-profile-actions">
            <Button
              variant="outline"
              busy={direct.isPending}
              onClick={() => {
                if (conversation.type === "direct" || personId === user?.id)
                  onChat();
                else if (!direct.isPending) direct.mutate();
              }}
            >
              <MessageCircle size={22} aria-hidden="true" />
              {personId === user?.id && conversation.type === "group"
                ? "Чат группы"
                : "Сообщение"}
            </Button>
            <DirectCallButton
              userId={profile.id}
              name={profile.displayName}
              label
            />
          </div>
          <dl className="conversation-profile-details">
            <div>
              <dt>Имя</dt>
              <dd>{profile.displayName}</dd>
            </div>
            {profile.bio?.trim() && (
              <div>
                <dt>О себе</dt>
                <dd className="conversation-profile-bio">{profile.bio}</dd>
              </div>
            )}
            <div>
              <dt>Email</dt>
              <dd>
                {profile.email ? (
                  <a href={`mailto:${encodeURIComponent(profile.email)}`}>
                    {profile.email}
                  </a>
                ) : (
                  "Не указан"
                )}
              </dd>
            </div>
          </dl>
        </>
      )}
    </>
  );
}
