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
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-x64-test-setup.exe",
        sizeBytes: 121775536,
        sha256:
          "6ac2db3f70fa3f3ed43e136e17e28a8943467518e92c907c8333dc6ab31e4875",
      },
      {
        name: "MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        sizeBytes: 116516452,
        sha256:
          "4702dc821bd8364bbdb828098f4c3aeaccc167a6b1854836db7cb049c9cc65c5",
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
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-amd64-test.deb",
        sizeBytes: 100464852,
        sha256:
          "65fa62e0ca38b8db6b2f536dc33eabb6556d5f9d152eeccca4c5d8a01676b526",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        sizeBytes: 90096925,
        sha256:
          "fbc7d86741ff15e037dab4dc7c321c742cd0161d2fded99fa92e49af67cb4dcd",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        sizeBytes: 127015597,
        sha256:
          "a202a3ec6b10d4c825b0f1121a7b15cc536aeb6cd0a7bd9addc3cf69afc11b41",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        sizeBytes: 95688656,
        sha256:
          "ad8a312bf7fe9d2dcfceb9fea2fae2cf778fba6b9f9d6e0b36aefcb24340b3d2",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        sizeBytes: 85834613,
        sha256:
          "e280db4bc6676f4a9320d7a7d85f8b08fcd34169ee957be6500ca5064e5a74e5",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/4ca6c2eb653aabf9c2437f72a827ed7b10e0dd90/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        sizeBytes: 119182389,
        sha256:
          "41b5a19634906d0d6e0187581399ad6b7cd99b96e24c59f9cfff6f726a819d35",
      },
    ],
  },
];
