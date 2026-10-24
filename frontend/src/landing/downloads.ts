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
      "https://github.com/janickiy/meet-space-desktop/tree/3e5a487298fb0fee517216809bcb151b51862e27/source/windows/0.1.4-beta",
    buildNote: "Тестовая бета-версия · Установщик без цифровой подписи",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["EXE"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.4-beta-win-x64-test-setup.exe",
        version: "0.1.4-beta",
        architecture: "x64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/windows/0.1.4-beta/MeetSpace-0.1.4-beta-win-x64-test-setup.exe",
        sizeBytes: 121786956,
        sha256:
          "57f9577203eadc69b8a00a108e4c3e8bc9f81c1c34123827299c3b5e06f9feb8",
      },
      {
        name: "MeetSpace-0.1.4-beta-win-arm64-test-setup.exe",
        version: "0.1.4-beta",
        architecture: "ARM64",
        format: "EXE",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/windows/0.1.4-beta/MeetSpace-0.1.4-beta-win-arm64-test-setup.exe",
        sizeBytes: 116527659,
        sha256:
          "a1fe5618763919250251112ae1690bc1a4174b9802bb3a5601010730d7df0d5a",
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
      "https://github.com/janickiy/meet-space-desktop/tree/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta",
    buildNote: "Тестовая бета-версия · Проверьте требования пакета",
    targetArchitectures: ["x64", "ARM64"],
    targetFormats: ["DEB", "RPM", "AppImage"],
    supportedSystems: [],
    assets: [
      {
        name: "MeetSpace-0.1.4-beta-linux-amd64-test.deb",
        version: "0.1.4-beta",
        architecture: "x64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta/DEB/MeetSpace-0.1.4-beta-linux-amd64-test.deb",
        sizeBytes: 100476352,
        sha256:
          "02704649a1aad068be2624093746f9b9b36f32ae8a5e732f2c66b47e2f67a21d",
      },
      {
        name: "MeetSpace-0.1.4-beta-linux-x86_64-test.rpm",
        version: "0.1.4-beta",
        architecture: "x64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta/RPM/MeetSpace-0.1.4-beta-linux-x86_64-test.rpm",
        sizeBytes: 90090181,
        sha256:
          "1a198218e4c06a5c70aff9ad9de6e4046ad56fdbd8c49993fca4644f41a9a2c5",
      },
      {
        name: "MeetSpace-0.1.4-beta-linux-x86_64-test.AppImage",
        version: "0.1.4-beta",
        architecture: "x64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta/AppImage/MeetSpace-0.1.4-beta-linux-x86_64-test.AppImage",
        sizeBytes: 127060741,
        sha256:
          "cc7d42cd6161e2528a9234dc355d35ef495ca8265b6c4728799820a65894d50e",
      },
      {
        name: "MeetSpace-0.1.4-beta-linux-arm64-test.deb",
        version: "0.1.4-beta",
        architecture: "ARM64",
        format: "DEB",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta/DEB/MeetSpace-0.1.4-beta-linux-arm64-test.deb",
        sizeBytes: 95699808,
        sha256:
          "70eafcaa73b78e596cfb0794c28777119d86efac02868faeee4b66836933a8fb",
      },
      {
        name: "MeetSpace-0.1.4-beta-linux-aarch64-test.rpm",
        version: "0.1.4-beta",
        architecture: "ARM64",
        format: "RPM",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta/RPM/MeetSpace-0.1.4-beta-linux-aarch64-test.rpm",
        sizeBytes: 85830221,
        sha256:
          "e835e21141d2c8f0f4897581a75466c0f76d7110c84ebb39cb805fab6052deb7",
      },
      {
        name: "MeetSpace-0.1.4-beta-linux-arm64-test.AppImage",
        version: "0.1.4-beta",
        architecture: "ARM64",
        format: "AppImage",
        url: "https://github.com/janickiy/meet-space-desktop/raw/3e5a487298fb0fee517216809bcb151b51862e27/source/linux/0.1.4-beta/AppImage/MeetSpace-0.1.4-beta-linux-arm64-test.AppImage",
        sizeBytes: 119218986,
        sha256:
          "4f8b6bb93e9fca9f14b9ebe894a3225065abbc1d5ce001805a1ff430b2515ac5",
      },
    ],
  },
];
