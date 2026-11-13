import { Banner, Card, Page, PageHead } from "@ac/web/components/ui";

/** FR-C20 in the Phase 1A demo: an installation request is stored by the Core API and decided by HQ, so the demo keeps no
requests (the live app lists, creates and cancels them). */
export function InstallationsDemo() {
  return (
    <Page>
      <PageHead title="Installation requests" sub="Ask HQ to install new air conditioners, with or without an IoT device." />
      <Card title="Not in the demo">
        <Banner>Installation requests are stored by the Core API and decided by HQ, so the demo keeps none. Sign in to the live app to send one.</Banner>
      </Card>
    </Page>
  );
}
