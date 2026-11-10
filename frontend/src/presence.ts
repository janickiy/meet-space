import type { Participant, PresenceParticipant, RealtimeState } from "./types";

/** Применяет новое состояние медиа без изменения физического присутствия. */
export function applyParticipantMediaUpdate(
  state: RealtimeState | null,
  data: unknown,
): RealtimeState | null {
  const update = (data as { participant?: Participant } | null)?.participant;
  if (
    !state ||
    typeof update?.id !== "string" ||
    typeof update.updatedAt !== "string" ||
    typeof update.microphoneEnabled !== "boolean"
  )
    return state;

  const updatedAt = Date.parse(update.updatedAt);
  const current = state.participants.find((person) => person.id === update.id);
  if (
    !current ||
    !Number.isFinite(updatedAt) ||
    updatedAt < Date.parse(current.updatedAt) ||
    (update.mediaPolicyVersion ?? 0) < (current.mediaPolicyVersion ?? 0)
  )
    return state;

  const next = {
    ...current,
    ...update,
    online: current.online,
    connections: current.connections,
    connectionIds: current.connectionIds,
  };
  return {
    ...state,
    participants: state.participants.map((person) =>
      person === current ? next : person,
    ),
  };
}

/** Собирает состав живой комнаты по подтверждённому сервером присутствию.
 * Членство и история сами по себе не подтверждают подключение. Последний снимок
 * присутствия дополняет состав, а более свежая запись обновляет данные участника.
 * @args participants — сохранённые членства в порядке API; presence — последний снимок WebSocket или отсутствие связи.
 * @return Уникальные допущенные участники онлайн; исходные массивы не изменяются.
 */
export function onlineParticipants(
  participants: readonly Participant[],
  presence: readonly PresenceParticipant[] | undefined,
): Participant[] {
  if (!presence) return [];
  const snapshot = new Map(presence.map((person) => [person.id, person]));
  const merged = new Map(participants.map((person) => [person.id, person]));
  for (const person of snapshot.values()) {
    const saved = merged.get(person.id);
    const savedIsNewer =
      saved && Date.parse(saved.updatedAt) > Date.parse(person.updatedAt);
    merged.set(
      person.id,
      savedIsNewer ? { ...person, ...saved } : { ...saved, ...person },
    );
  }
  return [...merged.values()].filter((person) => {
    const current = snapshot.get(person.id);
    return (
      current?.online === true &&
      person.status === "joined" &&
      (!person.admissionState || person.admissionState === "admitted")
    );
  });
}
