<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId } from "vue";
const dialog = ref<HTMLDialogElement>();
const cancelButton = ref<HTMLButtonElement>();
const titleId = useId();
let answer: ((discard: boolean) => void) | undefined;
let previousFocus: HTMLElement | null = null;
async function confirm(): Promise<boolean> {
  if (answer) return false;
  previousFocus =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
  const result = new Promise<boolean>((resolve) => {
    answer = resolve;
  });
  await nextTick();
  if (!dialog.value || !answer) {
    finish(false);
    return result;
  }
  dialog.value.showModal();
  cancelButton.value?.focus();
  return result;
}
function finish(discard: boolean) {
  dialog.value?.close();
  const resolve = answer;
  answer = undefined;
  resolve?.(discard);
  previousFocus?.focus();
  previousFocus = null;
}
onBeforeUnmount(() => finish(false));
defineExpose({ confirm });
</script>
<template>
  <dialog
    ref="dialog"
    class="discard-dialog"
    :aria-labelledby="titleId"
    @cancel.prevent="finish(false)"
  >
    <h2 :id="titleId">Discard unsaved page edits?</h2>
    <p>Your saved draft and published page will stay as they are.</p>
    <div class="product-actions">
      <button
        ref="cancelButton"
        type="button"
        class="auth-primary"
        @click="finish(false)"
      >
        Keep editing</button
      ><button type="button" class="secondary-button" @click="finish(true)">
        Discard edits
      </button>
    </div>
  </dialog>
</template>
