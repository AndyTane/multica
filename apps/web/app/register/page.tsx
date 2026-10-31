"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { ProductMark } from "@multica/ui/components/common/product-mark";
import { PRODUCT_NAME } from "@multica/core/config/brand";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { resolvePostAuthDestination } from "@multica/core/paths";
import { workspaceKeys } from "@multica/core/workspace/queries";
import {
  RegistrationPage,
  type RegistrationSuccess,
} from "@multica/views/auth";
import { setLoggedInCookie } from "@/features/auth/auth-cookie";

/**
 * Web registration entry.
 *
 * The shared page hands back the session the server just created; this route
 * finishes it exactly like the login page does — cookie marker, auth store,
 * workspace list seed — and then resolves the destination from whether the
 * account finished onboarding. An invited existing account lands in its own
 * workspace; a brand-new account goes through the normal onboarding.
 */
export default function Page() {
  const router = useRouter();
  const qc = useQueryClient();

  const handleSuccess = async ({ token, user }: RegistrationSuccess) => {
    setLoggedInCookie();
    useAuthStore.getState().adoptSession(token, user);
    const workspaces = await api.listWorkspaces().catch(() => []);
    qc.setQueryData(workspaceKeys.list(), workspaces);
    router.replace(
      resolvePostAuthDestination(workspaces, user.onboarded_at != null),
    );
  };

  return (
    <RegistrationPage
      logo={<ProductMark name={PRODUCT_NAME} size={32} />}
      onSuccess={(result) => void handleSuccess(result)}
    />
  );
}
