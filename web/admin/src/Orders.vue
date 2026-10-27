<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import DiscardPrompt from "./DiscardPrompt.vue";
import SaveFeedback from "./SaveFeedback.vue";
type Summary = {
  id: string;
  state: string;
  customerName: string;
  customerEmail: string;
  totalPence: number;
  createdAt: string;
  paidAt: string | null;
  needsAttention: boolean;
  notificationNeedsAttention: boolean;
  providerKnown: boolean;
  fulfilmentState: string;
};
type Customer = {
  name: string;
  email: string;
  address1: string;
  address2: string;
  town: string;
  postcode: string;
  country: string;
};
type Line = {
  ID: string;
  VariantID: number;
  ProductName: string;
  VariantLabel: string;
  Quantity: number;
  UnitPricePence: number;
  TotalPence: number;
  OwnerName: string;
};
type Snapshot = {
  Currency: string;
  Lines: Line[];
  Shipping: { name: string };
  ShippingPence: number;
  TotalPence: number;
};
type Fulfilment = {
  revision: number;
  state: string;
  carrier: string;
  trackingReference: string;
  trackingUrl: string;
  dispatchedAt: string | null;
};
type Detail = Summary & {
  customer: Customer;
  snapshot: Snapshot;
  fulfilment: Fulfilment | null;
  allocations: {
    variantId: number;
    physicalQuantity: number;
    productionQuantity: number;
  }[];
  notifications: { kind: string; state: string; needsAttention: boolean }[];
};
type Unfinished = {
  id: string;
  customer: Customer;
  snapshot: Snapshot;
  savedAt: string;
  abandoned: boolean;
};
type Page = {
  orders: Summary[];
  nextAfter: string;
  unfinished: Unfinished[];
  needsAttentionCount: number;
  notificationAttentionCount: number;
};
const emit = defineEmits<{ sessionExpired: [] }>();
const orders = ref<Summary[]>([]),
  unfinished = ref<Unfinished[]>([]),
  nextAfter = ref(""),
  detail = ref<Detail>();
const busy = ref(false),
  error = ref(""),
  success = ref(""),
  providerId = ref("");
const fields = ref<Record<string, string>>({});
const draft = ref({ carrier: "", trackingReference: "", trackingUrl: "" });
const discard = ref<InstanceType<typeof DiscardPrompt>>();
let controller: AbortController | undefined;
const hasUnsaved = computed(
  () =>
    !!detail.value?.fulfilment &&
    (["carrier", "trackingReference", "trackingUrl"] as const).some(
      (name) => draft.value[name] !== detail.value?.fulfilment?.[name],
    ),
);
defineExpose({ hasUnsaved });
type AttentionFilter = "" | "payments" | "notifications";
const attention = ref(0),
  notificationAttention = ref(0),
  onlyAttention = ref<AttentionFilter>("");
const money = (pence: number) =>
  new Intl.NumberFormat("en-GB", { style: "currency", currency: "GBP" }).format(
    pence / 100,
  );
const date = (value: string) =>
  new Intl.DateTimeFormat("en-GB", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Europe/London",
  }).format(new Date(value));
const states: Record<string, string> = {
  paid: "Paid",
  failed: "Declined",
  pending: "Checking",
  unknown: "Needs checking",
  submitting: "Submitting",
  awaiting_dispatch: "To prepare",
  dispatched: "Dispatched",
  queued: "Queued",
  sending: "Sending",
  sent: "Accepted for delivery",
};
const label = (state: string) => states[state] || state;
const confirmationStates: Record<string, { label: string; help: string }> = {
  queued: {
    label: "Queued — not sent",
    help: "Waiting for a configured email worker. Your paid order remains saved.",
  },
  sending: {
    label: "Request in progress",
    help: "A provider request started. This does not confirm email delivery.",
  },
  validated: {
    label: "Test validated — not sent",
    help: "Brevo drop mode validated the request without sending an email.",
  },
  sent: {
    label: "Accepted by email provider",
    help: "The provider accepted the request. Inbox delivery is not confirmed.",
  },
  failed: {
    label: "Email request rejected",
    help: "Review the provider configuration and activity. The payment is unchanged; do not charge again.",
  },
  unknown: {
    label: "Email outcome uncertain",
    help: "It may have been accepted. Check provider activity before any resend; automatic retries are disabled.",
  },
};
const confirmation = (state: string) =>
  confirmationStates[state] || {
    label: "Status unavailable",
    help: "Refresh and review provider activity before taking action.",
  };
