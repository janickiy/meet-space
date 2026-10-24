import type { ReactNode } from "react";
import {
  DownloadSection,
  InstallSection,
  LandingFooter,
  LandingHeader,
  SelfHostedSection,
} from "./Landing";

function MarketingPage({ children }: { children: ReactNode }) {
  return (
    <div className="lp-site">
      <a className="lp-skip" href="#main">
        Перейти к содержимому
      </a>
      <LandingHeader />
      <main id="main" tabIndex={-1}>
        {children}
      </main>
      <LandingFooter />
    </div>
  );
}

export function DownloadPage() {
  return (
    <MarketingPage>
      <DownloadSection standalone />
    </MarketingPage>
  );
}

export function SelfHostedOverview() {
  return (
    <MarketingPage>
      <SelfHostedSection standalone />
      <InstallSection />
    </MarketingPage>
  );
}
