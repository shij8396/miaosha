<!--
  [修复] 侧边栏菜单改造：将「运营管理」「风控管理」「监控大屏」改为可折叠下拉分组菜单
  使用 el-sub-menu 组件，层级清晰，符合企业后台规范
  router 属性绑定启用路由跳转，default-active 高亮当前菜单
-->
<template>
  <div class="sidebar" :class="{ collapsed: globalStore.sidebarCollapsed }">
    <div class="logo-area" @click="router.push('/seckill')">
      <el-icon :size="28" color="#e94560"><Lightning /></el-icon>
      <span class="logo-text" v-show="!globalStore.sidebarCollapsed">秒杀系统</span>
    </div>
    <el-menu
      :default-active="activeMenu"
      :collapse="globalStore.sidebarCollapsed"
      :router="true"
      background-color="transparent"
      text-color="#999"
      active-text-color="#e94560"
      class="side-menu"
    >
      <!-- 默认菜单 -->
      <el-menu-item index="/seckill">
        <el-icon><HomeFilled /></el-icon>
        <template #title>秒杀首页</template>
      </el-menu-item>
      <el-menu-item index="/orders">
        <el-icon><List /></el-icon>
        <template #title>我的订单</template>
      </el-menu-item>

      <!-- [修复] 运营管理 - 可折叠下拉分组 -->
      <el-sub-menu v-if="hasMenuAccess('operator')" index="group-operator">
        <template #title>
          <el-icon><Setting /></el-icon>
          <span>运营管理</span>
        </template>
        <el-menu-item index="/admin/goods">
          <el-icon><Goods /></el-icon>
          <template #title>商品管理</template>
        </el-menu-item>
        <el-menu-item index="/admin/orders">
          <el-icon><Document /></el-icon>
          <template #title>订单管理</template>
        </el-menu-item>
        <el-menu-item index="/admin/activity">
          <el-icon><Timer /></el-icon>
          <template #title>活动配置</template>
        </el-menu-item>
        <el-menu-item index="/admin/stock">
          <el-icon><Check /></el-icon>
          <template #title>库存对账</template>
        </el-menu-item>
      </el-sub-menu>

      <!-- [修复] 风控管理 - 可折叠下拉分组 -->
      <el-sub-menu v-if="hasMenuAccess('risk_control')" index="group-risk">
        <template #title>
          <el-icon><Warning /></el-icon>
          <span>风控管理</span>
        </template>
        <el-menu-item index="/admin/sentinel">
          <el-icon><Cpu /></el-icon>
          <template #title>限流规则</template>
        </el-menu-item>
        <el-menu-item index="/admin/blacklist">
          <el-icon><Lock /></el-icon>
          <template #title>黑名单</template>
        </el-menu-item>
        <el-menu-item index="/admin/roles">
          <el-icon><User /></el-icon>
          <template #title>权限管理</template>
        </el-menu-item>
      </el-sub-menu>

      <!-- [修复] 监控大屏 - 可折叠下拉分组 -->
      <el-sub-menu v-if="hasMenuAccess('monitor_readonly')" index="group-monitor">
        <template #title>
          <el-icon><Monitor /></el-icon>
          <span>监控大屏</span>
        </template>
        <el-menu-item index="/screen">
          <el-icon><DataAnalysis /></el-icon>
          <template #title>数据大屏</template>
        </el-menu-item>
        <el-menu-item index="/monitor">
          <el-icon><TrendCharts /></el-icon>
          <template #title>服务监控</template>
        </el-menu-item>
      </el-sub-menu>
    </el-menu>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGlobalStore } from '@/pinia/globalStore'
import { useUserStore } from '@/pinia/userStore'

const route = useRoute()
const router = useRouter()
const globalStore = useGlobalStore()
const userStore = useUserStore()

// [修复] 当前激活菜单路径
const activeMenu = computed(() => route.path)

// [修复] 用户角色
const userRole = computed(() => userStore.userInfo?.role || '')

// [修复] 菜单分组权限判断：admin/super_admin 可访问所有分组
function hasMenuAccess(menuGroup) {
  const role = userRole.value
  if (role === 'super_admin' || role === 'admin') return true
  const roleMenus = {
    operator: ['operator'],
    monitor_readonly: ['monitor_readonly'],
    risk_control: ['risk_control']
  }
  return roleMenus[menuGroup]?.includes(role) || false
}
</script>

<style scoped>
.sidebar {
  width: 220px;
  height: 100%;
  background: var(--sidebar-bg);
  border-right: 1px solid rgba(255,255,255,.06);
  display: flex;
  flex-direction: column;
  transition: width .3s;
  overflow: hidden;
  flex-shrink: 0;
  z-index: 100;
}

.sidebar.collapsed {
  width: 64px;
}

.logo-area {
  height: 60px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 20px;
  cursor: pointer;
  border-bottom: 1px solid rgba(255,255,255,.06);
  flex-shrink: 0;
}

.logo-text {
  font-size: 18px;
  font-weight: bold;
  color: #e94560;
  white-space: nowrap;
}

.side-menu {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  border-right: none !important;
  padding: 8px 0;
}

/* [修复] 子菜单项缩进 */
.side-menu :deep(.el-sub-menu .el-menu-item) {
  padding-left: 56px !important;
  min-width: auto;
}

/* [修复] 子菜单标题样式 */
.side-menu :deep(.el-sub-menu__title) {
  font-weight: 500;
}

/* [修复] 菜单项 hover 效果 */
.side-menu :deep(.el-menu-item:hover),
.side-menu :deep(.el-sub-menu__title:hover) {
  background: rgba(233,69,96,.08) !important;
}

/* [修复] 折叠状态下的弹出菜单样式 */
.side-menu :deep(.el-menu--popup) {
  background: rgba(20,20,40,.95) !important;
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255,255,255,.06);
}
</style>