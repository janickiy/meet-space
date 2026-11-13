/** Deletes in order and stops on failure so the remaining selection can be retried. */
export async function deleteChatMessages<T>(
  ids: string[],
  remove: (id: string, signal: AbortSignal) => Promise<T>,
  signal: AbortSignal,
  onDeleted: (id: string, result: T) => void,
) {
  const unique = [...new Set(ids)];
  const deleted: string[] = [];
  for (const id of unique) {
    if (signal.aborted) break;
    try {
      const result = await remove(id, signal);
      deleted.push(id);
      onDeleted(id, result);
    } catch (error) {
      return { deleted, remaining: unique.slice(deleted.length), error };
    }
  }
  return { deleted, remaining: unique.slice(deleted.length), error: null };
}
