<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";
const props = defineProps<{ productId: number }>();
const emit = defineEmits<{ sessionExpired: []; saved: []; close: [] }>();
type State = {
  shopRevision: number;
  shop: { name: string; tagline: string; description: string };
  revision: number;
  publishedRevision: number;
  publicPath: string;
};
type Preview = {
  name: string;
  description: string;
  certificateName: string;
  revision: number;
};
type Maker = {
  revision: number;
  variants: {
    label: string;
    sizeLabel: string;
    colourPair: string;
    effectivePricePence: number;
  }[];
};
const state = ref<State>();
const preview = ref<Preview>();
const maker = ref<Maker>();
const photo = ref<{ id: string; alt: string }>();
const busy = ref(false);
const message = ref("");
const success = ref("");
const blocked = ref(false);
let controller: AbortController | undefined;
function validState(v: unknown): v is State {
  if (typeof v !== "object" || v === null) return false;
  const s = v as State;
  return (
    Number.isSafeInteger(s.revision) &&
    s.revision > 0 &&
    Number.isSafeInteger(s.shopRevision) &&
    s.shopRevision > 0 &&
    !!s.shop &&
    typeof s.shop.name === "string" &&
    typeof s.shop.tagline === "string" &&
    typeof s.shop.description === "string" &&
    Number.isSafeInteger(s.publishedRevision) &&
    s.publishedRevision >= 0 &&
    s.publishedRevision <= s.revision &&
    typeof s.publicPath === "string" &&
    (s.publishedRevision === 0
      ? s.publicPath === ""
      : new RegExp(
          "^/shop/[0-9a-f]{32}/products/" + props.productId + "$",
        ).test(s.publicPath))
  );
}
async function request(path: string, body?: unknown) {
  const current = new AbortController();
  controller = current;
  const timer = setTimeout(() => current.abort(), 8000);
  try {
    const r = await fetch("/api/admin/products/" + props.productId + path, {
      method: body === undefined ? "GET" : "PUT",
      credentials: "same-origin",
      cache: "no-store",
      signal: current.signal,
      headers:
        body === undefined
          ? {}
          : {
              "Content-Type": "application/json",
              "X-Selloovy-Request": "owner-auth",
            },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data: unknown = await r.json();
    if (r.status === 401) emit("sessionExpired");
    return { ok: r.ok, status: r.status, data };
  } finally {
    clearTimeout(timer);
  }
}
async function load() {
  if (busy.value) return;
  busy.value = true;
  message.value = "";
  success.value = "";
  blocked.value = true;
  try {
    const s = await request("/publication");
    if (!s.ok || !validState(s.data)) throw Error();
    const p = await request("");
    const m = await request("/maker");
    const product = p.data as Preview;
    const options = m.data as Maker;
    if (
      !p.ok ||
      !m.ok ||
      !product ||
      !options ||
      product.revision !== s.data.revision ||
      options.revision !== s.data.revision ||
      typeof product.name !== "string" ||
      typeof product.description !== "string" ||
      !["none", "optional", "required"].includes(product.certificateName) ||
      !Array.isArray(options.variants) ||
      !options.variants.every(
        (v) =>
          v &&
          typeof v.label === "string" &&
          typeof v.sizeLabel === "string" &&
          typeof v.colourPair === "string" &&
          Number.isSafeInteger(v.effectivePricePence) &&
          v.effectivePricePence > 0,
      )
    )
      throw Error();
    const f = await request("/photo");
    const image = f.data as {
      revision: number;
      photo: { id: string; alt: string };
    };
    if (
      !f.ok ||
      !image ||
      image.revision !== s.data.revision ||
      !image.photo ||
      typeof image.photo.id !== "string" ||
      typeof image.photo.alt !== "string"
    )
      throw Error();
    photo.value = image.photo;
    state.value = s.data;
    preview.value = product;
    maker.value = options;
    blocked.value = false;
  } catch {
    message.value =
      "Could not load a consistent product preview. Reload before publishing.";
  } finally {
    busy.value = false;
  }
}
async function save(publish: boolean) {
  if (busy.value || blocked.value || !state.value) return;
  busy.value = true;
  message.value = "";
  success.value = "";
  try {
    const r = await request("/publication", {
      revision: state.value.revision,
      shopRevision: state.value.shopRevision,
      publish,
    });
    if (r.status === 401) return;
    if (r.status === 409) {
      blocked.value = true;
      message.value =
        "The product or shop identity changed. Reload and review before continuing.";
      return;
    }
    if (r.status === 422) {
      message.value =
        "Save at least one variant in Variants & stock before publishing.";
      return;
    }
    if (!r.ok || !validState(r.data)) throw Error();
    state.value = r.data;
    success.value = publish
      ? "Product published. Open the public page below."
      : "Product unpublished. It is no longer available to browse.";
    emit("saved");
  } catch {
    blocked.value = true;
    message.value =
      "The change could not be confirmed. Reload publication status before trying again.";
  } finally {
    busy.value = false;
  }
}
function money(n: number) {
  return "£" + (n / 100).toFixed(2);
}
onMounted(() => void load());
onBeforeUnmount(() => {
  controller?.abort();
  state.value = undefined;
  preview.value = undefined;
  maker.value = undefined;
  photo.value = undefined;
});
</script>
<template>
  <section class="card shop-settings" aria-labelledby="publication-title">
    <div class="card-top">
      <h2 id="publication-title">Publish your creation</h2>
      <button class="secondary-button" :disabled="busy" @click="emit('close')">
        Back to products
      </button>
    </div>
    <p class="settings-intro">
      Review the saved content and options below. Publishing shares this product
      and your shop name, tagline and description. Your contact email stays
      private.
    </p>
    <p class="settings-note">
      Descriptions, prices and option labels stay as published until you
      republish. Stock, supply mode and made-to-order fallback update
      availability immediately. Cover photos are included when you publish. The
      basket follows next.
    </p>
    <p v-if="message" class="settings-error" role="alert">{{ message }}</p>
    <p v-if="success" class="settings-success" role="status">{{ success }}</p>
    <p v-if="busy" role="status">Working…</p>
    <template v-if="state && preview && maker">
      <p>
        <strong>{{
          state.publishedRevision ? "Published" : "Private draft"
        }}</strong
        ><span
          v-if="
            state.publishedRevision &&
            state.publishedRevision !== state.revision
          "
        >
          · Saved changes need republishing</span
        >
      </p>
      <h3>Shop identity</h3>
      <p>
        <strong>{{ state.shop.name }}</strong> · {{ state.shop.tagline }}
      </p>
      <p class="publication-description">{{ state.shop.description }}</p>
      <h3>{{ preview.name }}</h3>
      <img
        v-if="photo?.id"
        :key="photo.id"
        class="publication-photo"
        :src="'/api/admin/products/' + productId + '/photo/image'"
        :alt="photo.alt"
      />
      <p class="publication-description">{{ preview.description }}</p>
      <ul class="product-list">
        <li v-for="(v, index) in maker.variants" :key="index">
          <div>
            <strong>{{ v.label }} · {{ money(v.effectivePricePence) }}</strong
            ><small
              >{{ v.sizeLabel }}{{ v.sizeLabel && v.colourPair ? " · " : ""
              }}{{ v.colourPair }}</small
            >
          </div>
        </li>
      </ul>
      <p>
        {{
          preview.certificateName === "required"
            ? "Certificate owner name required"
            : preview.certificateName === "optional"
              ? "Certificate owner name optional"
              : "No personalised certificate"
        }}
      </p>
      <p v-if="!maker.variants.length" class="settings-note">
        Save at least one variant before publishing.
      </p>
      <div class="product-actions">
        <button
          class="auth-primary"
          :disabled="busy || blocked || !maker.variants.length"
          @click="save(true)"
        >
          {{
            state.publishedRevision
              ? "Republish reviewed product"
              : "Publish reviewed product"
          }}
        </button>
        <button
          v-if="state.publishedRevision"
          class="secondary-button"
          :disabled="busy || blocked"
          @click="save(false)"
        >
          Unpublish product
        </button>
      </div>
      <p v-if="state.publicPath">
        <a :href="state.publicPath" target="_blank" rel="noopener noreferrer"
          >Open public product page ↗</a
        >
      </p>
    </template>
    <button class="secondary-button" :disabled="busy" @click="load()">
      Reload preview and status
    </button>
  </section>
</template>
<style scoped>
.publication-photo {
  display: block;
  width: 100%;
  max-height: 360px;
  object-fit: contain;
  border-radius: 12px;
  margin: 16px 0;
}
.publication-description {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
