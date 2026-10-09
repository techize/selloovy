<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";
import SaveFeedback from "./SaveFeedback.vue";
type Service = {
  id: string;
  name: string;
  feePence: number;
  daysMin: number;
  daysMax: number;
  freeFromPence: number | null;
  enabled: boolean;
};
type Settings = { revision: number; services: Service[] };
type Draft = Service & { fee: string; threshold: string };
const emit = defineEmits<{ sessionExpired: [] }>();
const state = ref<Settings>();
const drafts = ref<Draft[]>([]);
const busy = ref(false);
const blocked = ref(true);
const error = ref("");
const success = ref("");
let controller: AbortController | undefined;
function valid(v: unknown): v is Settings {
  const s = v as Settings;
  return (
    !!s &&
    Number.isSafeInteger(s.revision) &&
    s.revision > 0 &&
    Array.isArray(s.services) &&
    s.services.length <= 10 &&
    s.services.every(
      (x) =>
        x &&
        /^[0-9a-f]{32}$/.test(x.id) &&
        typeof x.name === "string" &&
        Number.isSafeInteger(x.feePence) &&
        x.feePence >= 0 &&
        Number.isInteger(x.daysMin) &&
        Number.isInteger(x.daysMax) &&
        typeof x.enabled === "boolean" &&
        (x.freeFromPence === null || Number.isSafeInteger(x.freeFromPence)),
    )
  );
}
function accept(s: Settings) {
  state.value = s;
  drafts.value = s.services.map((x) => ({
    ...x,
    fee: (x.feePence / 100).toFixed(2),
    threshold:
      x.freeFromPence === null ? "" : (x.freeFromPence / 100).toFixed(2),
  }));
}
async function request(body?: Settings) {
  const c = new AbortController();
  controller = c;
  const timer = setTimeout(() => c.abort(), 8000);
  try {
    const r = await fetch("/api/admin/shipping", {
      method: body ? "PUT" : "GET",
      credentials: "same-origin",
      cache: "no-store",
      signal: c.signal,
      headers: body
        ? {
            "Content-Type": "application/json",
            "X-Selloovy-Request": "owner-auth",
          }
        : {},
      body: body ? JSON.stringify(body) : undefined,
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
  blocked.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await request();
    if (!r.ok || !valid(r.data)) throw Error();
    accept(r.data);
    blocked.value = false;
  } catch {
    error.value = "Could not load shipping services. Reload before saving.";
  } finally {
    busy.value = false;
  }
}
function money(v: string): number | undefined {
  v = v.trim();
  if (!/^\d{1,7}(\.\d{1,2})?$/.test(v)) return;
  const [a, b = ""] = v.split(".");
  return Number(a) * 100 + Number(b.padEnd(2, "0"));
}
function add() {
  if (busy.value || blocked.value || drafts.value.length >= 10) return;
  success.value = "";
  drafts.value.push({
    id: crypto.randomUUID().replaceAll("-", ""),
    name: "",
    feePence: 0,
    fee: "4.99",
    daysMin: 3,
    daysMax: 5,
    freeFromPence: null,
    threshold: "",
    enabled: true,
  });
}
function starter() {
  if (busy.value || blocked.value || drafts.value.length) return;
  const examples = [
    {
      name: "Standard delivery",
      fee: "4.99",
      daysMin: 3,
      daysMax: 5,
      threshold: "30.00",
    },
    { name: "Tracked 48", fee: "5.99", daysMin: 2, daysMax: 3, threshold: "" },
    { name: "Tracked 24", fee: "8.99", daysMin: 1, daysMax: 2, threshold: "" },
  ];
  drafts.value = examples.map((x) => ({
    ...x,
    id: crypto.randomUUID().replaceAll("-", ""),
    feePence: 0,
    freeFromPence: null,
    enabled: true,
  }));
  success.value =
    "Starter services added to your draft. Review and save to apply them.";
}
async function save() {
  if (busy.value || blocked.value || !state.value) return;
  error.value = "";
  success.value = "";
  const services: Service[] = [];
  for (const d of drafts.value) {
    const fee = money(d.fee);
    const threshold = d.threshold.trim() === "" ? null : money(d.threshold);
    if (fee === undefined || threshold === undefined) {
      error.value =
        "Enter charges and thresholds in GBP with up to two decimal places.";
      return;
    }
    services.push({
      id: d.id,
      name: d.name,
      feePence: fee,
      daysMin: Number(d.daysMin),
      daysMax: Number(d.daysMax),
      freeFromPence: threshold,
      enabled: d.enabled,
    });
  }
  busy.value = true;
  try {
    const r = await request({ revision: state.value.revision, services });
    if (r.status === 401) return;
    if (r.status === 422) {
      error.value =
        (r.data as { fields?: { services?: string } }).fields?.services ??
        "Check the shipping services.";
      return;
    }
    if (r.status === 409) {
      blocked.value = true;
      error.value =
        "Shop settings changed. Reload shipping services before saving.";
      return;
    }
    if (!r.ok || !valid(r.data)) throw Error();
    accept(r.data);
    success.value =
      "Shipping services saved. Customer baskets use these rates and estimates.";
  } catch {
    blocked.value = true;
    error.value =
      "The save could not be confirmed. Reload before trying again.";
  } finally {
    busy.value = false;
  }
}
onMounted(() => void load());
onBeforeUnmount(() => controller?.abort());
</script>
<template>
  <section class="card shop-settings" aria-labelledby="shipping-title">
    <h2 id="shipping-title">UK shipping services</h2>
    <p class="settings-intro">
      Set the services customers can choose, their final GBP charges and transit
      times after dispatch. All enabled services currently cover all UK
      addresses.
    </p>
    <p class="settings-note">
      Free delivery uses the product subtotal, excluding shipping. To make
      standard delivery free from £30, enter 30.00 on that service; leave
      upgrades blank. Transit is counted in working days after production, using
      the England and Wales holiday calendar.
    </p>
    <SaveFeedback v-if="!state" :error="error" :success="success" />
    <form v-if="state" @submit.prevent="save">
      <fieldset :disabled="busy || blocked">
        <legend>Your delivery choices</legend>
        <div v-for="(d, i) in drafts" :key="d.id" class="shipping-service">
          <h3>Service {{ i + 1 }}</h3>
          <label :for="'service-name-' + d.id">Service name</label
          ><input :id="'service-name-' + d.id" v-model="d.name" required />
          <label :for="'service-fee-' + d.id">Customer charge (GBP)</label
          ><input
            :id="'service-fee-' + d.id"
            v-model="d.fee"
            inputmode="decimal"
            required
          />
          <label :for="'service-free-' + d.id"
            >Free from product subtotal (GBP, optional)</label
          ><input
            :id="'service-free-' + d.id"
            v-model="d.threshold"
            inputmode="decimal"
          />
          <label :for="'service-min-' + d.id"
            >Minimum transit working days</label
          ><input
            :id="'service-min-' + d.id"
            v-model.number="d.daysMin"
            type="number"
            min="1"
            max="30"
            required
          />
          <label :for="'service-max-' + d.id"
            >Maximum transit working days</label
          ><input
            :id="'service-max-' + d.id"
            v-model.number="d.daysMax"
            type="number"
            min="1"
            max="30"
            required
          />
          <label class="available"
            ><input v-model="d.enabled" type="checkbox" />Available to
            customers</label
          >
          <button
            type="button"
            class="secondary-button"
            @click="drafts.splice(i, 1)"
          >
            Remove service {{ i + 1 }}
          </button>
        </div>
        <p v-if="!drafts.length">
          No services configured. Add your first shipping service or use the
          editable UK starter services.
        </p>
        <button
          type="button"
          class="secondary-button"
          :disabled="drafts.length >= 10"
          @click="add"
        >
          Add shipping service
        </button>
        <button
          v-if="!drafts.length"
          type="button"
          class="secondary-button"
          @click="starter"
        >
          Use UK starter services
        </button>
        <SaveFeedback :error="error" :success="success" />
        <button type="submit" class="auth-primary">
          {{ busy ? "Saving…" : "Save shipping services" }}
        </button>
      </fieldset>
    </form>
    <button class="secondary-button" :disabled="busy" @click="load">
      Reload saved shipping services
    </button>
  </section>
</template>
<style scoped>
.shipping-service {
  border-bottom: 1px solid #dce1d8;
  padding: 20px 0;
  margin-bottom: 16px;
}
.available {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 16px 0;
}
.available input {
  width: auto;
  margin: 0;
}
</style>
