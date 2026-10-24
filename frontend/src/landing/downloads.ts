/**
 * Published client downloads checked on 23 October 2026 and captured at build time.
 * Source repositories publish binaries in Git folders; GitHub release assets are empty.
 * URLs are pinned to the inspected commits so sizes and publisher checksums stay aligned.
 * A source-code archive is not an application installer.
 */
export type DownloadPlatformId = "android" | "windows" | "linux";

export interface DownloadAsset {
  name: string;
  version: string;
  architecture: string;
  format: string;
  url: string;
  sizeBytes: number | null;
  /** Publisher-provided checksum; this is not a signing or runtime compatibility claim. */
  sha256: string | null;
}

export interface DownloadPlatform {
  id: DownloadPlatformId;
  label: string;
  status: "available" | "unavailable";
  description: string;
  repositoryUrl: string;
  /** Release notes or the directory containing published builds. */
  releaseUrl: string;
  buildNote: string;
  targetArchitectures: readonly string[];
  targetFormats: readonly string[];
  /** Declared minimum OS requirements; empty when no broad compatibility claim is verified. */
  supportedSystems: readonly string[];
  assets: readonly DownloadAsset[];
}

export const PROJECT_URL = "https://github.com/janickiy/meet-space";
export const DESKTOP_PROJECT_URL =
  "https://github.com/janickiy/meet-space-desktop";
export const ANDROID_PROJECT_URL =
  "https://github.com/janickiy/meet-space-android";
export const HOSTED_APP_URL = "https://meeting.janickiy.com/app";

/** Declared OS targets; per-package release notes document compatibility limits. */
export const WINDOWS_OS_VERSIONS: Readonly<Record<string, string>> = {
  x64: "Windows 10 22H2, Windows 11",
  ARM64: "Windows 11",
};

export const LINUX_DISTRIBUTIONS = [
  {
    name: "Ubuntu",
    format: "DEB",
    versions: "Ubuntu 22.04 LTS, 24.04 LTS",
    arm64Versions: "Ubuntu 24.04 LTS",
  },
  {
    name: "Debian",
    format: "DEB",
    versions: "Debian 12, 13",
    arm64Versions: null,
  },
  {
    name: "Fedora",
    format: "RPM",
    versions: "Fedora Workstation 44",
    arm64Versions: null,
  },
] as const;

/** Server release metadata; client build versions are recorded on each asset. */
export const RELEASE = {
  version: "0.3.5",
  name: "0.3.5 beta",
  url: `${PROJECT_URL}/releases/tag/0.3.5`,
  publishedAt: "2026-10-18T23:46:50Z",
  checkedAt: "2026-10-19",
} as const;

export const DOWNLOADS: readonly DownloadPlatform[] = [
  {
    id: "android",
    label: "Android",
    status: "available",
    description:
      "Берите с собой свою команду. Загрузите бета-версию для Android на свой телефон или планшет.",
    repositoryUrl: "https://github.com/janickiy/meet-space-android",
    releaseUrl:
      "https://github.com/janickiy/meet-space-android/releases/tag/0.3.8-beta",
    buildNote: "Бета-версия APK",
    targetArchitectures: ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"],
    targetFormats: ["APK"],
    supportedSystems: ["Android 8.0 и новее"],
    assets: [
      {
        name: "MeetSpace-0.3.8-beta.apk",
        version: "0.3.8-beta",
        architecture: "Universal",
        format: "APK",
        url: "https://github.com/janickiy/meet-space-android/raw/86915851ca4ea7d0717ce4e98fdc9d11b2b0fda5/dist/MeetSpace-0.3.8-beta.apk",
        sizeBytes: 52850224,
        sha256:
          "fac7bcdbbd307e10be611c3dd34b822f986fd4b8ed47a8ae45b66fbb1ad69ee2",
      },
    ],
  },
  {
    id: "windows",
    label: "Windows",
    status: "available",
    description:
      "Отдельное приложение для компьютера с установщиками для x64 и ARM64.",
    repositoryUrl: "https://github.com/janickiy/meet-space-desktop",
    releaseUrl:
      "https://github.com/janickiy/meet-space-desktop/releases/tag/0.1.2-beta",
    buildNote: "Тестовая бета-версия · Установщик без цифровой подписи",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["EXE"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.2-beta-win-x64-test-setup.exe",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-x64-test-setup.exe",
        sizeBytes: 121753483,
        sha256:
          "3b21846b1f73141743a1dd69829f48e3790332b77b3bef47b6c7cd909b83656e",
      },
      {
        name: "MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        sizeBytes: 116494316,
        sha256:
          "38896ade79686783d7f9b169bafafa279c84062763e4150aee8875f23defdff9",
      },
    ],
  },
  {
    id: "linux",
    label: "Linux",
    status: "available",
    description:
      "Выберите Ubuntu, Debian или Fedora и архитектуру x64 (Intel / AMD) или ARM64.",
    repositoryUrl: "https://github.com/janickiy/meet-space-desktop",
    releaseUrl:
      "https://github.com/janickiy/meet-space-desktop/releases/tag/0.1.2-beta",
    buildNote: "Тестовая бета-версия · Проверьте требования пакета",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["DEB", "RPM", "AppImage"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.2-beta-linux-amd64-test.deb",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-amd64-test.deb",
        sizeBytes: 100437924,
        sha256:
          "f316cfd52a06c088e69db6eb4acb1a100af833d4da2a1ef648edde554923abc1",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        sizeBytes: 90079817,
        sha256:
          "258611790b851fa86aa9dead95df9e10a09cdfd1663ec1532ed87dd0a95397e8",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        sizeBytes: 126888488,
        sha256:
          "c0ac2540941a1066d1aac42ebf0a89b209714420fcecfa4af5180f3bc5ef9657",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        sizeBytes: 95662188,
        sha256:
          "dcd54dace5d4ffae837308e330db0e0ee30842f59f235b4e745ba7aeeb8091da",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        sizeBytes: 85800593,
        sha256:
          "8e515daf5104cefe4ef2d8c4931bf14bdd91c1352f3ea19b34faf97b6c5fa4d1",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        sizeBytes: 119067319,
        sha256:
          "7e008f306090dc1703fc2a27c37959bd12f15100791dc890479ea1886f6e84cc",
      },
    ],
  },
];
