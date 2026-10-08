<script setup lang="ts">
import { ref, onBeforeUnmount } from "vue";
const emit = defineEmits<{ close: []; complete: [] }>();
const stage = ref<"password" | "scan" | "saved">("password");
const password = ref("");
const code = ref("");
const key = ref("");
const qr = ref("");
const codes = ref<string[]>([]);
const busy = ref(false);
const message = ref("");
const saved = ref(false);
let controller: AbortController | undefined;
async function submit() {
  if (busy.value) return;
  busy.value = true;
  message.value = "";
  controller = new AbortController();
  const timer = setTimeout(() => controller?.abort(), 8000);
  try {
    const response = await fetch(
      `/api/auth/mfa/${stage.value === "password" ? "start" : "confirm"}`,
      {
        method: "POST",
        credentials: "same-origin",
        cache: "no-store",
        signal: controller.signal,
        headers: {
          "Content-Type": "application/json",
          "X-Selloovy-Request": "owner-auth",
        },
        body: JSON.stringify(
          stage.value === "password"
            ? { password: password.value }
            : { code: code.value },
        ),
      },
    );
    password.value = "";
    code.value = "";
    if (!response.ok) {
      message.value =
        response.status === 429
          ? "Too many attempts. Wait 15 minutes before trying again."
          : response.status === 503
            ? "Security setup is temporarily unavailable."
            : "Check your details. If setup expired, start again.";
      return;
    }
    const data: unknown = await response.json();
    if (
      stage.value === "password" &&
      typeof data === "object" &&
      data !== null &&
      "key" in data &&
      typeof data.key === "string" &&
      "qr" in data &&
      typeof data.qr === "string" &&
      data.qr.startsWith("data:image/png;base64,")
    ) {
      key.value = data.key;
      qr.value = data.qr;
      stage.value = "scan";
    } else if (
      typeof data === "object" &&
      data !== null &&
      "recoveryCodes" in data &&
      Array.isArray(data.recoveryCodes) &&
      data.recoveryCodes.length === 10 &&
      data.recoveryCodes.every((x) => typeof x === "string")
    ) {
      codes.value = data.recoveryCodes;
      key.value = "";
      qr.value = "";
      stage.value = "saved";
    } else message.value = "Setup could not be completed. Please start again.";
  } catch {
    message.value =
      "Connection interrupted. Start again. If MFA was enabled, sign in with your authenticator.";
  } finally {
    clearTimeout(timer);
    password.value = "";
    code.value = "";
    busy.value = false;
  }
}
function restart() {
  stage.value = "password";
  key.value = "";
  qr.value = "";
  code.value = "";
  message.value = "";
}
onBeforeUnmount(() => {
  controller?.abort();
  password.value = "";
  key.value = "";
  qr.value = "";
  codes.value = [];
  code.value = "";
});
</script>
<template>
  <main class="security-layout">
    <a class="wordmark" href="/">selloovy<span>✦</span></a>
    <section class="security-card" aria-labelledby="security-title">
      <p class="eyebrow">Account security</p>
      <h1 id="security-title">
        {{
          stage === "saved" ? "MFA is enabled." : "A little extra protection."
        }}
      </h1>
      <template v-if="stage === 'saved'">
        <p>
          Save these ten recovery codes somewhere private. Each can be used
          once, alongside your password. They are shown only now.
        </p>
        <ul class="recovery-grid" aria-label="Recovery codes">
          <li v-for="item in codes" :key="item">
            <code>{{ item }}</code>
          </li>
        </ul>
        <p>
          Your previous sessions are signed out. Wait for a fresh authenticator
          code before signing in again.
        </p>
        <label class="saved-codes"
          ><input v-model="saved" type="checkbox" /> I have saved my recovery
          codes.</label
        >
        <button
          class="auth-primary"
          :disabled="!saved"
          @click="emit('complete')"
        >
          Sign in again
        </button>
      </template>
      <form v-else @submit.prevent="submit">
        <template v-if="stage === 'password'">
          <p>
            MFA is optional and recommended. Enter your current password to
            start. You’ll scan a QR code and confirm a code from your
            authenticator.
          </p>
          <label for="security-password">Current password</label
          ><input
            id="security-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            :disabled="busy"
          />
        </template>
        <template v-else>
          <p>
            Scan this QR code with your authenticator app, then enter its
            six-digit code. This setup expires after ten minutes.
          </p>
          <img
            class="enrollment-qr"
            :src="qr"
            alt="QR code to add your Selloovy account to an authenticator"
            width="320"
            height="320"
          />
          <details>
            <summary>Enter the key manually instead</summary>
            <code class="enrollment-key">{{ key }}</code>
            <p>Selloovy · SHA1 · six digits · 30 seconds</p>
          </details>
          <label for="security-code">Authenticator code</label
          ><input
            id="security-code"
            v-model="code"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            minlength="6"
            maxlength="6"
            required
            :disabled="busy"
          />
        </template>
        <p v-if="message" class="auth-error" role="alert">{{ message }}</p>
        <button class="auth-primary" type="submit" :disabled="busy">
          {{
            busy
              ? "Checking…"
              : stage === "password"
                ? "Show my QR code"
                : "Enable MFA"
          }}
        </button>
        <button
          v-if="stage === 'scan'"
          class="auth-link"
          type="button"
          :disabled="busy"
          @click="restart"
        >
          Start setup again
        </button>
        <button
          class="auth-link"
          type="button"
          :disabled="busy"
          @click="emit('close')"
        >
          Back to workspace
        </button>
      </form>
    </section>
  </main>
</template>
