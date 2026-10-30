import { useCallback, useState } from "react";
import { AlertCircle, ArrowDownToLine, Check, Loader2 } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "@multica/views/i18n";
import { SettingsCard, SettingsRow, SettingsTab } from "@multica/views/settings";

type CheckState =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "up-to-date" }
  | { status: "available"; latestVersion: string; sourceUrl: string }
  | { status: "error"; message: string };

export function UpdatesSettingsTab() {
  const { t } = useT("settings");
  const [state, setState] = useState<CheckState>({ status: "idle" });
  const currentVersion = window.desktopAPI.appInfo.version;

  const handleCheck = useCallback(async () => {
    setState({ status: "checking" });
    const result = await window.updater.checkForUpdates();
    if (!result.ok) {
      setState({ status: "error", message: result.error });
      return;
    }
    setState(
      result.available
        ? { status: "available", latestVersion: result.latestVersion, sourceUrl: result.sourceUrl }
        : { status: "up-to-date" },
    );
  }, []);

  return (
    <SettingsTab
      title={t(($) => $.desktop.updates.title)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.desktop.updates.current_version)}>
          <span className="font-mono text-caption text-muted-foreground">
            v{currentVersion}
          </span>
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.desktop.updates.check_section_title)}
          align="start"
          description={
            <>

              {state.status === "up-to-date" && (
                <p className="mt-2 inline-flex items-center gap-1.5">
                  <Check className="size-3.5 text-success" />
                  {t(($) => $.desktop.updates.up_to_date)}
                </p>
              )}
              {state.status === "available" && (
                <p className="mt-2 inline-flex items-center gap-1.5">
                  <ArrowDownToLine className="size-3.5 text-primary" />
                  {t(($) => $.desktop.updates.available, {
                    version: state.latestVersion,
                  })}
                  <span className="break-all">{state.sourceUrl}</span>
                  <Button variant="outline" size="sm" onClick={() => {
                    void window.updater.downloadUpdate().catch((error: unknown) => setState({ status: "error", message: error instanceof Error ? error.message : String(error) }));
                  }}>{t(($) => $.desktop.updates.download_confirm)}</Button>
                </p>
              )}
              {state.status === "error" && (
                <p className="mt-2 inline-flex items-center gap-1.5 text-destructive">
                  <AlertCircle className="size-3.5" />
                  {state.message}
                </p>
              )}
            </>
          }
        >
          <Button
            variant="outline"
            size="sm"
            onClick={handleCheck}
            disabled={state.status === "checking"}
          >
            {state.status === "checking" ? (
              <>
                <Loader2 className="size-3.5 animate-spin" />
                {t(($) => $.desktop.updates.checking)}
              </>
            ) : (
              t(($) => $.desktop.updates.check_now)
            )}
          </Button>
        </SettingsRow>
      </SettingsCard>
    </SettingsTab>
  );
}
