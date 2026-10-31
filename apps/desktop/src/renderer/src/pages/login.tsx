import { useState } from "react";
import { useConfigStore } from "@multica/core/config";
import { useAuthStore } from "@multica/core/auth";
import { LoginPage, RegistrationPage, type RegistrationSuccess } from "@multica/views/auth";
import { DragStrip } from "@multica/views/platform";
import { ProductMark } from "@multica/ui/components/common/product-mark";

function requireRuntimeAppUrl(): string {
  const runtimeConfig = window.desktopAPI.runtimeConfig;
  if (!runtimeConfig.ok) {
    throw new Error(
      "Invariant violated: DesktopLoginPage rendered before App accepted runtime config",
    );
  }
  return runtimeConfig.config.appUrl;
}

export function DesktopLoginPage() {
  const webUrl = requireRuntimeAppUrl();
  const googleClientId = useConfigStore((s) => s.googleClientId);
  const localPasswordLoginEnabled = useConfigStore(
    (s) => s.localPasswordLoginEnabled,
  );
  const emailCodeLoginEnabled = useConfigStore((s) => s.emailCodeLoginEnabled);
  const authConfigLoaded = useConfigStore((s) => s.authConfigLoaded);
  // Registration is a step INSIDE the login screen, not a route: the desktop
  // shell mounts this page until a session exists, and a new route would need
  // tab/overlay plumbing that a one-shot pre-workspace flow should not own.
  const [step, setStep] = useState<"login" | "register">("login");

  const handleGoogleLogin = () => {
    // Open web login page in the default browser with platform=desktop flag.
    // The web callback will redirect back via multica:// deep link with the token.
    window.desktopAPI.openExternal(
      `${webUrl}/login?platform=desktop`,
    );
  };

  const handleRegistered = ({ token, user }: RegistrationSuccess) => {
    // Publish the session the server just created. In desktop token mode this
    // also persists the bearer token; App re-renders into the shell because the
    // auth store now holds a user.
    useAuthStore.getState().adoptSession(token, user);
    setStep("login");
  };

  const registrationAvailable =
    authConfigLoaded && localPasswordLoginEnabled;

  return (
    <div className="flex h-screen flex-col">
      <DragStrip />
      {step === "register" && registrationAvailable ? (
        <RegistrationPage
          logo={<ProductMark name="U Workers" size={32} />}
          onSuccess={handleRegistered}
          onBack={() => setStep("login")}
        />
      ) : (
        <LoginPage
          logo={<ProductMark name="U Workers" size={32} />}
          onSuccess={() => {
            // Auth store update triggers AppContent re-render → shows DesktopShell.
            // Initial workspace navigation happens in routes.tsx via IndexRedirect.
          }}
          onGoogleLogin={googleClientId ? handleGoogleLogin : undefined}
          passwordLogin={localPasswordLoginEnabled}
          emailCodeLogin={emailCodeLoginEnabled}
          onRegister={
            registrationAvailable
              ? () => setStep("register")
              : undefined
          }
          configReady={authConfigLoaded}
        />
      )}
    </div>
  );
}
