<template>
  <div class="data-table-wrap">
    <div class="table-toolbar" v-if="$slots.toolbar">
      <slot name="toolbar" />
    </div>
    <el-table :data="data" v-loading="loading" stripe :empty-text="emptyText" @selection-change="onSelect" style="width:100%">
      <el-table-column v-if="selectable" type="selection" width="45" />
      <slot />
      <!-- [修复] 任务9: 友好的空数据提示 -->
      <template #empty>
        <div class="empty-container">
          <el-empty :description="emptyText" :image-size="80" />
        </div>
      </template>
    </el-table>
    <div class="table-footer" v-if="total > 0">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="total"
        layout="total, sizes, prev, pager, next"
        background
        small
        @size-change="$emit('size-change', $event)"
        @current-change="$emit('page-change', $event)"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
const props = defineProps({
  data: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  total: { type: Number, default: 0 },
  emptyText: { type: String, default: '暂无数据' },
  selectable: { type: Boolean, default: false }
})
const emit = defineEmits(['selection-change', 'size-change', 'page-change'])
const currentPage = ref(1)
const pageSize = ref(10)
function onSelect(rows) { emit('selection-change', rows) }
</script>

<style scoped>
.data-table-wrap { background: transparent }
.table-toolbar { margin-bottom: 16px; display: flex; gap: 12px; align-items: center }
.table-footer { margin-top: 16px; display: flex; justify-content: flex-end }
:deep(.el-table) {
  --el-table-bg-color: transparent; --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: rgba(255,255,255,.04); --el-table-border-color: rgba(255,255,255,.06);
  --el-table-text-color: var(--text-primary); --el-table-header-text-color: var(--text-secondary);
  --el-table-row-hover-bg-color: rgba(255,255,255,.06);
}
</style>