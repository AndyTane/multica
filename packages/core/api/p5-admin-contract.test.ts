// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";

// P5-R1 contract: the account-administration endpoints are consumed by installed
// desktop clients talking to newer servers, so every response passes a zod
// schema and degrades safely instead of throwing mid-flow. The delete preview in
// particular must FAIL CLOSED: a malformed body can never read as can_delete.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

function clientWith(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse(body));
  vi.stubGlobal("fetch", fetchMock);
  return { client: new ApiClient("https://api.example.test"), fetchMock };
}

describe("instance user administration contracts", () => {
  it("falls back to an empty user list when the payload is malformed", async () => {
    const { client } = clientWith({ users: "not-an-array" });
    await expect(client.listInstanceUsers()).resolves.toEqual({ users: [] });
  });

  it("fills defaults for optional user fields", async () => {
    const { client } = clientWith({
      users: [{ id: "u1", name: "User One", email: "one@example.test" }],
    });
    const result = await client.listInstanceUsers();
    expect(result.users).toHaveLength(1);
    expect(result.users[0]).toMatchObject({
      id: "u1",
      is_instance_admin: false,
      onboarded: false,
      has_password: false,
      invite_expires_at: null,
      created_at: "",
    });
  });

  it("does not invent an invitation secret when the field is missing", async () => {
    const { client } = clientWith({
      user: { id: "u1", name: "User One", email: "one@example.test" },
    });
    const result = await client.addInstanceUser({ name: "User One", email: "one@example.test" });
    expect(result.invite_token).toBe("");
    expect(result.invite_expires_at).toBe("");
    expect(result.user.id).toBe("u1");
  });

  it("treats a malformed registration response as no session", async () => {
    const { client } = clientWith({ token: 42, user: "nope" });
    const result = await client.redeemRegistrationInvite({
      token: "invite",
      name: "Name",
      username: "name",
      password: "password",
    });
    expect(result.token).toBe("");
    expect(result.user.id).toBe("");
  });

  it("defaults the open-registration switch to off when malformed", async () => {
    const { client } = clientWith({ open_registration_enabled: "yes" });
    await expect(client.getRegistrationSettings()).resolves.toEqual({
      open_registration_enabled: false,
    });
  });

  it("defaults a malformed password-change response to requiring re-auth", async () => {
    const { client } = clientWith({ reauth_required: "sure" });
    await expect(
      client.changeCurrentPassword({ current_password: "a", new_password: "b" }),
    ).resolves.toEqual({ reauth_required: true });
  });

  it("fails the delete preview closed on a malformed body", async () => {
    const { client } = clientWith({ can_delete: "yes", blockers: "none" });
    await expect(client.getInstanceUserDeletePreview("u1")).resolves.toEqual({
      can_delete: false,
      blockers: [],
      removes: [],
    });
  });

  it("keeps a well-formed delete preview intact", async () => {
    const payload = {
      can_delete: false,
      blockers: [{ code: "workspace_owner", count: 2 }],
      removes: ["account", "membership"],
    };
    const { client } = clientWith(payload);
    await expect(client.getInstanceUserDeletePreview("u1")).resolves.toEqual(payload);
  });
});
