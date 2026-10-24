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
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-x64-test-setup.exe",
        sizeBytes: 121780467,
        sha256:
          "80e7a92e7d85316ba3bbb909439d9bb8286b4bf545e1dca387472ca1e3951bf0",
      },
      {
        name: "MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        sizeBytes: 116521395,
        sha256:
          "86361b634e4551a9613bbf6f5e20c2cb9b99374a8dfe5a4554a4ac6a66422d98",
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
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-amd64-test.deb",
        sizeBytes: 100468176,
        sha256:
          "9b59864bf9a24e2a55857157cc39c192871b0b8340367527f39c72c60a4840e9",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        sizeBytes: 90098901,
        sha256:
          "f52762cb839b14f96d394c8a9d78295409707d54d47537a2e60dd67a1a56eae9",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        sizeBytes: 127019781,
        sha256:
          "e099147353c59472015108d84600bc17fe4322e6f380d48a312cdd0d4af79b87",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        sizeBytes: 95691464,
        sha256:
          "d025d6f28276cadbbd9f5f1f276895c0a24047faa63c3b45ed4a5514a4a54a5a",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        sizeBytes: 85836713,
        sha256:
          "75164551af4e2fc9abce94f8076a993880684adc89fccd98fc1c8c375ea530e3",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/432fd28a6b9e984fdd4711ca92dfb8c4c7e077a0/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        sizeBytes: 119186432,
        sha256:
          "3eae401194128d2a93904961fb6ea044295f26ca9f0efa6935f4ddc5810f07fa",
      },
    ],
  },
];
