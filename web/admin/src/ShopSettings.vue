<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";
const emit = defineEmits<{ sessionExpired: []; loaded: [name: string] }>();
type Settings = {
  name: string;
  tagline: string;
  description: string;
  contactEmail: string;
  currencyCode: string;
  countryCode: string;
  timezone: string;
  revision: number;
};
const data = ref<Settings | null>(null);
const busy = ref(false);
const loading = ref(true);
const message = ref("");
const success = ref("");
const fields = ref<Record<string, string>>({});
const conflict = ref(false);
let controller: AbortController | undefined;
function valid(value: unknown): value is Settings {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    [
      "name",
      "tagline",
      "description",
      "contactEmail",
      "currencyCode",
      "countryCode",
      "timezone",
    ].every((k) => typeof v[k] === "string") &&
    typeof v.revision === "number" &&
    Number.isSafeInteger(v.revision) &&
    v.revision > 0
  );
}
async function request(method: string, body?: unknown) {
  controller?.abort();
  controller = new AbortController();
  const timer = setTimeout(() => controller?.abort(), 8000);
  try {
    const response = await fetch("/api/admin/shop", {
      method,
      credentials: "same-origin",
      cache: "no-store",
      signal: controller.signal,
      headers:
        body === undefined
          ? {}
          : {
              "Content-Type": "application/json",
              "X-Selloovy-Request": "owner-auth",
            },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const result: unknown = await response.json();
    return { status: response.status, ok: response.ok, result };
  } finally {
    clearTimeout(timer);
  }
}
async function load() {
  loading.value = true;
  message.value = "";
  success.value = "";
  fields.value = {};
  try {
    const r = await request("GET");
    if (r.status === 401) {
      emit("sessionExpired");
      return;
    }
    if (!r.ok || !valid(r.result)) {
      message.value = "Shop settings are temporarily unavailable.";
      return;
    }
    data.value = r.result;
    conflict.value = false;
    emit("loaded", r.result.name);
  } catch {
    message.value = "Could not load your shop. Please try again.";
  } finally {
    loading.value = false;
  }
}
async function save() {
  if (!data.value || busy.value) return;
  busy.value = true;
  message.value = "";
  success.value = "";
  fields.value = {};
  const d = data.value;
  try {
    const r = await request("PUT", {
      name: d.name,
      tagline: d.tagline,
      description: d.description,
      contactEmail: d.contactEmail,
      revision: d.revision,
    });
    if (r.status === 401) {
      emit("sessionExpired");
      return;
    }
    if (r.status === 409) {
      conflict.value = true;
      message.value =
        "Your shop changed in another tab. Reload the saved details before trying again.";
      return;
    }
    if (
      r.status === 422 &&
      typeof r.result === "object" &&
      r.result !== null &&
      "fields" in r.result &&
      typeof r.result.fields === "object" &&
      r.result.fields !== null
    ) {
      for (const [k, v] of Object.entries(r.result.fields)) {
        if (typeof v === "string") fields.value[k] = v;
      }
      message.value = "Check the highlighted fields.";
      return;
    }
    if (!r.ok || !valid(r.result)) {
      message.value =
        "Your save could not be confirmed. Reload the saved details before trying again.";
      conflict.value = true;
      return;
    }
    data.value = r.result;
    success.value = "Shop details saved.";
    emit("loaded", r.result.name);
  } catch {
    message.value =
      "Connection interrupted. Reload the saved details before trying again.";
    conflict.value = true;
  } finally {
    busy.value = false;
  }
}
onMounted(() => void load());
onBeforeUnmount(() => {
  controller?.abort();
  data.value = null;
});
</script>
<template>
  <section class="card shop-settings" aria-labelledby="shop-settings-title">
    <p class="eyebrow">Shop identity</p>
    <h2 id="shop-settings-title">Make it yours.</h2>
    <p class="settings-intro">Your name, story and customer contact details.</p>
    <p v-if="loading" role="status">Loading your shop…</p>
    <p v-if="message" class="settings-error" role="alert">{{ message }}</p>
    <p v-if="success" class="settings-success" role="status">{{ success }}</p>
    <form v-if="data && !loading" @submit.prevent="save">
      <fieldset :disabled="busy">
        <label for="shop-name"
          >Shop name <small>Required · up to 120 characters</small></label
        ><input
          id="shop-name"
          v-model="data.name"
          type="text"
          required
          :aria-invalid="!!fields.name"
          :aria-describedby="fields.name ? 'name-error' : undefined"
        />
        <p v-if="fields.name" id="name-error" class="field-error">
          {{ fields.name }}
        </p>
        <label for="shop-tagline"
          >Tagline <small>Optional · up to 160 characters</small></label
        ><input
          id="shop-tagline"
          v-model="data.tagline"
          type="text"
          :aria-invalid="!!fields.tagline"
          :aria-describedby="fields.tagline ? 'tagline-error' : undefined"
        />
        <p v-if="fields.tagline" id="tagline-error" class="field-error">
          {{ fields.tagline }}
        </p>
        <label for="shop-description"
          >Your shop’s story
          <small>Optional · plain text, up to 2,000 characters</small></label
        ><textarea
          id="shop-description"
          v-model="data.description"
          rows="5"
          :aria-invalid="!!fields.description"
          :aria-describedby="
            fields.description ? 'description-error' : undefined
          "
        ></textarea>
        <p v-if="fields.description" id="description-error" class="field-error">
          {{ fields.description }}
        </p>
        <label for="shop-email"
          >Customer contact email <small>Optional</small></label
        ><input
          id="shop-email"
          v-model="data.contactEmail"
          type="text"
          inputmode="email"
          autocomplete="email"
          :aria-invalid="!!fields.contactEmail"
          :aria-describedby="fields.contactEmail ? 'email-error' : undefined"
        />
        <p v-if="fields.contactEmail" id="email-error" class="field-error">
          {{ fields.contactEmail }}
        </p>
        <div class="shop-defaults" aria-label="UK launch defaults">
          <div>
            <span>Country</span
            ><strong>{{
              data.countryCode === "GB" ? "United Kingdom" : data.countryCode
            }}</strong>
          </div>
          <div>
            <span>Currency</span><strong>{{ data.currencyCode }}</strong>
          </div>
          <div>
            <span>Time zone</span><strong>{{ data.timezone }}</strong>
          </div>
        </div>
        <p class="settings-note">
          These shop details stay in admin for now. Storefront publishing, tax
          and delivery settings follow as the shop develops.
        </p>
        <button class="auth-primary" type="submit" :disabled="busy || conflict">
          {{ busy ? "Saving…" : "Save shop details" }}
        </button>
      </fieldset>
    </form>
    <button
      v-if="!loading && (conflict || !data)"
      class="secondary-button"
      :disabled="busy"
      @click="load"
    >
      Reload saved details
    </button>
  </section>
</template>
