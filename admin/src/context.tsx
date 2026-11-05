import { createContext, useContext } from "react";
import type { ReactNode } from "react";
import type {
  AdminSession,
  Administrator,
  AuditEvent,
  Conference,
  Recording,
} from "./types";

export type DialogRequest =
  | {
      kind: "confirm";
      title: string;
      body: ReactNode;
      label: string;
      danger?: boolean;
      run: (signal: AbortSignal) => Promise<void>;
      success: string;
    }
  | { kind: "administrator"; administrator?: Administrator }
  | { kind: "password"; administrator: Administrator }
  | { kind: "profile" }
  | { kind: "conference"; conference: Conference }
  | { kind: "recording"; recording: Recording }
  | { kind: "audit"; event: AuditEvent };

export interface AdminContextValue {
  session: AdminSession;
  openDialog: (dialog: DialogRequest) => void;
  notify: (message: string) => void;
  logout: (signal?: AbortSignal) => Promise<void>;
}
export const AdminContext = createContext<AdminContextValue | null>(null);
export function useAdmin() {
  const value = useContext(AdminContext);
  if (!value) throw new Error("Admin context is missing");
  return value;
}
