<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import Products from "./Products.vue";
import ShopSettings from "./ShopSettings.vue";

const props = defineProps<{ authenticated: boolean; mfaEnabled: boolean }>();
const emit = defineEmits<{ logout: []; enableMfa: []; sessionExpired: [] }>();

const sections = [
  {
    id: "overview",
    name: "Overview",
    symbol: "◈",
    title: "Your shop, taking shape.",
    description: "A little less friction. A little more time to make.",
  },
  {
    id: "products",
    name: "Products",
    symbol: "◇",
    title: "Made by you.",
    description:
      "A place for stocked pieces, made-to-order creations and personal touches.",
  },
  {
    id: "pages",
    name: "Pages",
    symbol: "▤",
    title: "Tell your story.",
    description:
      "Your homepage and information pages will grow from reusable sections.",
  },
  {
    id: "orders",
    name: "Orders",
    symbol: "▱",
    title: "From basket to doorstep.",
    description: "Orders, payments and fulfilment will come together here.",
  },
  {
    id: "settings",
    name: "Settings",
    symbol: "⚙",
    title: "Make it your own.",
    description:
      "Your shop identity, delivery choices and account settings will live here.",
  },
] as const;

type SectionId = (typeof sections)[number]["id"];
type Health = "checking" | "ready" | "not_ready" | "unreachable";
const selected = ref<SectionId>("overview");
const shopName = ref("Maker workspace");
const active = computed(() =>
  sections.find((section) => section.id === selected.value)!,
);
const health = ref<Health>("checking");
const healthText = computed(
  () =>
    ({
      checking: "Checking connection",
      ready: "Database ready",
      not_ready: "Database not ready",
      unreachable: "Connection unavailable",
    })[health.value],
);
let request: AbortController | undefined;

async function checkConnection() {
  request?.abort();
  const current = new AbortController();
  request = current;
  health.value = "checking";
  const timeout = setTimeout(() => current.abort(), 3000);
  try {
    const response = await fetch("/health/ready", {
      signal: current.signal,
      cache: "no-store",
    });
    const body: unknown = await response.json();
    if (request !== current) return;
    health.value =
      response.status === 200 &&
      typeof body === "object" &&
      body !== null &&
      "status" in body &&
      body.status === "ready"
        ? "ready"
        : "not_ready";
  } catch {
    if (request === current) health.value = "unreachable";
  } finally {
    clearTimeout(timeout);
  }
}

onMounted(checkConnection);
onBeforeUnmount(() => {
  request?.abort();
  request = undefined;
});
</script>

