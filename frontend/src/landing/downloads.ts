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
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-x64-test-setup.exe",
        sizeBytes: 121776956,
        sha256:
          "e66d4cf8f329f35103db14581a2ad4b68152d969bfe9cdb86352c67285ac3bca",
      },
      {
        name: "MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/windows/0.1.2-beta/MeetSpace-0.1.2-beta-win-arm64-test-setup.exe",
        sizeBytes: 116517854,
        sha256:
          "486503e3f3366b1874d3dc934061271c32b5e838442d93a26a7f92edfa889551",
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
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-amd64-test.deb",
        sizeBytes: 100464488,
        sha256:
          "cbe6353d6516d4a54bc6d270b05cde287911c0aacbccf30eac4952a587593df8",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-x86_64-test.rpm",
        sizeBytes: 90089729,
        sha256:
          "c87115164f1b4217f7bf719317464c5aff4d5f20de0821d3d41940ea5bef6844",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage",
        sizeBytes: 127019883,
        sha256:
          "a460d5cc64b371860918491f69cfd18d7ddb848672b5875dc6ecdddb744bdd77",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/linux/0.1.2-beta/DEB/MeetSpace-0.1.2-beta-linux-arm64-test.deb",
        sizeBytes: 95688312,
        sha256:
          "4fc38c5305e3dac4379ec82e7e0a84a0412840244470c6d600211559184476ea",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/linux/0.1.2-beta/RPM/MeetSpace-0.1.2-beta-linux-aarch64-test.rpm",
        sizeBytes: 85834885,
        sha256:
          "1fc796b7a7b5371732d872353385c7848afda32fd61a5e01aa2815bb0d8e802e",
      },
      {
        name: "MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        version: "0.1.2-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/791a591ff08ad179eaa34da8236a6fe63427fcd1/source/linux/0.1.2-beta/AppImage/MeetSpace-0.1.2-beta-linux-arm64-test.AppImage",
        sizeBytes: 119182248,
        sha256:
          "63dc59995d7c66320a3a59aef2a38c8ec59e9ec80946cd56313d3281dc3c57f8",
      },
    ],
  },
];
