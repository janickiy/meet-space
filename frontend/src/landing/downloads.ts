/**
 * Published desktop downloads checked on 24 October 2026 and captured at build time.
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
        name: "MeetSpace-0.3.11-beta.apk",
        version: "0.3.11-beta",
        architecture: "Universal",
        format: "APK",
        url: "https://github.com/janickiy/meet-space-android/raw/86915851ca4ea7d0717ce4e98fdc9d11b2b0fda5/dist/MeetSpace-0.3.11-beta.apk",
        sizeBytes: 78143680,
        sha256:
          "6de79eeaaba9c77381d7d503198b394e5676c7313b3d7c0bf9d0d8da780336ae",
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
      "https://github.com/janickiy/meet-space-desktop/tree/855374bfe9067dff12b44acf1b3ee646097910ef/source/windows/0.1.6-beta",
    buildNote: "Тестовая бета-версия · Установщик без цифровой подписи",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["EXE"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.6-beta-win-x64-test-setup.exe",
        version: "0.1.6-beta",
        architecture: "x64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/windows/0.1.6-beta/MeetSpace-0.1.6-beta-win-x64-test-setup.exe",
        sizeBytes: 121789024,
        sha256:
          "261562b6c8c21a88867106e3962465ea8a629e5e3690a9f5abac0ccd56416ba3",
      },
      {
        name: "MeetSpace-0.1.6-beta-win-arm64-test-setup.exe",
        version: "0.1.6-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/windows/0.1.6-beta/MeetSpace-0.1.6-beta-win-arm64-test-setup.exe",
        sizeBytes: 116529860,
        sha256:
          "61cf14b0dee9409d064389d1c6228ef4582411e1be7c1ff9c1152f83d22a9924",
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
      "https://github.com/janickiy/meet-space-desktop/tree/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta",
    buildNote: "Тестовая бета-версия · Проверьте требования пакета",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["DEB", "RPM", "AppImage"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.6-beta-linux-amd64-test.deb",
        version: "0.1.6-beta",
        architecture: "x64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta/DEB/MeetSpace-0.1.6-beta-linux-amd64-test.deb",
        sizeBytes: 100478872,
        sha256:
          "c21c21aaa0832c0de0877c527aec3774551baf95703102aa9310215a6ae6a94f",
      },
      {
        name: "MeetSpace-0.1.6-beta-linux-x86_64-test.rpm",
        version: "0.1.6-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta/RPM/MeetSpace-0.1.6-beta-linux-x86_64-test.rpm",
        sizeBytes: 90107113,
        sha256:
          "86e945dd7056459bedf4050d6bed3b782b2c1ef15702d5db1fde2276b3e5b0af",
      },
      {
        name: "MeetSpace-0.1.6-beta-linux-x86_64-test.AppImage",
        version: "0.1.6-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta/AppImage/MeetSpace-0.1.6-beta-linux-x86_64-test.AppImage",
        sizeBytes: 127060803,
        sha256:
          "2c794328c04c43cf998b1a6855894fa8cbacbc28d19c323e1135d2702d844bec",
      },
      {
        name: "MeetSpace-0.1.6-beta-linux-arm64-test.deb",
        version: "0.1.6-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta/DEB/MeetSpace-0.1.6-beta-linux-arm64-test.deb",
        sizeBytes: 95701632,
        sha256:
          "4c6e5c1c0cfbc83537e7953a46e62037b8450ebbc9cdc944da041f3601a0c063",
      },
      {
        name: "MeetSpace-0.1.6-beta-linux-aarch64-test.rpm",
        version: "0.1.6-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta/RPM/MeetSpace-0.1.6-beta-linux-aarch64-test.rpm",
        sizeBytes: 85831345,
        sha256:
          "4488cb0d08eea6ebf92f3fd0f96e7f92060e194b0ac3357ab5a74e3b21f18f5c",
      },
      {
        name: "MeetSpace-0.1.6-beta-linux-arm64-test.AppImage",
        version: "0.1.6-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/855374bfe9067dff12b44acf1b3ee646097910ef/source/linux/0.1.6-beta/AppImage/MeetSpace-0.1.6-beta-linux-arm64-test.AppImage",
        sizeBytes: 119219187,
        sha256:
          "a23f1f371bd5b10fd4ab520bd7aaccc807a5e60564f831c5d637c77a622e85f3",
      },
    ],
  },
];
