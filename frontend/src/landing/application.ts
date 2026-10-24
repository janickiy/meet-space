/** A standalone marketing host sends account actions to the application host. */
export const MARKETING_ONLY = import.meta.env.VITE_MARKETING_ONLY === "true";

const application = new URL(
  import.meta.env.VITE_PUBLIC_APP_URL || "https://meeting.janickiy.com",
);
if (
  !["https:", "http:"].includes(application.protocol) ||
  application.username ||
  application.password ||
  application.pathname !== "/" ||
  application.search ||
  application.hash
) {
  throw new Error("VITE_PUBLIC_APP_URL must be an absolute HTTP(S) origin");
}
export const PUBLIC_APPLICATION_ORIGIN = application.origin;

export function applicationPath(path: string) {
  if (!path.startsWith("/") || path.startsWith("//")) {
    throw new Error("Application links must use a local absolute path");
  }
  return MARKETING_ONLY ? `${PUBLIC_APPLICATION_ORIGIN}${path}` : path;
}
