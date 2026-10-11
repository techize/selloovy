<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from "vue";
import DiscardPrompt from "./DiscardPrompt.vue";
import SaveFeedback from "./SaveFeedback.vue";
type FAQ = { question: string; answer: string };
type Section = {
  id: string;
  kind: string;
  heading: string;
  text: string;
  theme: string;
  imageProductId: number;
  productIds: number[];
  questions: FAQ[];
  buttonLabel: string;
  buttonPath: string;
};
type Page = { slug: string; title: string; sections: Section[] };
type Product = {
  id: number;
  name: string;
  photoId: string;
  photoAlt: string;
  pricePence: number;
};
type State = {
  revision: number;
  drafts: Page[];
  published: Page[];
  publicKey: string;
  products: Product[];
  moreProducts: boolean;
};
const emit = defineEmits<{ sessionExpired: [] }>();
const discardPrompt = ref<InstanceType<typeof DiscardPrompt>>();
const state = ref<State>();
const draft = ref<Page>();
const saved = ref("");
const busy = ref(false);
const blocked = ref(true);
const preview = ref(false);
const error = ref("");
const success = ref("");
const newSlug = ref("");
const kind = ref("hero");
const dragged = ref("");
const announcement = ref("");
let controller: AbortController | undefined;
let active = true;
const dirty = computed(
  () => !!draft.value && JSON.stringify(draft.value) !== saved.value,
);
defineExpose({ hasUnsaved: dirty });
const published = computed(() =>
  state.value?.published.some((p) => p.slug === draft.value?.slug),
);
const livePath = computed(() =>
  !state.value?.publicKey || !draft.value
    ? ""
    : `/shop/${state.value.publicKey}${draft.value.slug === "home" ? "" : "/pages/" + draft.value.slug}`,
);
function validPage(v: unknown): v is Page {
  const p = v as Page;
  return (
    !!p &&
    typeof p.slug === "string" &&
    typeof p.title === "string" &&
    Array.isArray(p.sections) &&
    p.sections.every(
      (s) =>
        s &&
        typeof s.id === "string" &&
        ["hero", "text-image", "collection", "faq"].includes(s.kind) &&
        typeof s.heading === "string" &&
        typeof s.text === "string" &&
        ["cream", "sage", "ink"].includes(s.theme) &&
        Number.isSafeInteger(s.imageProductId) &&
        Array.isArray(s.productIds) &&
        s.productIds.every(Number.isSafeInteger) &&
        Array.isArray(s.questions) &&
        s.questions.every(
          (q) =>
            q && typeof q.question === "string" && typeof q.answer === "string",
        ) &&
        typeof s.buttonLabel === "string" &&
        typeof s.buttonPath === "string",
    )
  );
}
function valid(v: unknown): v is State {
  const s = v as State;
  return (
    !!s &&
    Number.isSafeInteger(s.revision) &&
    s.revision > 0 &&
    Array.isArray(s.drafts) &&
    s.drafts.every(validPage) &&
    Array.isArray(s.published) &&
    s.published.every(validPage) &&
    typeof s.moreProducts === "boolean" &&
    typeof s.publicKey === "string" &&
    (s.publicKey === "" || /^[0-9a-f]{32}$/.test(s.publicKey)) &&
    Array.isArray(s.products) &&
    s.products.every(
      (p) =>
        p &&
        Number.isSafeInteger(p.id) &&
        typeof p.name === "string" &&
        typeof p.photoId === "string" &&
        (p.photoId === "" || /^[0-9a-f]{32}$/.test(p.photoId)) &&
        typeof p.photoAlt === "string" &&
        Number.isSafeInteger(p.pricePence),
    )
  );
}
async function choose(p: Page) {
  if (busy.value) return;
  if (dirty.value && draft.value && !(await discardPrompt.value?.confirm()))
    return;
  draft.value = JSON.parse(JSON.stringify(p)) as Page;
  saved.value = JSON.stringify(draft.value);
  preview.value = false;
  error.value = "";
  success.value = "";
}
async function request(body?: unknown) {
  const c = new AbortController();
  controller = c;
  const timer = setTimeout(() => c.abort(), 8000);
  try {
    const r = await fetch("/api/admin/pages", {
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
    if (active && r.status === 401) emit("sessionExpired");
    return { ok: r.ok, status: r.status, data };
  } finally {
    clearTimeout(timer);
  }
}
async function load() {
  if (busy.value) return;
  if (draft.value && dirty.value && !(await discardPrompt.value?.confirm()))
    return;
  busy.value = true;
  blocked.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await request();
    if (!active) return;
    if (!r.ok || !valid(r.data)) throw Error();
    state.value = r.data;
    draft.value = undefined;
    saved.value = "";
    blocked.value = false;
    const p = r.data.drafts.find((x) => x.slug === "home") ?? r.data.drafts[0];
    if (p) {
      draft.value = JSON.parse(JSON.stringify(p)) as Page;
      saved.value = JSON.stringify(p);
    }
  } catch {
    if (active) error.value = "Could not load pages. Reload before saving.";
  } finally {
    if (active) busy.value = false;
  }
}
async function addPage(home = false) {
  if (busy.value || blocked.value || !state.value) return;
  const slug = home ? "home" : newSlug.value.trim();
  if (
    !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(slug) ||
    slug.length > 60 ||
    state.value.drafts.some((p) => p.slug === slug)
  ) {
    error.value =
      "Use a unique lowercase slug, with words separated by hyphens.";
    return;
  }
  if (state.value.drafts.length >= 20) {
    error.value = "A shop can have up to 20 pages.";
    return;
  }
  if (draft.value && dirty.value && !(await discardPrompt.value?.confirm()))
    return;
  draft.value = { slug, title: home ? "Welcome" : "New page", sections: [] };
  saved.value = "";
  preview.value = false;
  success.value = "";
  error.value = "";
  newSlug.value = "";
  addSection();
}
function addSection() {
  if (
    !draft.value ||
    busy.value ||
    blocked.value ||
    draft.value.sections.length >= 20
  )
    return;
  draft.value.sections.push({
    id: crypto.randomUUID().replaceAll("-", ""),
    kind: kind.value,
    heading:
      (
        {
          hero: "Welcome to our shop",
          "text-image": "Our story",
          collection: "Our creations",
          faq: "Your questions",
        } as Record<string, string>
      )[kind.value] ?? "",
    text: "",
    theme: "cream",
    imageProductId: 0,
    productIds: [],
    questions: kind.value === "faq" ? [{ question: "", answer: "" }] : [],
    buttonLabel: "",
    buttonPath: "",
  });
  success.value = "";
}
function move(id: string, to: number) {
  if (!draft.value || busy.value || preview.value) return;
  const list = draft.value.sections;
  const from = list.findIndex((s) => s.id === id);
  if (from < 0 || to < 0 || to >= list.length || from === to) return;
  const [s] = list.splice(from, 1);
  if (s) list.splice(to, 0, s);
  announcement.value = `${s?.heading || "Section"} moved to position ${to + 1}.`;
  success.value = "";
}
function pointerStart(event: PointerEvent, id: string) {
  if (busy.value || preview.value || event.button !== 0) return;
  const handle = event.currentTarget as HTMLElement;
  dragged.value = id;
  handle.focus();
  handle.setPointerCapture(event.pointerId);
  event.preventDefault();
}
function pointerEnd(event: PointerEvent) {
  const target = document
    .elementFromPoint(event.clientX, event.clientY)
    ?.closest<HTMLElement>("[data-section-id]");
  const index =
    draft.value?.sections.findIndex(
      (s) => s.id === target?.dataset.sectionId,
    ) ?? -1;
  if (dragged.value && index >= 0) move(dragged.value, index);
  dragged.value = "";
  const handle = event.currentTarget as HTMLElement;
  if (handle.hasPointerCapture(event.pointerId))
    handle.releasePointerCapture(event.pointerId);
}
function product(id: number) {
  return state.value?.products.find((p) => p.id === id);
}
function image(id: number) {
  const p = product(id);
  return p?.photoId && state.value?.publicKey
    ? `/shop/${state.value.publicKey}/products/${id}/photos/${p.photoId}`
    : "";
}
async function change(action: string) {
  if (!draft.value || !state.value || busy.value || blocked.value) return;
  if (action === "publish" && (dirty.value || !preview.value)) {
    error.value = "Save the draft, then review its preview before publishing.";
    return;
  }
  busy.value = true;
  error.value = "";
  success.value = "";
  const slug = draft.value.slug;
  try {
    const r = await request({
      revision: state.value.revision,
      action,
      slug,
      page:
        action === "save" ? draft.value : { slug: "", title: "", sections: [] },
    });
    if (!active) return;
    if (!r.ok || !valid(r.data)) {
      if (r.status === 409) blocked.value = true;
      const body = r.data as { error?: string; fields?: { page?: string } };
      error.value =
        body?.fields?.page ||
        body?.error ||
        "The page could not be saved. Reload to check its state.";
      return;
    }
    state.value = r.data;
    const p = r.data.drafts.find((x) => x.slug === slug);
    draft.value = p ? (JSON.parse(JSON.stringify(p)) as Page) : undefined;
    saved.value = JSON.stringify(draft.value) ?? "";
    if (action === "save") preview.value = false;
    success.value =
      (
        {
          save: "Page draft saved. Review the preview to publish it.",
          publish: "Page published successfully.",
          unpublish: "Page unpublished. The draft is still available.",
          delete: "Page draft deleted.",
        } as Record<string, string>
      )[action] ?? "Page updated.";
  } catch {
    if (active) {
      blocked.value = true;
      error.value =
        "The request did not complete. Reload to check the saved page before trying again.";
    }
  } finally {
    if (active) busy.value = false;
  }
}
function beforeLeave(event: BeforeUnloadEvent) {
  if (dirty.value && draft.value) {
    event.preventDefault();
    event.returnValue = "";
  }
}
onMounted(() => {
  load();
  window.addEventListener("beforeunload", beforeLeave);
});
onBeforeUnmount(() => {
  active = false;
  controller?.abort();
  window.removeEventListener("beforeunload", beforeLeave);
});
</script>

<template>
  <section class="card page-builder" aria-label="Page builder">
    <DiscardPrompt ref="discardPrompt" />
    <div class="card-top">
      <h2>Your pages</h2>
      <button class="secondary-button" :disabled="busy" @click="load">
        Reload saved pages
      </button>
    </div>
    <p>
      Build with reusable sections. Save a private draft, review it, then
      publish from here. Use <strong>home</strong> for your homepage;
      information pages appear in your shop navigation.
    </p>
    <nav class="product-actions" aria-label="Choose a page">
      <button
        v-for="p in state?.drafts"
        :key="p.slug"
        class="secondary-button"
        :disabled="busy || blocked"
        :aria-pressed="draft?.slug === p.slug"
        @click="choose(p)"
      >
        {{ p.title }} · {{ p.slug }}
      </button>
      <button
        v-if="state && !state.drafts.some((p) => p.slug === 'home')"
        class="secondary-button"
        :disabled="busy || blocked"
        @click="addPage(true)"
      >
        Create homepage
      </button>
    </nav>
    <form class="product-actions" @submit.prevent="addPage()">
      <label for="new-page-slug">New information page slug</label
      ><input
        id="new-page-slug"
        v-model="newSlug"
        placeholder="our-story"
        maxlength="60"
        :disabled="busy || blocked"
        required
      /><button class="secondary-button" :disabled="busy || blocked">
        Add page
      </button>
    </form>
    <p v-if="state?.moreProducts">
      The selector shows your first 500 published products.
    </p>
    <template v-if="draft">
      <div class="product-actions">
        <h3>{{ draft.slug === "home" ? "Homepage" : draft.slug }}</h3>
        <span
          >{{ published ? "Published" : "Private draft"
          }}{{ dirty ? " · Unsaved edits" : "" }}</span
        ><a
          v-if="published && livePath"
          :href="livePath"
          target="_blank"
          rel="noopener"
          >View live page ↗</a
        >
      </div>
      <fieldset :disabled="busy || blocked" class="page-controls">
        <label for="page-title">Page title (120 characters)</label
        ><input
          id="page-title"
          v-model="draft.title"
          maxlength="120"
          :readonly="preview"
        />
        <div v-if="!preview" class="product-actions">
          <label for="section-kind">Section type</label
          ><select id="section-kind" v-model="kind">
            <option value="hero">Hero</option>
            <option value="text-image">Text and image</option>
            <option value="collection">Product collection</option>
            <option value="faq">FAQ</option></select
          ><button
            type="button"
            class="secondary-button"
            :disabled="draft.sections.length >= 20"
            @click="addSection"
          >
            Add section
          </button>
        </div>
        <p v-if="!preview">
          Drag a section’s handle onto another section to reorder, or use Move
          up / Move down. Up to 20 sections per page.
        </p>
        <p class="sr-only" role="status" aria-live="polite">
          {{ announcement }}
        </p>
        <div v-if="!preview" class="section-list">
          <section
            v-for="(s, index) in draft.sections"
            :key="s.id"
            class="section-editor"
            :data-section-id="s.id"
            :class="{ dragging: dragged === s.id }"
          >
            <div class="product-actions">
              <button
                type="button"
                class="secondary-button drag-handle"
                :aria-label="`Drag section ${index + 1}: ${s.heading}`"
                @pointerdown="pointerStart($event, s.id)"
                @pointerup="pointerEnd"
                @pointercancel="dragged = ''"
              >
                ⠿ {{ index + 1 }} · {{ s.kind }}</button
              ><button
                type="button"
                class="secondary-button"
                :disabled="index === 0"
                @click="move(s.id, index - 1)"
              >
                Move up</button
              ><button
                type="button"
                class="secondary-button"
                :disabled="index === draft.sections.length - 1"
                @click="move(s.id, index + 1)"
              >
                Move down</button
              ><button
                type="button"
                class="secondary-button"
                @click="draft.sections.splice(index, 1)"
              >
                Remove section
              </button>
            </div>
            <label :for="s.id + '-heading'">Heading (160 characters)</label
            ><input
              :id="s.id + '-heading'"
              v-model="s.heading"
              maxlength="160"
            />
            <label :for="s.id + '-theme'">Colour palette</label
            ><select :id="s.id + '-theme'" v-model="s.theme">
              <option value="cream">Warm cream</option>
              <option value="sage">Soft sage</option>
              <option value="ink">Deep ink</option>
            </select>
            <template v-if="s.kind === 'hero' || s.kind === 'text-image'">
              <label :for="s.id + '-text'">Text (4,000 characters)</label
              ><textarea
                :id="s.id + '-text'"
                v-model="s.text"
                maxlength="4000"
                rows="4"
              />
              <label :for="s.id + '-image'"
                >Image from a published product cover</label
              ><select :id="s.id + '-image'" v-model="s.imageProductId">
                <option :value="0">No image</option>
                <option
                  v-for="p in state?.products.filter((p) => p.photoId)"
                  :key="p.id"
                  :value="p.id"
                >
                  {{ p.name }} · #{{ p.id }}
                </option>
              </select>
              <p>
                To add or change an image, upload and publish its product cover
                in Products, then reload these pages.
              </p>
              <label :for="s.id + '-button'"
                >Button label (optional, 80 characters)</label
              ><input
                :id="s.id + '-button'"
                v-model="s.buttonLabel"
                maxlength="80"
              />
              <label :for="s.id + '-target'">Button destination</label
              ><select :id="s.id + '-target'" v-model="s.buttonPath">
                <option value="">No button</option>
                <option value="/products">All creations</option>
                <option
                  v-for="p in state?.published.filter((p) => p.slug !== 'home')"
                  :key="p.slug"
                  :value="'/pages/' + p.slug"
                >
                  {{ p.title }}
                </option>
              </select>
            </template>
            <p
              v-if="s.imageProductId && !product(s.imageProductId)"
              role="status"
            >
              The selected image's product is no longer published. Choose
              another cover or No image before publishing.
            </p>
            <fieldset v-if="s.kind === 'collection'">
              <legend>Choose 1–12 published products</legend>
              <label
                v-for="id in s.productIds.filter((id) => !product(id))"
                :key="id"
                class="collection-option"
                ><input
                  v-model="s.productIds"
                  type="checkbox"
                  :value="id"
                />Product #{{ id }} is no longer published — deselect to
                remove</label
              >
              <label
                v-for="p in state?.products"
                :key="p.id"
                class="collection-option"
                ><input
                  v-model="s.productIds"
                  type="checkbox"
                  :value="p.id"
                  :disabled="
                    !s.productIds.includes(p.id) && s.productIds.length >= 12
                  "
                />{{ p.name }} · #{{ p.id }}</label
              >
              <p v-if="!state?.products.length">
                Publish products first to include them here.
              </p>
            </fieldset>
            <template v-if="s.kind === 'faq'"
              ><fieldset v-for="(q, i) in s.questions" :key="i">
                <legend>Question {{ i + 1 }}</legend>
                <label :for="s.id + '-q' + i">Question (200 characters)</label
                ><input
                  :id="s.id + '-q' + i"
                  v-model="q.question"
                  maxlength="200"
                /><label :for="s.id + '-a' + i">Answer (2,000 characters)</label
                ><textarea
                  :id="s.id + '-a' + i"
                  v-model="q.answer"
                  maxlength="2000"
                /><button
                  type="button"
                  class="secondary-button"
                  @click="s.questions.splice(i, 1)"
                >
                  Remove question
                </button>
              </fieldset>
              <button
                type="button"
                class="secondary-button"
                :disabled="s.questions.length >= 12"
                @click="s.questions.push({ question: '', answer: '' })"
              >
                Add question
              </button></template
            >
          </section>
        </div>
        <div v-else class="page-preview" aria-label="Saved page preview">
          <p>
            This preview uses the saved section order and current published
            product covers. Product prices and images follow their own
            publications.
          </p>
          <h2>{{ draft.title }}</h2>
          <section
            v-for="s in draft.sections"
            :key="s.id"
            class="preview-section"
            :class="'preview-' + s.theme"
          >
            <h3>{{ s.heading }}</h3>
            <img
              v-if="image(s.imageProductId)"
              :src="image(s.imageProductId)"
              :alt="product(s.imageProductId)?.photoAlt"
            />
            <p class="page-copy">{{ s.text }}</p>
            <span v-if="s.buttonLabel" class="preview-button"
              >{{ s.buttonLabel }} →</span
            >
            <div v-if="s.kind === 'collection'" class="preview-products">
              <article v-for="id in s.productIds" :key="id">
                <img
                  v-if="image(id)"
                  :src="image(id)"
                  :alt="product(id)?.photoAlt"
                />
                <h4>
                  {{ product(id)?.name || "Product no longer published" }}
                </h4>
                <p v-if="product(id)">
                  From £{{ ((product(id)?.pricePence ?? 0) / 100).toFixed(2) }}
                </p>
              </article>
            </div>
            <details v-for="(q, i) in s.questions" :key="i">
              <summary>{{ q.question }}</summary>
              <p class="page-copy">{{ q.answer }}</p>
            </details>
          </section>
        </div>
      </fieldset>
      <div class="product-actions">
        <button
          v-if="!preview"
          class="auth-primary"
          :disabled="busy || blocked"
          @click="change('save')"
        >
          {{ busy ? "Saving…" : "Save draft" }}</button
        ><button
          class="secondary-button"
          :disabled="busy || blocked || (!preview && dirty)"
          @click="preview = !preview"
        >
          {{ preview ? "Back to editor" : "Review saved preview" }}</button
        ><button
          v-if="preview"
          class="auth-primary"
          :disabled="busy || blocked || dirty || !state?.publicKey"
          @click="change('publish')"
        >
          {{
            busy ? "Publishing…" : published ? "Republish page" : "Publish page"
          }}</button
        ><button
          v-if="published"
          class="secondary-button"
          :disabled="busy || blocked || dirty"
          @click="change('unpublish')"
        >
          Unpublish</button
        ><button
          v-if="!published && state?.drafts.some((p) => p.slug === draft?.slug)"
          class="secondary-button"
          :disabled="busy || blocked || dirty"
          @click="change('delete')"
        >
          Delete draft
        </button>
      </div>
      <p v-if="!state?.publicKey">
        Publish a product before publishing your first page.
      </p>
    </template>
    <SaveFeedback :error="error" :success="success" />
  </section>
</template>
