import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";
import { api, configureAuth, getAccessToken } from "../src/api.ts";

const originalFetch = globalThis.fetch;
let rejected;
beforeEach(() => {
  rejected = [];
  configureAuth("another-account-token", (token) => rejected.push(token));
});
afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});

test("LDAP login sends domain login only to its explicit endpoint with multi-account cookies", async () => {
  const controller = new AbortController();
  globalThis.fetch = async (url, options) => {
    assert.equal(url, "/api/v1/auth/ldap/login");
    assert.equal(options.method, "POST");
    assert.equal(options.credentials, "same-origin");
    assert.equal(options.headers.has("Authorization"), false);
    assert.equal(options.signal, controller.signal);
    assert.deepEqual(JSON.parse(options.body), {
      login: "alice",
      password: "synthetic-directory-password",
    });
    return new Response(JSON.stringify({ status: "success" }), { status: 200 });
  };
  await api.ldapLogin(
    "alice",
    "synthetic-directory-password",
    controller.signal,
  );
  assert.equal(getAccessToken(), "another-account-token");
});

test("LDAP credential failure does not fall back to local login or revoke another account", async () => {
  const paths = [];
  globalThis.fetch = async (url) => {
    paths.push(url);
    return new Response(JSON.stringify({ message: "invalid credentials" }), {
      status: 401,
    });
  };
  await assert.rejects(api.ldapLogin("alice", "wrong-password"));
  assert.deepEqual(paths, ["/api/v1/auth/ldap/login"]);
  assert.deepEqual(rejected, []);
  assert.equal(getAccessToken(), "another-account-token");
});

test("provider discovery is public and receives cancellation without exposing the current session", async () => {
  const controller = new AbortController();
  globalThis.fetch = async (url, options) => {
    assert.equal(url, "/api/v1/auth/providers");
    assert.equal(options.credentials, "omit");
    assert.equal(options.headers.has("Authorization"), false);
    assert.equal(options.signal, controller.signal);
    return new Response(
      JSON.stringify({
        mode: "ldap",
        registrationEnabled: false,
        passwordRecoveryEnabled: false,
        ldap: { enabled: true, label: "Корпоративный вход" },
      }),
    );
  };
  assert.equal((await api.authProviders(controller.signal)).ldap.enabled, true);
});

test("unified login sends email and password to the mode-aware endpoint", async () => {
  globalThis.fetch = async (url, options) => {
    assert.equal(url, "/api/v1/auth/login");
    assert.equal(options.method, "POST");
    assert.equal(options.credentials, "same-origin");
    assert.equal(options.headers.has("Authorization"), false);
    assert.deepEqual(JSON.parse(options.body), {
      email: "alice@example.test",
      password: "directory-password",
    });
    return new Response(JSON.stringify({ status: "success" }));
  };
  await api.login("alice@example.test", "directory-password");
  assert.equal(getAccessToken(), "another-account-token");
});

test("invalid or inconsistent auth policy fails closed", async () => {
  for (const policy of [
    {},
    { ldap: { enabled: true, label: "LDAP" } },
    {
      mode: "ldap",
      registrationEnabled: true,
      passwordRecoveryEnabled: false,
      ldap: { enabled: true, label: "LDAP" },
    },
    {
      mode: "ldap",
      registrationEnabled: false,
      passwordRecoveryEnabled: true,
      ldap: { enabled: true, label: "LDAP" },
    },
    {
      mode: "local",
      registrationEnabled: true,
      passwordRecoveryEnabled: true,
      ldap: { enabled: true, label: "LDAP" },
    },
  ]) {
    globalThis.fetch = async () => new Response(JSON.stringify(policy));
    await assert.rejects(api.authProviders(), /настройки входа/);
  }
});

test("local mode permits registration and recovery", async () => {
  const policy = {
    mode: "local",
    registrationEnabled: true,
    passwordRecoveryEnabled: true,
    ldap: { enabled: false, label: "Корпоративный вход" },
  };
  globalThis.fetch = async () => new Response(JSON.stringify(policy));
  assert.deepEqual(await api.authProviders(), policy);
});
