<script setup lang="ts">
import ProductPublication from "./ProductPublication.vue";
import SaveFeedback from "./SaveFeedback.vue";
import { ref, computed, onMounted, onBeforeUnmount } from "vue";
const props = defineProps<{ productId: number }>();
const emit = defineEmits<{ sessionExpired: []; saved: []; close: [] }>();
type Settings = {
  revision: number;
  photo: { id: string; alt: string; width: number; height: number };
};
const state = ref<Settings>();
const reviewOpen = ref(false);
const savedDraft = computed(
  () => !!state.value && !file.value && alt.value === state.value.photo.alt,
);
const alt = ref("");
const file = ref<File>();
const preview = ref("");
const input = ref<HTMLInputElement>();
const busy = ref(false);
const blocked = ref(true);
const message = ref("");
const success = ref("");
let controller: AbortController | undefined;
const path = computed(
  () => "/api/admin/products/" + props.productId + "/photo",
);
function valid(v: unknown): v is Settings {
  if (!v || typeof v !== "object") return false;
  const s = v as Settings;
  return (
    Number.isSafeInteger(s.revision) &&
    s.revision > 0 &&
    !!s.photo &&
    typeof s.photo.id === "string" &&
    (s.photo.id === "" || /^[0-9a-f]{32}$/.test(s.photo.id)) &&
    typeof s.photo.alt === "string" &&
    Number.isSafeInteger(s.photo.width) &&
    Number.isSafeInteger(s.photo.height)
  );
}
function clearFile(resetInput = true) {
  if (preview.value) URL.revokeObjectURL(preview.value);
  preview.value = "";
  file.value = undefined;
  if (resetInput && input.value) input.value.value = "";
}
function choose(event: Event) {
  const selected = (event.target as HTMLInputElement).files?.[0];
  clearFile(false);
  message.value = "";
  success.value = "";
  if (!selected) return;
  if (
    !["image/jpeg", "image/png"].includes(selected.type) ||
    selected.size > 5 * 1024 * 1024
  ) {
    message.value = "Choose a JPEG or PNG up to 5 MiB.";
    return;
  }
  file.value = selected;
  preview.value = URL.createObjectURL(selected);
}
function review() {
  if (busy.value || !savedDraft.value) return;
  message.value = "";
  reviewOpen.value = true;
}
async function request(method: string, body?: BodyInit) {
  const current = new AbortController();
  controller = current;
  const timer = setTimeout(() => current.abort(), 15000);
  try {
    const response = await fetch(path.value, {
      method,
      credentials: "same-origin",
      cache: "no-store",
      signal: current.signal,
      headers:
        body === undefined
          ? {}
          : method === "DELETE"
            ? {
                "Content-Type": "application/json",
                "X-Selloovy-Request": "owner-auth",
              }
            : { "X-Selloovy-Request": "owner-auth" },
      body,
    });
    const data: unknown = await response.json();
    if (response.status === 401) emit("sessionExpired");
    return { ok: response.ok, status: response.status, data };
  } finally {
    clearTimeout(timer);
  }
}
async function load() {
  if (busy.value) return;
  busy.value = true;
  blocked.value = true;
  message.value = "";
  success.value = "";
  try {
    const r = await request("GET");
    if (!r.ok || !valid(r.data)) throw Error();
    state.value = r.data;
    alt.value = r.data.photo.alt;
    clearFile();
    blocked.value = false;
  } catch {
    message.value = "Could not load the photo. Reload before saving.";
  } finally {
    busy.value = false;
  }
}
async function save(remove = false) {
  if (busy.value || blocked.value || !state.value) return;
  busy.value = true;
  message.value = "";
  success.value = "";
  try {
    let body: BodyInit;
    if (remove) body = JSON.stringify({ revision: state.value.revision });
    else {
      const form = new FormData();
      form.append("revision", String(state.value.revision));
      form.append("alt", alt.value);
      if (file.value) form.append("photo", file.value);
      body = form;
    }
    const r = await request(remove ? "DELETE" : "PUT", body);
    if (r.status === 401) return;
    if (r.status === 409) {
      blocked.value = true;
      message.value = "This product changed. Reload before saving the photo.";
      return;
    }
    if (r.status === 422 || r.status === 400) {
      const data = r.data as {
        error?: string;
        fields?: Record<string, string>;
      };
      message.value =
        data?.fields?.alt ??
        data?.fields?.photo ??
        data?.error ??
        "Check the photo and alt text.";
      return;
    }
    if (!r.ok || !valid(r.data)) throw Error();
    state.value = r.data;
    alt.value = r.data.photo.alt;
    clearFile();
    success.value = remove
      ? "Draft photo removed. Republish to remove it from the public page."
      : "Draft photo saved. Review and republish to update the public page.";
    emit("saved");
  } catch {
    blocked.value = true;
    message.value =
      "The photo change could not be confirmed. Reload before trying again.";
  } finally {
    busy.value = false;
  }
}
onMounted(() => void load());
onBeforeUnmount(() => {
  controller?.abort();
  clearFile();
  state.value = undefined;
  alt.value = "";
});
</script>
<template>
  <section class="card shop-settings" aria-labelledby="photo-title">
    <div class="card-top">
      <h2 id="photo-title">Product cover photo</h2>
      <button class="secondary-button" :disabled="busy" @click="emit('close')">
        Back to products
      </button>
    </div>
    <p class="settings-intro">
      Add one clear cover photo and describe it for customers using a screen
      reader. Save it here, then review and publish the product.
    </p>
    <p class="settings-note">
      JPEG or PNG · up to 5 MiB · 4,096 pixels per side · 8 megapixels. Embedded
      metadata is removed. More gallery photos follow later.
    </p>
    <SaveFeedback v-if="!state" :error="message" :success="success" />
    <p v-if="busy" role="status">Working…</p>
    <form v-if="state" @submit.prevent="save()">
      <fieldset :disabled="busy || blocked || reviewOpen">
        <legend>Draft cover photo</legend>
        <img
          v-if="preview"
          class="photo-preview"
          :src="preview"
          :alt="alt || 'Selected photo preview'"
        />
        <img
          v-else-if="state.photo.id"
          :key="state.photo.id"
          class="photo-preview"
          :src="path + '/image'"
          :alt="state.photo.alt"
        />
        <p v-else>No cover photo yet.</p>
        <label for="photo-file">{{
          state.photo.id ? "Replace photo (optional)" : "Choose a photo"
        }}</label
        ><input
          id="photo-file"
          ref="input"
          type="file"
          accept="image/jpeg,image/png"
          @change="choose"
        />
        <label for="photo-alt"
          >Photo description (alt text)<small
            >Required · 1–160 characters</small
          ></label
        ><input id="photo-alt" v-model="alt" required />
        <SaveFeedback :error="message" :success="success" />
        <button
          type="submit"
          class="auth-primary"
          :disabled="!file && !state.photo.id"
        >
          {{ busy ? "Saving…" : "Save draft photo" }}
        </button>
        <button
          v-if="state.photo.id"
          type="button"
          class="secondary-button"
          @click="save(true)"
        >
          Remove draft photo
        </button>
        <button
          type="button"
          class="secondary-button"
          :disabled="!savedDraft"
          @click="review"
        >
          Review &amp; publish
        </button>
        <p v-if="!savedDraft" class="settings-note">
          Save the photo changes before reviewing for publication.
        </p>
      </fieldset>
    </form>
    <ProductPublication
      v-if="reviewOpen"
      :product-id="productId"
      embedded
      @session-expired="emit('sessionExpired')"
      @saved="emit('saved')"
      @close="
        reviewOpen = false;
        load();
      "
    />
    <button
      class="secondary-button"
      :disabled="busy || reviewOpen"
      @click="load()"
    >
      Reload saved photo
    </button>
  </section>
</template>
<style scoped>
.photo-preview {
  display: block;
  max-width: 100%;
  width: 100%;
  max-height: 400px;
  height: auto;
  object-fit: contain;
  border-radius: 12px;
  margin: 16px 0;
}
</style>
