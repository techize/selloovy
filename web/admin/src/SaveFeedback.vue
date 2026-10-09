<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
const props = defineProps<{ error: string; success: string }>();
const feedback = ref<HTMLElement>();
watch(
  () => [props.error, props.success],
  async () => {
    if (!props.error && !props.success) return;
    await nextTick();
    feedback.value?.focus({ preventScroll: true });
    feedback.value?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  },
  { immediate: true },
);
</script>
<template>
  <p
    v-if="error || success"
    ref="feedback"
    tabindex="-1"
    :class="error ? 'settings-error' : 'settings-success'"
    :role="error ? 'alert' : 'status'"
  >
    {{ error || success }}
  </p>
</template>