function validSummary(v: unknown): v is Summary {
  const s = v as Summary;
  return (
    !!s &&
    /^[0-9a-f]{32}$/.test(s.id) &&
    ["paid", "failed", "pending", "unknown", "submitting"].includes(s.state) &&
    typeof s.customerName === "string" &&
    typeof s.customerEmail === "string" &&
    Number.isSafeInteger(s.totalPence) &&
    s.totalPence > 0 &&
    typeof s.createdAt === "string" &&
    !Number.isNaN(Date.parse(s.createdAt)) &&
    typeof s.needsAttention === "boolean" &&
    typeof s.notificationNeedsAttention === "boolean" &&
    typeof s.providerKnown === "boolean"
  );
}
function validPage(v: unknown): v is Page {
  const p = v as Page;
  return (
    !!p &&
    Array.isArray(p.orders) &&
    p.orders.length <= 50 &&
    p.orders.every(validSummary) &&
    typeof p.nextAfter === "string" &&
    (p.nextAfter === "" || /^[0-9a-f]{32}$/.test(p.nextAfter)) &&
    Array.isArray(p.unfinished) &&
    p.unfinished.length <= 50 &&
    Number.isSafeInteger(p.needsAttentionCount) &&
    p.needsAttentionCount >= 0 &&
    Number.isSafeInteger(p.notificationAttentionCount) &&
    p.notificationAttentionCount >= 0
  );
}
function validDetail(v: unknown): v is Detail {
  const d = v as Detail;
  return (
    validSummary(d) &&
    !!d.customer &&
    typeof d.customer.email === "string" &&
    !!d.snapshot &&
    d.snapshot.Currency === "GBP" &&
    Array.isArray(d.snapshot.Lines) &&
    d.snapshot.Lines.every(
      (line) =>
        typeof line.ProductName === "string" &&
        Number.isSafeInteger(line.Quantity) &&
        line.Quantity > 0,
    ) &&
    Array.isArray(d.allocations) &&
    Array.isArray(d.notifications) &&
    d.notifications.length <= 2 &&
    d.notifications.every(
      (n) =>
        ["customer_receipt", "owner_notification"].includes(n.kind) &&
        Object.hasOwn(confirmationStates, n.state) &&
        typeof n.needsAttention === "boolean",
    ) &&
    (d.fulfilment === null ||
      (!!d.fulfilment &&
        Number.isSafeInteger(d.fulfilment.revision) &&
        ["awaiting_dispatch", "dispatched"].includes(d.fulfilment.state)))
  );
}
async function request(path: string, method = "GET", body?: unknown) {
  const c = new AbortController();
  controller = c;
  const timeout = setTimeout(() => c.abort(), 8000);
  try {
    const response = await fetch("/api/admin/orders" + path, {
      method,
      credentials: "same-origin",
      cache: "no-store",
      signal: c.signal,
      headers:
        body === undefined
          ? {}
          : {
              "Content-Type": "application/json",
              "X-Selloovy-Request": "owner-auth",
            },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data: unknown = await response.json();
    if (response.status === 401) emit("sessionExpired");
    return { ok: response.ok, status: response.status, data };
  } finally {
    clearTimeout(timeout);
  }
}
function failure(data: unknown, fallback: string) {
  const value = data as { error?: unknown; fields?: Record<string, string> };
  error.value = typeof value?.error === "string" ? value.error : fallback;
  fields.value = value?.fields || {};
}
async function load(more = false, filter?: AttentionFilter) {
  if (busy.value || (hasUnsaved.value && !(await discard.value?.confirm())))
    return;
  busy.value = true;
  error.value = "";
  success.value = "";
  const requestedFilter = filter ?? onlyAttention.value;
  const query = new URLSearchParams();
  if (more) query.set("after", nextAfter.value);
  if (requestedFilter)
    query.set(
      "attention",
      requestedFilter === "payments" ? "1" : "notifications",
    );
  try {
    const response = await request(query.size ? "?" + query.toString() : "");
    if (!response.ok) {
      failure(response.data, "Orders could not be loaded.");
      return;
    }
    if (!validPage(response.data)) {
      error.value =
        "Order information could not be confirmed. Reload before making changes.";
      return;
    }
    onlyAttention.value = requestedFilter;
    orders.value = more
      ? [...orders.value, ...response.data.orders]
      : response.data.orders;
    unfinished.value = response.data.unfinished;
    attention.value = response.data.needsAttentionCount;
    notificationAttention.value = response.data.notificationAttentionCount;
    nextAfter.value = response.data.nextAfter;
    detail.value = undefined;
  } catch {
    error.value =
      "Orders could not be loaded. Check the connection and try again.";
  } finally {
    busy.value = false;
  }
}
function accept(order: Detail) {
  detail.value = order;
  draft.value = {
    carrier: order.fulfilment?.carrier || "",
    trackingReference: order.fulfilment?.trackingReference || "",
    trackingUrl: order.fulfilment?.trackingUrl || "",
  };
  providerId.value = "";
  fields.value = {};
  const i = orders.value.findIndex((o) => o.id === order.id);
  if (i >= 0) orders.value[i] = order;
}
async function open(id: string) {
  if (busy.value || (hasUnsaved.value && !(await discard.value?.confirm())))
    return;
  busy.value = true;
  error.value = "";
  success.value = "";
  try {
    const response = await request("/" + id);
    if (!response.ok) {
      failure(response.data, "Order unavailable.");
      return;
    }
    if (!validDetail(response.data)) {
      error.value = "Order information could not be confirmed.";
      return;
    }
    accept(response.data);
  } catch {
    error.value = "Order unavailable. Please try again.";
  } finally {
    busy.value = false;
  }
}
async function dispatch() {
  const current = detail.value;
  if (busy.value || !current?.fulfilment) return;
  busy.value = true;
  error.value = "";
  success.value = "";
  fields.value = {};
  try {
    const response = await request("/" + current.id + "/dispatch", "PUT", {
      revision: current.fulfilment.revision,
      ...draft.value,
    });
    if (!response.ok) {
      failure(response.data, "Dispatch details were not saved.");
      return;
    }
    if (!validDetail(response.data)) {
      error.value =
        "Save outcome could not be confirmed. Reload this order before trying again.";
      return;
    }
    accept(response.data);
    success.value =
      "The whole order is marked dispatched. Shipment details are saved.";
  } catch {
    error.value =
      "Save outcome could not be confirmed. Reload this order before trying again.";
  } finally {
    busy.value = false;
  }
}
async function reconcile() {
  const current = detail.value;
  if (busy.value || !current) return;
  busy.value = true;
  error.value = "";
  success.value = "";
  fields.value = {};
  try {
    const response = await request("/" + current.id + "/reconcile", "POST", {
      providerId: providerId.value.trim(),
    });
    if (!response.ok) {
      failure(response.data, "Square could not confirm this payment.");
      return;
    }
    const order = await request("/" + current.id);
    if (!order.ok || !validDetail(order.data)) {
      error.value = "Reload the order to see the reconciliation outcome.";
      return;
    }
    accept(order.data);
    success.value =
      order.data.state === "paid"
        ? "Square confirmed payment. The order can be prepared."
        : order.data.state === "failed"
          ? "Square confirmed the payment failed. No fulfilment should begin."
          : "Payment is still uncertain. Do not fulfil it or request another payment.";
  } catch {
    error.value =
      "Square check could not be confirmed. Reload the order; do not fulfil an uncertain payment.";
  } finally {
    busy.value = false;
  }
}
function variantName(id: number) {
  return (
    detail.value?.snapshot.Lines.find((line) => line.VariantID === id)
      ?.VariantLabel || "Saved variant"
  );
}
onMounted(() => load());
onBeforeUnmount(() => controller?.abort());
</script>

<template>
  <DiscardPrompt ref="discard" />
  <div class="order-tools">
    <button class="secondary-button" :disabled="busy" @click="load()">
      Refresh orders and checkouts</button
    ><button
      v-if="attention || onlyAttention === 'payments'"
      class="secondary-button"
      :disabled="busy"
      :aria-pressed="onlyAttention === 'payments'"
      @click="load(false, onlyAttention === 'payments' ? '' : 'payments')"
    >
      {{
        onlyAttention === "payments"
          ? "Show all attempts"
          : "Show payments needing attention"
      }}</button
    ><button
      v-if="notificationAttention || onlyAttention === 'notifications'"
      class="secondary-button"
      :disabled="busy"
      :aria-pressed="onlyAttention === 'notifications'"
      @click="
        load(false, onlyAttention === 'notifications' ? '' : 'notifications')
      "
    >
      {{
        onlyAttention === "notifications"
          ? "Show all attempts"
          : "Show email issues"
      }}</button
    ><span v-if="busy" role="status">Working…</span>
  </div>
  <SaveFeedback :error="error" :success="success" />
  <p v-if="attention" class="mfa-reminder" role="alert">
    {{ attention }} payment(s) need attention. Check Square before fulfilment;
    never ask the customer to pay again while an outcome is uncertain.
  </p>
  <p v-if="notificationAttention" class="mfa-reminder" role="alert">
    {{ notificationAttention }} order(s) have confirmation emails that need
    review. Email status does not change payment or dispatch status. Check
    provider activity before resending.
  </p>
  <section class="card order-block" aria-labelledby="orders-title">
    <h2 id="orders-title">Orders and payment attempts</h2>
    <p v-if="!orders.length && !busy">
      {{
        onlyAttention
          ? "No attempts match this filter."
          : "No payment attempts yet. Confirmed orders will appear here."
      }}
    </p>
    <div class="order-table-wrap" v-if="orders.length">
      <table class="order-table">
        <thead>
          <tr>
            <th>Customer</th>
            <th>Total</th>
            <th>Payment / preparation</th>
            <th>Created</th>
            <th>Details</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="order in orders" :key="order.id">
            <td>
              {{ order.customerName }}<small>{{ order.customerEmail }}</small>
            </td>
            <td>{{ money(order.totalPence) }}</td>
            <td>
              {{ label(order.state)
              }}<small v-if="order.fulfilmentState">{{
                label(order.fulfilmentState)
              }}</small
              ><small v-if="order.needsAttention">Payment needs attention</small
              ><small v-if="order.notificationNeedsAttention"
                >Confirmation email needs review</small
              >
            </td>
            <td>{{ date(order.createdAt) }}</td>
            <td>
              <button
                class="secondary-button"
                :disabled="busy"
                :aria-label="'Open order for ' + order.customerName"
                @click="open(order.id)"
              >
                Open
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <button
      v-if="nextAfter"
      class="secondary-button"
      :disabled="busy"
      @click="load(true)"
    >
      Load more payment attempts
    </button>
  </section>
  <section
    v-if="detail"
    class="card order-detail"
    aria-labelledby="order-title"
  >
    <h2 id="order-title">
      {{ label(detail.state) }} · {{ detail.customer.name }}
    </h2>
    <p class="muted">Reference {{ detail.id }}</p>
    <p v-if="detail.state !== 'paid'" class="availability" role="alert">
      This is not a paid order. Do not prepare or dispatch it until Square
      confirms payment.
    </p>
    <table class="order-table">
      <thead>
        <tr>
          <th>Creation</th>
          <th>Quantity</th>
          <th>Total</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="line in detail.snapshot.Lines" :key="line.ID">
          <td>
            {{ line.ProductName }}<small>{{ line.VariantLabel }}</small
            ><small v-if="line.OwnerName"
              >Certificate owner:
              <span class="certificate-name">{{ line.OwnerName }}</span></small
            >
          </td>
          <td>{{ line.Quantity }}</td>
          <td>{{ money(line.TotalPence) }}</td>
        </tr>
      </tbody>
    </table>
    <p>
      Delivery: {{ detail.snapshot.Shipping.name }} ·
      {{ money(detail.snapshot.ShippingPence) }}. All items ship together.
    </p>
    <p>
      <strong>Total {{ money(detail.totalPence) }}</strong
      ><span v-if="detail.paidAt"> · Paid {{ date(detail.paidAt) }}</span>
    </p>
    <h3>Customer and delivery</h3>
    <p>
      {{ detail.customer.name }} · {{ detail.customer.email }}<br />{{
        detail.customer.address1
      }}<br v-if="detail.customer.address2" /><span
        v-if="detail.customer.address2"
        >{{ detail.customer.address2 }}</span
      ><br />{{ detail.customer.town }}, {{ detail.customer.postcode }} ·
      {{ detail.customer.country }}
    </p>
    <template v-if="detail.state === 'paid'"
      ><h3>Items to prepare</h3>
      <p v-for="allocation in detail.allocations" :key="allocation.variantId">
        {{ variantName(allocation.variantId) }}:
        {{ allocation.physicalQuantity }} from stock,
        {{ allocation.productionQuantity }} to make.
      </p>
      <form
        v-if="detail.fulfilment"
        class="order-dispatch"
        @submit.prevent="dispatch"
      >
        <h3>
          {{
            detail.fulfilment.state === "dispatched"
              ? "Shipment details"
              : "Dispatch the whole order"
          }}
        </h3>
        <p v-if="detail.fulfilment.dispatchedAt">
          Dispatched {{ date(detail.fulfilment.dispatchedAt) }}. Updating
          tracking keeps the original dispatch time.
        </p>
        <p v-else>
          Prepare every item before marking the order dispatched. It will be
          recorded as one shipment.
        </p>
        <label for="order-carrier">Carrier (optional)</label
        ><input
          id="order-carrier"
          v-model="draft.carrier"
          maxlength="80"
          :disabled="busy"
        />
        <p v-if="fields.carrier" role="alert">{{ fields.carrier }}</p>
        <label for="order-tracking">Tracking reference (optional)</label
        ><input
          id="order-tracking"
          v-model="draft.trackingReference"
          maxlength="120"
          :disabled="busy"
        />
        <p v-if="fields.trackingReference" role="alert">
          {{ fields.trackingReference }}
        </p>
        <label for="order-tracking-url">HTTPS tracking link (optional)</label
        ><input
          id="order-tracking-url"
          v-model="draft.trackingUrl"
          type="url"
          maxlength="2048"
          :disabled="busy"
        />
        <p v-if="fields.trackingUrl" role="alert">{{ fields.trackingUrl }}</p>
        <button :disabled="busy">
          {{
            detail.fulfilment.state === "dispatched"
              ? "Save shipment details"
              : "Mark entire order dispatched"
          }}
        </button>
      </form>
      <h3>Confirmation delivery</h3>
      <p
        v-for="notice in detail.notifications"
        :key="notice.kind"
        class="confirmation-status"
      >
        {{
          notice.kind === "customer_receipt"
            ? "Customer confirmation"
            : "Owner notification"
        }}: {{ confirmation(notice.state).label }}.
        <small class="muted">{{
          notice.needsAttention && notice.state === "sending"
            ? "This request is taking longer than expected. Check provider activity before any resend."
            : confirmation(notice.state).help
        }}</small>
        <strong v-if="notice.needsAttention">Needs review.</strong>
      </p>
      <p class="muted">
        Your paid order remains saved regardless of email status. The current
        POC supports email request validation only; test validation does not
        send a message. No resend action is available here.
      </p>
    </template>
    <form
      v-else-if="detail.state !== 'failed'"
      class="order-dispatch"
      @submit.prevent="reconcile"
    >
      <h3>Check the payment with Square</h3>
      <p>
        The check reads Square’s saved payment. It never sends another charge.
      </p>
      <template v-if="!detail.providerKnown"
        ><p>
          Find the Square payment whose reference matches this order, then enter
          its payment ID. Selloovy verifies the location, reference, GBP amount
          and capture state.
        </p>
        <label for="square-payment-id">Square payment ID</label
        ><input
          id="square-payment-id"
          v-model="providerId"
          maxlength="80"
          :disabled="busy"
          required
        />
        <p v-if="fields.providerId" role="alert">
          {{ fields.providerId }}
        </p></template
      ><button :disabled="busy">Check saved Square payment</button>
    </form>
  </section>
  <section class="card order-block" aria-labelledby="unfinished-title">
    <h2 id="unfinished-title">Saved, unfinished checkouts</h2>
    <p>
      Latest 50 saved checkouts without a paid order or unresolved payment.
      Inactive for over an hour is shown as abandoned. No recovery emails are
      sent.
    </p>
    <p v-if="!unfinished.length && !busy">No saved unfinished checkouts.</p>
    <article
      v-for="checkout in unfinished"
      :key="checkout.id"
      class="unfinished-checkout"
    >
      <h3>
        {{ checkout.customer.name }} · {{ money(checkout.snapshot.TotalPence) }}
      </h3>
      <p>
        {{ checkout.customer.email }} ·
        {{ checkout.abandoned ? "Abandoned" : "Recently saved" }} ·
        {{ date(checkout.savedAt) }}
      </p>
      <ul>
        <li v-for="line in checkout.snapshot.Lines" :key="line.ID">
          {{ line.ProductName }} · {{ line.VariantLabel }} · {{ line.Quantity
          }}<span v-if="line.OwnerName">
            · Certificate: {{ line.OwnerName }}</span
          >
        </li>
      </ul>
    </article>
  </section>
</template>
