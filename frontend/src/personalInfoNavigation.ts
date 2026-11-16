/** Возвращает переписку или её информационный экран с сохранением фильтров списка. */
export function personalInfoLink(
  id: string,
  search = "",
  info = false,
  profileId?: string,
) {
  const params = new URLSearchParams(search);
  params.delete("view");
  params.delete("profile");
  if (info) params.set("view", "info");
  if (info && profileId) params.set("profile", profileId);
  const query = params.toString();
  return `/personal${id ? `/${encodeURIComponent(id)}` : ""}${query ? `?${query}` : ""}`;
}
