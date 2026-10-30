"use client";

import { useState } from "react";
import { api } from "@multica/core/api";
import { PRODUCT_NAME } from "@multica/core/config/brand";
import { ProductMark } from "@multica/ui/components/common/product-mark";
import { Button } from "@multica/ui/components/ui/button";
import { DragStrip } from "../../platform";
import { useT } from "../../i18n";

export function StepWelcome({ onNext, onSkip }: { onNext: () => void | Promise<void>; onSkip?: () => void | Promise<void> }) {
  const { t } = useT("onboarding");
  const [pending, setPending] = useState(false);
  async function run(action: () => void | Promise<void>) { setPending(true); try { await action(); } finally { setPending(false); } }
  return <div className="flex min-h-svh flex-col">
    <DragStrip />
    <main className="m-auto flex w-full max-w-lg flex-col gap-8 px-6 py-16">
      <ProductMark name={PRODUCT_NAME} variant="lockup" size={32} />
      <h1 className="text-display-sm font-semibold tracking-tight">{t(($) => $.service.title)}</h1>
      <div className="rounded-lg border p-4"><p className="mb-2 text-caption text-muted-foreground">{t(($) => $.service.address)}</p><p className="break-all font-mono text-body">{api.getBaseUrl() || "/"}</p></div>
      <Button size="lg" disabled={pending} onClick={() => void run(onNext)}>{t(($) => $.service.continue)}</Button>
      {onSkip && <Button variant="ghost" disabled={pending} onClick={() => void run(onSkip)}>{t(($) => $.welcome.skip_existing)}</Button>}
    </main>
  </div>;
}
