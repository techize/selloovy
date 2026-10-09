<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";
const props = defineProps<{ productId: number }>();
const emit = defineEmits<{ sessionExpired: []; saved: []; close: [] }>();
type Variant = {
  id: number;
  label: string;
  sizeLabel: string;
  colourPair: string;
  pricePence: number | null;
  supplyMode: string;
  stockQuantity: number;
  effectivePricePence: number;
  availability: string;
  dispatchDaysMin: number;
  dispatchDaysMax: number;
};
type Settings = {
  productId: number;
  productName: string;
  basePricePence: number;
  revision: number;
  madeToOrderFallback: boolean;
  variants: Variant[];
  shippingDaysMin: number;
  shippingDaysMax: number;
};
type Draft = {
  id: number;
  key: string;
  label: string;
  sizeLabel: string;
  colourPair: string;
  price: string;
  supplyMode: string;
  stock: string;
};
const data = ref<Settings | null>(null);
const drafts = ref<Draft[]>([]);
const fallback = ref(false);
const busy = ref(false);
const blocked = ref(false);
const message = ref("");
const success = ref("");
const fields = ref<Record<string, string>>({});
let controller: AbortController | undefined;
let active = true;
function valid(v: unknown): v is Settings {
  if (typeof v !== "object" || v === null) return false;
  const s = v as Settings;
  return (
    s.productId === props.productId &&
    typeof s.productName === "string" &&
    Number.isSafeInteger(s.basePricePence) &&
    s.basePricePence > 0 &&
    Number.isSafeInteger(s.revision) &&
    s.revision > 0 &&
    typeof s.madeToOrderFallback === "boolean" &&
    Array.isArray(s.variants) &&
    s.variants.length <= 50 &&
    s.variants.every(
      (v) =>
        v &&
        Number.isSafeInteger(v.id) &&
        v.id > 0 &&
        [v.label, v.sizeLabel, v.colourPair].every(
          (x) => typeof x === "string",
        ) &&
        (v.pricePence === null ||
          (Number.isSafeInteger(v.pricePence) && v.pricePence > 0)) &&
        ["stocked", "made_to_order"].includes(v.supplyMode) &&
        Number.isSafeInteger(v.stockQuantity) &&
        v.stockQuantity >= 0 &&
        Number.isSafeInteger(v.effectivePricePence) &&
        v.effectivePricePence > 0 &&
        ["in_stock", "made_to_order", "unavailable"].includes(v.availability) &&
        Number.isSafeInteger(v.dispatchDaysMin) &&
        Number.isSafeInteger(v.dispatchDaysMax),
    ) &&
    s.shippingDaysMin === 3 &&
    s.shippingDaysMax === 4
  );
}
async function request(method: string, body?: unknown) {
  const current = new AbortController();
  controller = current;
  const timer = setTimeout(() => current.abort(), 8000);
  try {
    const response = await fetch(
      `/api/admin/products/${props.productId}/maker`,
      {
        method,
        cache: "no-store",
        credentials: "same-origin",
        signal: current.signal,
        headers:
          body === undefined
            ? {}
            : {
                "Content-Type": "application/json",
                "X-Selloovy-Request": "owner-auth",
              },
        body: body === undefined ? undefined : JSON.stringify(body),
      },
    );
    const result: unknown = await response.json();
    if (active && response.status === 401) emit("sessionExpired");
    return { ok: response.ok, status: response.status, result };
  } finally {
    clearTimeout(timer);
  }
}
function apply(s: Settings) {
  data.value = s;
  fallback.value = s.madeToOrderFallback;
  drafts.value = s.variants.map((v) => ({
    id: v.id,
    key: String(v.id),
    label: v.label,
    sizeLabel: v.sizeLabel,
    colourPair: v.colourPair,
    price: v.pricePence === null ? "" : (v.pricePence / 100).toFixed(2),
    supplyMode: v.supplyMode,
    stock: String(v.stockQuantity),
  }));
  blocked.value = false;
  fields.value = {};
}
async function load() {
  if (busy.value) return;
  busy.value = true;
  message.value = "";
  success.value = "";
  try {
    const r = await request("GET");
    if (!active) return;
    if (!r.ok || !valid(r.result)) throw Error();
    apply(r.result);
  } catch {
    if (active)
      message.value = "Could not load variants. Your draft has been kept.";
  } finally {
    if (active) busy.value = false;
  }
}
function add() {
  if (drafts.value.length >= 50 || busy.value || blocked.value) return;
  drafts.value.push({
    id: 0,
    key: crypto.randomUUID(),
    label: "",
    sizeLabel: "",
    colourPair: "",
    price: "",
    supplyMode: "stocked",
    stock: "0",
  });
}
function pence(value: string): number | null | undefined {
  const v = value.trim();
  if (v === "") return null;
  if (!/^\d{1,7}(\.\d{1,2})?$/.test(v)) return undefined;
  const [a, b = ""] = v.split(".");
  return Number(a) * 100 + Number(b.padEnd(2, "0"));
}
function error(i: number, key: string) {
  return fields.value[`variants.${i}.${key}`] ?? "";
}
function money(value: number) {
  return `£${(value / 100).toFixed(2)}`;
}
function estimate(v: Variant) {
  return v.availability === "unavailable"
    ? "Unavailable · fallback off"
    : v.availability === "in_stock"
      ? "In stock · dispatch within 2 working days"
      : "Made to order · dispatch in 5–7 working days";
}
async function save() {
  if (!data.value || busy.value || blocked.value) return;
  fields.value = {};
  message.value = "";
  success.value = "";
  const variants = drafts.value.map((v, i) => {
    const price = pence(v.price);
    if (price === undefined)
      fields.value[`variants.${i}.pricePence`] =
        "Use a GBP price with up to two decimals, or leave blank.";
    if (!/^\d{1,7}$/.test(v.stock.trim()))
      fields.value[`variants.${i}.stockQuantity`] =
        "Enter a whole stock count.";
    return {
      id: v.id,
      label: v.label,
      sizeLabel: v.sizeLabel,
      colourPair: v.colourPair,
      pricePence: price ?? null,
      supplyMode: v.supplyMode,
      stockQuantity: Number(v.stock.trim()),
    };
  });
  if (Object.keys(fields.value).length) {
    message.value = "Check the highlighted fields.";
    return;
  }
  busy.value = true;
  try {
    const r = await request("PUT", {
      revision: data.value.revision,
      madeToOrderFallback: fallback.value,
      variants,
    });
    if (!active) return;
    if (r.status === 401) return;
    if (r.status === 422) {
      const b = r.result as { fields?: unknown };
      if (b && typeof b.fields === "object" && b.fields !== null) {
        for (const [k, v] of Object.entries(b.fields)) {
          if (typeof v === "string") fields.value[k] = v;
        }
      }
      message.value = "Check the highlighted fields.";
      return;
    }
    if (r.status === 409) {
      blocked.value = true;
      message.value =
        "This product changed in another tab. Reload saved variants before trying again.";
      return;
    }
    if (!r.ok || !valid(r.result)) throw Error();
    apply(r.result);
    success.value = "Variants and stock saved.";
    emit("saved");
  } catch {
    if (active) {
      blocked.value = true;
      message.value =
        "Your save could not be confirmed. Reload saved variants before trying again.";
    }
  } finally {
    if (active) busy.value = false;
  }
}
onMounted(() => void load());
onBeforeUnmount(() => {
  active = false;
  controller?.abort();
  data.value = null;
  drafts.value = [];
});
</script>
<template>
  <section
    class="card shop-settings maker-variants"
    aria-labelledby="maker-title"
  >
    <div class="card-top">
      <h2 id="maker-title">
        Variants &amp; stock<span v-if="data" class="maker-product-name">{{
          data.productName
        }}</span>
      </h2>
      <button class="secondary-button" :disabled="busy" @click="emit('close')">
        Back to products
      </button>
    </div>
    <p v-if="busy" role="status">Working…</p>
    <p v-if="message" class="settings-error" role="alert">{{ message }}</p>
    <p v-if="success" class="settings-success" role="status">{{ success }}</p>
    <form v-if="data" @submit.prevent="save">
      <fieldset :disabled="busy || blocked">
        <label class="maker-fallback"
          ><input v-model="fallback" type="checkbox" />Allow made-to-order
          fallback when a stocked variant sells out</label
        >
        <p class="settings-note">
          This applies only to {{ data.productName }}. Stock is counted
          separately for every variant and colour pair. Blank variant prices use
          {{ money(data.basePricePence) }}, the product price.
        </p>
        <p v-if="fields.variants" class="field-error" role="alert">
          {{ fields.variants }}
        </p>
        <details
          v-for="(v, i) in drafts"
          :key="v.key"
          class="variant-details"
          :open="
            !v.id ||
            Object.keys(fields).some((k) => k.startsWith(`variants.${i}.`))
          "
        >
          <summary>{{ v.label || `Variant ${i + 1}` }}</summary>
          <div class="variant-editor">
            <h3>Variant {{ i + 1 }}</h3>
            <div
              v-for="field in [
                {
                  key: 'label',
                  title: 'Variant name',
                  hint: 'Required · up to 120 characters',
                },
                {
                  key: 'sizeLabel',
                  title: 'Size',
                  hint: 'Optional · up to 80 characters',
                },
                {
                  key: 'colourPair',
                  title: 'Colour pair',
                  hint: 'Optional · up to 120 characters',
                },
                {
                  key: 'price',
                  title: 'Variant price (GBP)',
                  hint: 'Optional · blank uses product price',
                },
                {
                  key: 'stock',
                  title: 'Stock on hand',
                  hint: 'Whole count · 0 to 1,000,000',
                },
              ] as const"
              :key="field.key"
            >
              <label :for="`variant-${i}-${field.key}`"
                >{{ field.title }}<small>{{ field.hint }}</small></label
              ><input
                :id="`variant-${i}-${field.key}`"
                v-model="v[field.key]"
                :inputmode="
                  field.key === 'price'
                    ? 'decimal'
                    : field.key === 'stock'
                      ? 'numeric'
                      : undefined
                "
                :required="field.key === 'label' || field.key === 'stock'"
                :aria-invalid="
                  !!error(
                    i,
                    field.key === 'price'
                      ? 'pricePence'
                      : field.key === 'stock'
                        ? 'stockQuantity'
                        : field.key,
                  )
                "
                :aria-describedby="
                  error(
                    i,
                    field.key === 'price'
                      ? 'pricePence'
                      : field.key === 'stock'
                        ? 'stockQuantity'
                        : field.key,
                  )
                    ? `variant-${i}-${field.key}-error`
                    : undefined
                "
              />
              <p
                v-if="
                  error(
                    i,
                    field.key === 'price'
                      ? 'pricePence'
                      : field.key === 'stock'
                        ? 'stockQuantity'
                        : field.key,
                  )
                "
                :id="`variant-${i}-${field.key}-error`"
                class="field-error"
              >
                {{
                  error(
                    i,
                    field.key === "price"
                      ? "pricePence"
                      : field.key === "stock"
                        ? "stockQuantity"
                        : field.key,
                  )
                }}
              </p>
            </div>
            <label :for="`variant-${i}-mode`">Supply</label
            ><select
              :id="`variant-${i}-mode`"
              v-model="v.supplyMode"
              :aria-invalid="!!error(i, 'supplyMode')"
            >
              <option value="stocked">
                Stocked, with optional product fallback
              </option>
              <option value="made_to_order">Always made to order</option>
            </select>
            <p v-if="error(i, 'supplyMode')" class="field-error">
              {{ error(i, "supplyMode") }}
            </p>
            <p v-if="v.supplyMode === 'made_to_order'" class="settings-note">
              Use zero stock for an always-made-to-order variant.
            </p>
            <button
              v-if="!v.id"
              type="button"
              class="secondary-button"
              @click="drafts.splice(i, 1)"
            >
              Remove unsaved variant
            </button>
          </div>
        </details>
        <button
          type="button"
          class="secondary-button"
          :disabled="drafts.length >= 50"
          @click="add"
        >
          Add variant
        </button>
        <p class="settings-note">
          These are manual counts. Checkout stock holds and order deductions
          follow later. Saved variants cannot be removed here.
        </p>
        <button type="submit" class="auth-primary">
          Save variants &amp; stock
        </button>
      </fieldset>
    </form>
    <button
      v-if="!data || blocked"
      type="button"
      class="secondary-button"
      :disabled="busy"
      @click="load"
    >
      Reload saved variants
    </button>
    <section
      v-if="data && data.variants.length"
      class="saved-availability"
      aria-labelledby="availability-title"
    >
      <h3 id="availability-title">Saved availability</h3>
      <ul class="product-list">
        <li v-for="v in data.variants" :key="v.id">
          <div>
            <strong>{{ v.label }} · {{ money(v.effectivePricePence) }}</strong
            ><small>{{ estimate(v) }}</small
            ><small>Stock on hand: {{ v.stockQuantity }}</small>
          </div>
        </li>
      </ul>
      <p class="settings-note">
        Shipping then takes 3–4 working days. Estimates are launch defaults;
        shipping services and calendar dates follow later. Mixed baskets will
        ship together.
      </p>
    </section>
  </section>
</template>
