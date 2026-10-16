import {
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { api } from "../api";
import { useAuth } from "../auth";
import { recordingDate } from "../recordingPresentation";
import type { ConferenceRecording, Items } from "../types";
import { Button, ErrorNotice, Modal } from "./ui";

/** Removes cached artifacts only after the server confirms storage deletion. */
export function DeleteRecordingDialog({
  record,
  title,
  onClose,
}: {
  record: ConferenceRecording;
  title: string;
  onClose: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => api.deleteRecording(record.conferenceId, record.uuid),
    onSuccess: async () => {
      const pagesKey = [
        "conference-recording-pages",
        user?.id,
        record.conferenceId,
      ];
      const historyKey = ["history-recording-library", record.conferenceId];
      const listKey = ["recordings", record.conferenceId];
      const keys = [pagesKey, historyKey, listKey];
      await Promise.all(
        keys.map((queryKey) => client.cancelQueries({ queryKey })),
      );
      const withoutRecord = (page: Items<ConferenceRecording>) => ({
        ...page,
        items: page.items.filter((item) => item.uuid !== record.uuid),
      });
      for (const queryKey of [pagesKey, historyKey]) {
        client.setQueriesData<InfiniteData<Items<ConferenceRecording>>>(
          { queryKey },
          (cached) =>
            cached && { ...cached, pages: cached.pages.map(withoutRecord) },
        );
      }
      client.setQueryData<Items<ConferenceRecording>>(
        listKey,
        (cached) => cached && withoutRecord(cached),
      );
      client.removeQueries({
        queryKey: [
          "recording-detail",
          user?.id,
          record.conferenceId,
          record.uuid,
        ],
      });
      for (const queryKey of [
        ...keys,
        ["history", record.conferenceId],
        ["recording-conference-history", user?.id, record.conferenceId],
      ]) {
        // Refetch offsets after removal so pagination cannot skip a recording.
        void client.invalidateQueries({ queryKey });
      }
      onClose();
    },
  });
  return (
    <Modal
      title="Удалить запись?"
      onClose={() => {
        if (!mutation.isPending) onClose();
      }}
    >
      <p className="recordings-delete-title">
        {title} · {recordingDate(record)}
      </p>
      <p>
        Запись и её файлы будут удалены из хранилища для всех участников
        встречи. Восстановить их будет невозможно.
      </p>
      <ErrorNotice error={mutation.error} />
      <div className="recordings-delete-buttons">
        <Button
          variant="outline"
          disabled={mutation.isPending}
          onClick={onClose}
        >
          Отмена
        </Button>
        <Button
          variant="danger"
          busy={mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? "Удаляем…" : "Удалить запись"}
        </Button>
      </div>
    </Modal>
  );
}
