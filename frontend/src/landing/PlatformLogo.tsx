/** Original platform artwork; sources and attribution are in public/branding/PLATFORM-LOGOS.md. */
const PLATFORM_LOGOS = {
  android: "/branding/android-robot.svg",
  linux: "/branding/linux-tux.png",
  windows: "/branding/windows.svg",
} as const;

export function PlatformLogo({
  platform,
  size = 24,
}: {
  platform: keyof typeof PLATFORM_LOGOS;
  size?: number;
}) {
  return (
    <img
      className="lp-platform-logo"
      src={PLATFORM_LOGOS[platform]}
      width={size}
      height={size}
      alt=""
      aria-hidden="true"
    />
  );
}
