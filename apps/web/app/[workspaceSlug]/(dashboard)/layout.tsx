"use client";

import { Suspense } from "react";
import { DashboardLayout } from "@multica/views/layout";
import { ProductMark } from "@multica/ui/components/common/product-mark";
import { SearchCommand, SearchTrigger } from "@multica/views/search";
import { FloatingChat } from "@multica/views/chat/floating-chat";
import { WebNotificationBridge } from "@/components/web-notification-bridge";
import { WorkspaceDocumentTitle } from "@/platform/workspace-document-title";

export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <>
      {/* useSearchParams requires a Suspense boundary in the app router. Sits
          outside DashboardLayout so the tab is named while the guard is still
          resolving the workspace. */}
      <Suspense fallback={null}>
        <WorkspaceDocumentTitle />
      </Suspense>
      <DashboardLayout
        loadingIndicator={<ProductMark name="U Workers" size={24} />}
        searchSlot={<SearchTrigger />}
        extra={
          <>
            <SearchCommand />
            <WebNotificationBridge />
            <FloatingChat />
          </>
        }
      >
        {children}
      </DashboardLayout>
    </>
  );
}
