export type Status = "active" | "blocked";
export interface Administrator {
  id: string;
  username: string;
  name: string;
  email: string | null;
  status: Status;
  createdAt: string;
  updatedAt: string;
  lastLoginAt: string | null;
}
export interface AdminSession {
  administrator: Administrator;
  csrfToken: string;
}
export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
export interface User {
  id: string;
  name: string;
  email: string | null;
  status: Status;
  createdAt: string;
}
export interface Conference {
  id: string;
  title: string;
  ownerName: string;
  status: string;
  createdAt: string;
  startedAt: string | null;
  endedAt: string | null;
  participantCount: number | null;
  recordingStatus: string | null;
}
export interface Recording {
  id: string;
  title: string;
  conferenceId: string | null;
  conferenceTitle: string | null;
  status: string;
  durationSeconds: number | null;
  sizeBytes: number | null;
  createdAt: string;
  canDelete: boolean;
}
export interface ServiceHealth {
  name: string;
  status: string;
  latencyMs: number | null;
  version: string | null;
  checkedAt: string | null;
  message?: string;
}
export interface Dashboard {
  users: {
    total: number | null;
    active: number | null;
    blocked: number | null;
    new: number | null;
  };
  conferences: {
    total: number | null;
    active: number | null;
    completed: number | null;
    participants: number | null;
  };
  recordings: {
    total: number | null;
    active: number | null;
    bytes: number | null;
  };
  files: {
    count: number | null;
    bytes: number | null;
    capacityBytes: number | null;
  };
  services: ServiceHealth[];
  activity?: { date: string; count: number }[];
}
export interface FileSummary {
  count: number | null;
  bytes: number | null;
  capacityBytes: number | null;
  categories: { name: string; count: number | null; bytes: number | null }[];
  scope?: string;
}
export interface ChatSummary {
  directChats: number | null;
  groupChats: number | null;
  conferenceChats: number | null;
  messages: number | null;
  attachments: number | null;
}
export interface SystemSummary {
  services: ServiceHealth[];
  resources: {
    cpuPercent: number | null;
    memoryUsedBytes: number | null;
    memoryTotalBytes: number | null;
    diskUsedBytes: number | null;
    diskTotalBytes: number | null;
  };
}
export interface Setting {
  key: string;
  label: string;
  description: string;
  value?: string | number | boolean | null;
  editable: boolean;
  type: "string" | "number" | "boolean" | "secret";
  group: string;
  secret: boolean;
  configured: boolean;
  min?: number;
  max?: number;
  step?: number;
  options?: string[];
}
export interface Settings {
  items: Setting[];
  revision: number;
}
export interface AuditEvent {
  id: string;
  administratorUsername: string | null;
  action: string;
  objectType: string;
  objectId: string | null;
  ip: string;
  userAgent: string;
  metadata: Record<string, unknown> | null;
  createdAt: string;
}
