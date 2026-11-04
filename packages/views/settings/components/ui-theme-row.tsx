"use client";

import { useSyncExternalStore } from "react";
import { SegmentedToggle } from "../../common/segmented-toggle";
import { useT } from "../../i18n";
import { SettingsRow } from "./settings-layout";

type UiTheme = "uworkers" | "classic";
const STORAGE_KEY = "multica-ui-theme";
const CHANGE_EVENT = "multica-ui-theme-change";

function applyTheme(value: UiTheme) {
  if (value === "classic") document.documentElement.setAttribute("data-ui-theme", value);
  else document.documentElement.removeAttribute("data-ui-theme");
}

function subscribe(onChange: () => void) {
  const onStorage = (event: StorageEvent) => {
    if (event.key !== STORAGE_KEY && event.key !== null) return;
    applyTheme(event.newValue === "classic" ? "classic" : "uworkers");
    onChange();
  };
  window.addEventListener("storage", onStorage);
  window.addEventListener(CHANGE_EVENT, onChange);
  return () => {
    window.removeEventListener("storage", onStorage);
    window.removeEventListener(CHANGE_EVENT, onChange);
  };
}

function getTheme(): UiTheme {
  return document.documentElement.getAttribute("data-ui-theme") === "classic" ? "classic" : "uworkers";
}

const getServerTheme = (): UiTheme => "uworkers";

export function UiThemeRow() {
  const { i18n } = useT("settings");
  const value = useSyncExternalStore(subscribe, getTheme, getServerTheme);
  const chinese = i18n.language.toLowerCase().startsWith("zh");
  const label = chinese ? "界面风格" : "Interface style";
  const onChange = (next: UiTheme) => {
    applyTheme(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Apply immediately even when device storage is unavailable.
    }
    window.dispatchEvent(new Event(CHANGE_EVENT));
  };

  return (
    <SettingsRow anchor="ui-theme" label={label}>
      <div role="group" aria-label={label}>
        <SegmentedToggle
          value={value}
          onChange={onChange}
          buttonClassName="px-3 py-1 text-label"
          options={[["uworkers", "UWorkers"], ["classic", chinese ? "原版" : "Classic"]]}
        />
      </div>
    </SettingsRow>
  );
}
