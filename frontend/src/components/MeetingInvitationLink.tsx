import { QRCodeSVG } from "qrcode.react";
import { inviteLink } from "../utils";
import { CopyLink } from "./ui";
import "./meeting-invitation-link.css";

/**
 * Display a copyable meeting invitation and encode the same URL locally as a QR code.
 * @param inviteCode The meeting's invitation code, resolved against the current origin.
 */
export function MeetingInvitationLink({ inviteCode }: { inviteCode: string }) {
  const link = inviteLink(inviteCode);
  return (
    <div className="meeting-share-invitation">
      <section
        className="meeting-share-link-section"
        aria-label="Ссылка на встречу"
      >
        <h4>Ссылка на встречу</h4>
        <p className="meeting-share-description">
          Поделитесь ссылкой или отсканируйте QR-код, чтобы присоединиться с
          другого устройства.
        </p>
        <CopyLink value={link} />
        <p className="field-hint">
          По ссылке можно присоединиться без аккаунта.
        </p>
      </section>
      <figure className="meeting-share-qr">
        <div className="meeting-share-qr-code">
          <QRCodeSVG
            value={link}
            size={160}
            level="M"
            marginSize={4}
            fgColor="#1f1924"
            bgColor="#ffffff"
            title="QR-код приглашения на встречу"
            role="img"
            aria-label="QR-код приглашения на встречу"
          />
        </div>
        <figcaption>
          Подключиться с телефона
          <span>Наведите камеру на QR-код</span>
        </figcaption>
      </figure>
    </div>
  );
}
