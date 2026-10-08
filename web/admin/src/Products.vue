<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";
const emit = defineEmits<{ sessionExpired: [] }>();
type Product = {
  id: number;
  name: string;
  description: string;
  pricePence: number;
  certificateName: string;
  revision: number;
};
const products = ref<Product[]>([]);
const nextAfter = ref(0);
const ready = ref(false);
const busy = ref(false);
const editing = ref<Product | null>(null);
const name = ref("");
const description = ref("");
const price = ref("");
const certificateName = ref("none");
const formOpen = ref(false);
const key = ref("");
const message = ref("");
const success = ref("");
const fields = ref<Record<string, string>>({});
const uncertain = ref(false);
const conflict = ref(false);
let pending: Record<string, unknown> | undefined;
let controller: AbortController | undefined;
function valid(v: unknown): v is Product {
  if (typeof v !== "object" || v === null) return false;
  const p = v as Product;
  return (
    Number.isSafeInteger(p.id) &&
    p.id > 0 &&
    typeof p.name === "string" &&
    typeof p.description === "string" &&
    Number.isSafeInteger(p.pricePence) &&
    p.pricePence > 0 &&
    ["none", "optional", "required"].includes(p.certificateName) &&
    Number.isSafeInteger(p.revision) &&
    p.revision > 0
  );
}
async function request(path: string, method = "GET", body?: unknown) {
  const current = new AbortController();
  controller = current;
  const timer = setTimeout(() => current.abort(), 8000);
  try {
    const response = await fetch("/api/admin/products" + path, {
      method,
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
    const result: unknown = await response.json();
    if (response.status === 401) emit("sessionExpired");
    return { ok: response.ok, status: response.status, result };
  } finally {
    clearTimeout(timer);
  }
}
function money(pence: number) {
  return `£${(pence / 100).toFixed(2)}`;
}
async function load(more = false) {
  if (busy.value) return;
  busy.value = true;
  message.value = "";
  try {
    const r = await request(more ? `/?after=${nextAfter.value}` : "/");
    const page = r.result as { products?: unknown; nextAfter?: unknown };
    if (
      !r.ok ||
      typeof page !== "object" ||
      page === null ||
      !Array.isArray(page.products) ||
      !page.products.every(valid) ||
      !Number.isSafeInteger(page.nextAfter) ||
      Number(page.nextAfter) < 0
    )
      throw Error();
    products.value = more
      ? [...products.value, ...page.products]
      : page.products;
    nextAfter.value = Number(page.nextAfter);
    ready.value = true;
  } catch {
    message.value = "Could not load products. Please try again.";
  } finally {
    busy.value = false;
  }
}
function open(p?: Product) {
  editing.value = p ? { ...p } : null;
  name.value = p?.name ?? "";
  description.value = p?.description ?? "";
  price.value = p ? (p.pricePence / 100).toFixed(2) : "";
  certificateName.value = p?.certificateName ?? "none";
  key.value = p ? "" : crypto.randomUUID();
  fields.value = {};
  message.value = "";
  success.value = "";
  uncertain.value = false;
  conflict.value = false;
  pending = undefined;
  formOpen.value = true;
}
async function reloadEdited() {
  if (!editing.value || busy.value) return;
  busy.value = true;
  try {
    const r = await request(`/${editing.value.id}`);
    if (!r.ok || !valid(r.result)) throw Error();
    const product = r.result;
    const index = products.value.findIndex((p) => p.id === product.id);
    if (index >= 0) products.value[index] = product;
    open(product);
  } catch {
    message.value = "Could not reload the product. Your draft is still here.";
  } finally {
    busy.value = false;
  }
}
async function save() {
  if (busy.value || conflict.value) return;
  fields.value = {};
  message.value = "";
  success.value = "";
  if (!uncertain.value) {
    const value = price.value.trim();
    if (!/^\d{1,7}(\.\d{1,2})?$/.test(value)) {
      fields.value.pricePence =
        "Enter a GBP price with up to two decimal places.";
      return;
    }
    const [whole, fraction = ""] = value.split(".");
    const pence = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
    pending = {
      name: name.value,
      description: description.value,
      pricePence: pence,
      certificateName: certificateName.value,
      revision: editing.value?.revision ?? 0,
      creationKey: editing.value ? "" : key.value,
    };
  }
  busy.value = true;
  try {
    const r = await request(
      editing.value ? `/${editing.value.id}` : "/",
      editing.value ? "PUT" : "POST",
      pending,
    );
    if (r.status === 401) return;
    if (r.status === 422) {
      const body = r.result as { fields?: unknown };
      if (body && typeof body.fields === "object" && body.fields !== null) {
        for (const [k, v] of Object.entries(body.fields)) {
          if (typeof v === "string") fields.value[k] = v;
        }
      }
      uncertain.value = false;
      message.value = "Check the highlighted fields.";
      return;
    }
    if (r.status === 409) {
      conflict.value = true;
      uncertain.value = false;
      message.value = editing.value
        ? "This product changed in another tab. Reload the saved product before continuing."
        : "This product may already have been created. Reload saved products before continuing.";
      return;
    }
    if (!r.ok || !valid(r.result)) throw Error();
    const p = r.result;
    editing.value = p;
    name.value = p.name;
    description.value = p.description;
    price.value = (p.pricePence / 100).toFixed(2);
    certificateName.value = p.certificateName;
    uncertain.value = false;
    pending = undefined;
    success.value = "Product draft saved. It is not visible to customers.";
    const index = products.value.findIndex((v) => v.id === p.id);
    if (index >= 0) products.value[index] = p;
    else products.value.push(p);
  } catch {
    if (editing.value) {
      conflict.value = true;
      message.value =
        "Your save could not be confirmed. Reload the saved product before continuing.";
    } else {
      uncertain.value = true;
      message.value =
        "We could not confirm your product was saved. Try again safely without creating a duplicate.";
    }
  } finally {
    busy.value = false;
  }
}
onMounted(() => void load());
onBeforeUnmount(() => {
  controller?.abort();
  products.value = [];
  pending = undefined;
  editing.value = null;
  name.value = "";
  description.value = "";
});
</script>
<template>
  <section class="card shop-settings" aria-labelledby="products-title">
    <div class="card-top">
      <h2 id="products-title">Your product drafts</h2>
      <button
        class="auth-primary"
        :disabled="busy || !ready || uncertain"
        @click="open()"
      >
        Add product
      </button>
    </div>
    <p class="settings-intro">
      Start with the name, story and price. Drafts stay private until publishing
      is available.
    </p>
    <p v-if="message" class="settings-error" role="alert">{{ message }}</p>
    <p v-if="success" class="settings-success" role="status">{{ success }}</p>
    <p v-if="busy" role="status">Working…</p>
    <button v-if="!ready && !busy" class="secondary-button" @click="load()">
      Try loading again
    </button>
    <form v-if="formOpen" @submit.prevent="save">
      <fieldset :disabled="busy || uncertain">
        <legend>
          {{ editing ? "Edit product draft" : "New product draft" }}
        </legend>
        <label for="product-name"
          >Product name <small>Required · up to 160 characters</small></label
        ><input
          id="product-name"
          v-model="name"
          required
          :aria-invalid="!!fields.name"
          :aria-describedby="fields.name ? 'product-name-error' : undefined"
        />
        <p v-if="fields.name" id="product-name-error" class="field-error">
          {{ fields.name }}
        </p>
        <label for="product-description"
          >Description <small>Plain text · up to 5,000 characters</small></label
        ><textarea
          id="product-description"
          v-model="description"
          rows="5"
          :aria-invalid="!!fields.description"
          :aria-describedby="
            fields.description ? 'product-description-error' : undefined
          "
        ></textarea>
        <p
          v-if="fields.description"
          id="product-description-error"
          class="field-error"
        >
          {{ fields.description }}
        </p>
        <label for="product-price"
          >Customer price (GBP)
          <small>Final price · no VAT added at this stage</small></label
        ><input
          id="product-price"
          v-model="price"
          type="text"
          inputmode="decimal"
          required
          placeholder="19.99"
          :aria-invalid="!!fields.pricePence"
          :aria-describedby="
            fields.pricePence ? 'product-price-error' : undefined
          "
        />
        <p
          v-if="fields.pricePence"
          id="product-price-error"
          class="field-error"
        >
          {{ fields.pricePence }}
        </p>
        <label for="product-certificate">Owner name on the certificate</label
        ><select
          id="product-certificate"
          v-model="certificateName"
          :aria-invalid="!!fields.certificateName"
          :aria-describedby="
            fields.certificateName ? 'product-certificate-error' : undefined
          "
        >
          <option value="none">No personalised certificate</option>
          <option value="optional">Optional customer name</option>
          <option value="required">Customer name required</option>
        </select>
        <p
          v-if="fields.certificateName"
          id="product-certificate-error"
          class="field-error"
        >
          {{ fields.certificateName }}
        </p>
        <p class="settings-note">
          This saves the product’s certificate requirement. Customer name
          collection, variants, photos, stock and made-to-order rules follow
          next.
        </p>
        <button type="submit" class="auth-primary" :disabled="busy || conflict">
          Save draft
        </button>
        <button
          type="button"
          class="secondary-button"
          :disabled="busy"
          @click="formOpen = false"
        >
          Close editor
        </button>
      </fieldset>
      <button
        v-if="uncertain"
        type="button"
        class="auth-primary"
        :disabled="busy"
        @click="save"
      >
        Try saving again
      </button>
      <button
        v-if="conflict && editing"
        type="button"
        class="secondary-button"
        :disabled="busy"
        @click="reloadEdited"
      >
        Reload saved product
      </button>
      <button
        v-if="conflict && !editing"
        type="button"
        class="secondary-button"
        :disabled="busy"
        @click="
          formOpen = false;
          load();
        "
      >
        Reload saved products
      </button>
    </form>
    <p v-if="ready && !products.length && !busy" class="settings-note">
      No products yet. Add your first creation.
    </p>
    <ul
      v-if="products.length"
      class="product-list"
      aria-label="Saved product drafts"
    >
      <li v-for="p in products" :key="p.id">
        <div>
          <strong>{{ p.name }}</strong
          ><small
            >Draft · {{ money(p.pricePence) }} ·
            {{
              p.certificateName === "none"
                ? "No personalised certificate"
                : p.certificateName === "required"
                  ? "Certificate name required"
                  : "Certificate name optional"
            }}</small
          >
        </div>
        <button
          class="secondary-button"
          :disabled="busy || uncertain"
          :aria-label="`Edit ${p.name}`"
          @click="open(p)"
        >
          Edit
        </button>
      </li>
    </ul>
    <button
      v-if="nextAfter"
      class="secondary-button"
      :disabled="busy || uncertain"
      @click="load(true)"
    >
      Load more products
    </button>
  </section>
</template>
