<template>
  <div class="glass-card" :style="{ padding: padding || '20px' }">
    <div v-if="title" class="card-title">{{ title }}</div>
    <slot />
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  padding: { type: String, default: '' }
})
</script>

<style scoped>
.glass-card {
  background: rgba(255,255,255,.04); backdrop-filter: blur(12px);
  border: 1px solid rgba(255,255,255,.08); border-radius: 12px;
}
.card-title { font-size: 16px; font-weight: bold; color: var(--text-primary); margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid rgba(255,255,255,.06) }
</style>