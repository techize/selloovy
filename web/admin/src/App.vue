<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from "vue";
import Workspace from "./Workspace.vue";
import MFASetup from "./MFASetup.vue";
type Mode =
  | "checking"
  | "login"
  | "mfa"
  | "signedin"
  | "disabled"
  | "unavailable"
  | "security";
const mode = ref<Mode>("checking");
const mfaEnabled = ref(false);
const email = ref("");
const password = ref("");
const code = ref("");
const recovery = ref(false);
const busy = ref(false);
const message = ref("");
let request: AbortController | undefined;
async function api(path: string, body?: unknown) {
  request?.abort();
  const current = new AbortController();
  request = current;
  const timer = setTimeout(() => current.abort(), 8000);
  try {
    const response = await fetch(`/api/auth/${path}`, {
      method: body === undefined ? "GET" : "POST",
      credentials: "same-origin",
      cache: "no-store",
      headers:
        body === undefined
          ? {}
          : {
              "Content-Type": "application/json",
              "X-Selloovy-Request": "owner-auth",
            },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: current.signal,
    });
    const data: unknown = response.ok ? await response.json() : null;
    return { status: response.status, ok: response.ok, data };
  } finally {
    clearTimeout(timer);
  }
}
async function checkSession() {
  try {
    const response = await api("status");
    if (response.status === 404) {
      mode.value = "disabled";
      return;
    }
    if (!response.ok) {
      mode.value = "unavailable";
      return;
    }
    const data: unknown = response.data;
    if (
      typeof data !== "object" ||
      data === null ||
      !("authenticated" in data) ||
      typeof data.authenticated !== "boolean"
    ) {
      mode.value = "unavailable";
      return;
    }
    mode.value = data.authenticated ? "signedin" : "login";
    mfaEnabled.value = "mfaEnabled" in data && data.mfaEnabled === true;
  } catch {
    mode.value = "unavailable";
  }
}
async function submit() {
  if (busy.value) return;
  busy.value = true;
  message.value = "";
  try {
    const response =
      mode.value === "login"
        ? await api("login", { email: email.value, password: password.value })
        : await api("verify", { code: code.value, recovery: recovery.value });
    password.value = "";
    code.value = "";
    if (!response.ok) {
      message.value =
        response.status === 429
          ? "Too many attempts. Please wait 15 minutes before trying again."
          : response.status === 503
            ? "Sign-in is temporarily unavailable. Please try again later."
            : "Sign-in could not be completed. Check your details and try again.";
      return;
    }
    const data: unknown = response.data;
    if (
      mode.value === "login" &&
      typeof data === "object" &&
      data !== null &&
      "step" in data &&
      data.step === "mfa"
    )
      mode.value = "mfa";
    else if (
      typeof data === "object" &&
      data !== null &&
      "authenticated" in data &&
      data.authenticated === true
    ) {
      mode.value = "signedin";
      mfaEnabled.value = "mfaEnabled" in data && data.mfaEnabled === true;
      email.value = "";
    } else
      message.value = "Sign-in could not be completed. Please start again.";
  } catch {
    message.value = "Connection interrupted. Please start sign-in again.";
    mode.value = "login";
  } finally {
    password.value = "";
    code.value = "";
    busy.value = false;
  }
}
async function logout() {
  if (busy.value) return;
  busy.value = true;
  try {
    const response = await api("logout", {});
    if (response.ok) {
      mode.value = "login";
      message.value = "";
    } else {
      mode.value = "unavailable";
    }
  } catch {
    mode.value = "unavailable";
  } finally {
    busy.value = false;
  }
}
function startAgain() {
  mode.value = "login";
  code.value = "";
  password.value = "";
  recovery.value = false;
  message.value = "";
}
function onFocus() {
  if (mode.value === "signedin" && !busy.value) void checkSession();
}
onMounted(() => {
  void checkSession();
  window.addEventListener("focus", onFocus);
});
onBeforeUnmount(() => {
  request?.abort();
  window.removeEventListener("focus", onFocus);
  password.value = "";
  code.value = "";
});
</script>
<template>
  <Workspace
    v-if="mode === 'signedin' || mode === 'disabled'"
    :authenticated="mode === 'signedin'"
    :mfa-enabled="mfaEnabled"
    @enable-mfa="mode = 'security'"
    @logout="logout"
  />
  <MFASetup
    v-else-if="mode === 'security'"
    @close="checkSession"
    @complete="
      mode = 'login';
      message = '';
    "
  />
  <main v-else class="auth-layout">
    <section class="auth-story" aria-label="Selloovy">
      <a class="wordmark" href="/">selloovy<span>✦</span></a>
      <p class="eyebrow">Made for makers</p>
      <h1>More time<br />to make.</h1>
      <p>Your pieces. Your story. Your shop.<br />Selling, smoothly.</p>
      <div class="auth-flower" aria-hidden="true">✳</div>
      <small>Open source. Yours to host.</small>
    </section>
    <section class="auth-panel" aria-labelledby="auth-title">
      <div class="auth-card">
        <template v-if="mode === 'checking'"
          ><h2 id="auth-title">Opening your workspace…</h2>
          <p role="status">Checking your session.</p></template
        >
        <template v-else-if="mode === 'unavailable'"
          ><h2 id="auth-title">We couldn’t connect.</h2>
          <p>Sign-in is temporarily unavailable. Please try again.</p>
          <button class="auth-primary" @click="checkSession">
            Try again
          </button></template
        >
        <form v-else @submit.prevent="submit">
          <p class="eyebrow">Owner workspace</p>
          <h2 id="auth-title">
            {{
              mode === "login"
                ? "Welcome back."
                : recovery
                  ? "Use a recovery code."
                  : "One more step."
            }}
          </h2>
          <p>
            {{
              mode === "login"
                ? "Sign in to your Selloovy workspace."
                : recovery
                  ? "Enter one unused recovery code saved during setup."
                  : "Enter the six-digit code from your authenticator app."
            }}
          </p>
          <template v-if="mode === 'login'">
            <label for="owner-email">Email address</label
            ><input
              id="owner-email"
              v-model="email"
              type="text"
              inputmode="email"
              autocomplete="username"
              required
              maxlength="254"
              :disabled="busy"
            />
            <label for="owner-password">Password</label
            ><input
              id="owner-password"
              v-model="password"
              type="password"
              autocomplete="current-password"
              required
              :disabled="busy"
            />
          </template>
          <template v-else>
            <label for="owner-code">{{
              recovery ? "Recovery code" : "Authenticator code"
            }}</label
            ><input
              id="owner-code"
              v-model="code"
              type="text"
              :inputmode="recovery ? 'text' : 'numeric'"
              :autocomplete="recovery ? 'off' : 'one-time-code'"
              required
              :maxlength="recovery ? 37 : 6"
              :minlength="recovery ? 32 : 6"
              :disabled="busy"
              :key="recovery ? 'recovery' : 'totp'"
            />
          </template>
          <p v-if="message" class="auth-error" role="alert">{{ message }}</p>
          <button class="auth-primary" type="submit" :disabled="busy">
            {{
              busy
                ? "Checking…"
                : mode === "login"
                  ? "Continue"
                  : "Open workspace"
            }}
          </button>
          <template v-if="mode === 'mfa'"
            ><button
              class="auth-link"
              type="button"
              :disabled="busy"
              @click="
                recovery = !recovery;
                code = '';
                message = '';
              "
            >
              {{
                recovery
                  ? "Use my authenticator instead"
                  : "Use a recovery code"
              }}</button
            ><button
              class="auth-link"
              type="button"
              :disabled="busy"
              @click="startAgain"
            >
              Start sign-in again
            </button></template
          >
          <p class="auth-help" v-else>
            First time here? Your installation operator can create the owner
            account. You can add an authenticator later.
          </p>
        </form>
        <div class="auth-footnote">
          A little less friction. A little more making.
        </div>
      </div>
    </section>
  </main>
</template>
