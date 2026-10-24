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
      "https://github.com/janickiy/meet-space-desktop/tree/bad70077d6950eb1c0d600186235fbfadb680b44/source/windows/0.1.3-beta",
    buildNote: "Тестовая бета-версия · Установщик без цифровой подписи",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["EXE"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.3-beta-win-x64-test-setup.exe",
        version: "0.1.3-beta",
        architecture: "x64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/windows/0.1.3-beta/MeetSpace-0.1.3-beta-win-x64-test-setup.exe",
        sizeBytes: 121778551,
        sha256:
          "85f6ee214da1cf81a8b44babfc7b6822a2f49b760a61cee4556921928caa0c6c",
      },
      {
        name: "MeetSpace-0.1.3-beta-win-arm64-test-setup.exe",
        version: "0.1.3-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/windows/0.1.3-beta/MeetSpace-0.1.3-beta-win-arm64-test-setup.exe",
        sizeBytes: 116519325,
        sha256:
          "c71840e7e08eff75844312f79ca5a60925ae04ff32c4ff78e6156c15f6e7fedc",
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
      "https://github.com/janickiy/meet-space-desktop/tree/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta",
    buildNote: "Тестовая бета-версия · Проверьте требования пакета",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["DEB", "RPM", "AppImage"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.3-beta-linux-amd64-test.deb",
        version: "0.1.3-beta",
        architecture: "x64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta/DEB/MeetSpace-0.1.3-beta-linux-amd64-test.deb",
        sizeBytes: 100469080,
        sha256:
          "62f38ce1e70a507b11ddfa2bb2342367d88fbaba55ef1d70c08c29ae5aa4fbdd",
      },
      {
        name: "MeetSpace-0.1.3-beta-linux-x86_64-test.rpm",
        version: "0.1.3-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta/RPM/MeetSpace-0.1.3-beta-linux-x86_64-test.rpm",
        sizeBytes: 90093313,
        sha256:
          "1be9478e53cc5438ed602450d93fa3c535b41e875f006c6726e5be69d126d487",
      },
      {
        name: "MeetSpace-0.1.3-beta-linux-x86_64-test.AppImage",
        version: "0.1.3-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta/AppImage/MeetSpace-0.1.3-beta-linux-x86_64-test.AppImage",
        sizeBytes: 127019691,
        sha256:
          "edd13584e37ff0785af311ed6c2059e74c9f558f784fedc6f74f60d63cc60125",
      },
      {
        name: "MeetSpace-0.1.3-beta-linux-arm64-test.deb",
        version: "0.1.3-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta/DEB/MeetSpace-0.1.3-beta-linux-arm64-test.deb",
        sizeBytes: 95691864,
        sha256:
          "f9142c2361236c18c628d96f23222a802635ee7bd1d1d692fa7742adaec84892",
      },
      {
        name: "MeetSpace-0.1.3-beta-linux-aarch64-test.rpm",
        version: "0.1.3-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta/RPM/MeetSpace-0.1.3-beta-linux-aarch64-test.rpm",
        sizeBytes: 85837353,
        sha256:
          "2d197672ee59f115293bdeed36f6133d2f1a46b13d954384a38989b8186fbc7e",
      },
      {
        name: "MeetSpace-0.1.3-beta-linux-arm64-test.AppImage",
        version: "0.1.3-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/bad70077d6950eb1c0d600186235fbfadb680b44/source/linux/0.1.3-beta/AppImage/MeetSpace-0.1.3-beta-linux-arm64-test.AppImage",
        sizeBytes: 119186483,
        sha256:
          "f80fd86d18c07e3152da65ea98b9ffddb2f72b0f9dd66cf913510c7bed3dcc4f",
      },
    ],
  },
];