<template>
  <a class="skip-link" href="#workspace">Skip to workspace</a>
  <div class="layout">
    <aside class="sidebar">
      <a class="wordmark" href="/" aria-label="Selloovy storefront"
        >selloovy<span>✦</span></a
      >
      <div class="shop-card">
        <span class="shop-initial" aria-hidden="true">M</span>
        <div>
          <strong>{{ shopName }}</strong
          ><small>Development preview</small>
        </div>
      </div>
      <nav aria-label="Workspace sections">
        <button
          v-for="section in sections"
          :key="section.id"
          :class="{ selected: selected === section.id }"
          :aria-current="selected === section.id ? 'page' : undefined"
          @click="selected = section.id"
        >
          <span class="nav-symbol" aria-hidden="true">{{ section.symbol }}</span
          >{{ section.name }}
        </button>
      </nav>
      <div class="sidebar-note">
        <span aria-hidden="true">✳</span>
        <p>Made for makers.<br /><strong>Selling, smoothly.</strong></p>
      </div>
    </aside>
    <div class="content">
      <header class="topbar">
        <span
          >Workspace <span class="breadcrumb">/ {{ active.name }}</span></span
        >
        <div class="topbar-actions">
          <a href="/">View storefront <span aria-hidden="true">↗</span></a
          ><button
            v-if="props.authenticated"
            class="secondary-button"
            @click="emit('logout')"
          >
            Sign out
          </button>
        </div>
      </header>
      <main id="workspace" tabindex="-1">
        <aside
          v-if="props.authenticated && !props.mfaEnabled"
          class="mfa-reminder"
          aria-label="Security recommendation"
        >
          <div>
            <strong>Protect your shop with MFA.</strong
            ><span
              >Recommended: add an authenticator for an extra layer of account
              protection.</span
            >
          </div>
          <button type="button" @click="emit('enableMfa')">Enable MFA</button>
        </aside>
        <div class="preview-notice">
          <span class="notice-dot" aria-hidden="true"></span
          ><span>{{
            props.authenticated
              ? "Owner signed in · Shop settings and product drafts are available; checkout is still being built."
              : "Preview only · Authentication is not configured. No merchant data or shop changes are accessible."
          }}</span>
        </div>
        <div class="heading">
          <div>
            <p class="eyebrow">{{ active.name }}</p>
            <h1>{{ active.title }}</h1>
            <p class="intro">{{ active.description }}</p>
          </div>
          <div class="maker-mark" aria-hidden="true">✳</div>
        </div>
        <template v-if="selected === 'overview'">
          <section class="welcome-card" aria-labelledby="welcome-title">
            <div>
              <p class="eyebrow">The first steps</p>
              <h2 id="welcome-title">
                A good foundation for<br />something you love.
              </h2>
              <p>
                We’re building towards one simple moment: a maker sets up a shop
                and completes a test order.
              </p>
              <div class="pill">Foundation in progress</div>
            </div>
            <div class="illustration" aria-hidden="true">
              <div class="parcel"><span>✦</span></div>
              <div class="orbit orbit-one"></div>
              <div class="orbit orbit-two"></div>
              <span class="spark">✳</span>
            </div>
          </section>
          <div class="cards">
            <section class="card" aria-labelledby="progress-title">
              <div class="card-top">
                <h2 id="progress-title">Our path to the first shop</h2>
                <span class="small-label">3 steps</span>
              </div>
              <ol class="steps">
                <li>
                  <span class="step-number complete" aria-hidden="true">✓</span>
                  <div>
                    <strong>Lay the foundation</strong>
                    <p>Go app, database persistence and the admin preview.</p>
                    <small>Foundation preview available</small>
                  </div>
                </li>
                <li>
                  <span class="step-number" aria-hidden="true">2</span>
                  <div>
                    <strong>Make the shop yours</strong>
                    <p>Login, settings, products and page sections.</p>
                    <small
                      >Owner sign-in, shop settings and product drafts
                      available</small
                    >
                  </div>
                </li>
                <li>
                  <span class="step-number" aria-hidden="true">3</span>
                  <div>
                    <strong>Complete a test order</strong>
                    <p>Stock, delivery, payment and confirmations.</p>
                    <small>Square policy proof remains open</small>
                  </div>
                </li>
              </ol>
            </section>
            <section
              class="card connection-card"
              aria-labelledby="connection-title"
            >
              <div class="card-top">
                <h2 id="connection-title">Foundation connection</h2>
                <span aria-hidden="true">⌁</span>
              </div>
              <p
                class="health"
                :class="health"
                role="status"
                aria-live="polite"
              >
                <span class="status-dot" aria-hidden="true"></span
                >{{ healthText }}
              </p>
              <p>
                The preview checks the Go app’s database readiness. This does
                not establish checkout or live-shop readiness.
              </p>
              <button
                class="secondary-button"
                :disabled="health === 'checking'"
                @click="checkConnection"
              >
                {{ health === "checking" ? "Checking…" : "Check again" }}
              </button>
              <div class="connection-footnote">
                No sales or orders are being processed.
              </div>
            </section>
          </div>
        </template>
        <Products
          v-else-if="selected === 'products' && props.authenticated"
          @session-expired="emit('sessionExpired')"
        />
        <ShopSettings
          v-else-if="selected === 'settings' && props.authenticated"
          @loaded="shopName = $event"
          @session-expired="emit('sessionExpired')"
        />
        <section v-else class="card upcoming" aria-labelledby="upcoming-title">
          <div class="upcoming-symbol" aria-hidden="true">
            {{ active.symbol }}
          </div>
          <p class="eyebrow">Coming in a later increment</p>
          <h2 id="upcoming-title">{{ active.name }} is taking shape.</h2>
          <p>
            This section is a preview of the workspace structure. Its merchant
            functions are not implemented yet.
          </p>
          <button class="secondary-button" @click="selected = 'overview'">
            Back to overview
          </button>
        </section>
        <footer>
          Open source. Yours to host.
          <span>Built with care, one step at a time.</span>
        </footer>
      </main>
    </div>
  </div>
</template>
