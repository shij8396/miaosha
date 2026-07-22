<template>
  <div class="page-container seckill-home">
    <div class="hero-section fade-in-up">
      <h1 class="gradient-text">限时秒杀</h1>
      <p>超值好货，手慢无！</p>
    </div>

    <div class="product-grid" v-loading="loading">
      <el-empty v-if="!loading && products.length === 0" description="暂无秒杀商品" />
      <div v-for="p in products" :key="p.id" class="product-card glass-card fade-in-up">
        <div class="product-img">
          <el-icon :size="48"><Goods /></el-icon>
        </div>
        <div class="product-info">
          <div class="product-name">{{ p.name }}</div>
          <div class="product-desc">{{ p.description || '暂无描述' }}</div>
          <div class="countdown-row">
            <el-tag size="small" type="warning">距结束</el-tag>
            <CountDown :end-time="p.end_time" />
          </div>
          <div class="price-row">
            <span class="seckill-price">¥{{ p.seckill_price }}</span>
            <span class="original-price">¥{{ p.price }}</span>
          </div>
          <div class="stock-row">
            <el-progress :percentage="Math.round(p.remain_stock / p.total_stock * 100)" :color="progressColor(Math.round(p.remain_stock / p.total_stock * 100))" :stroke-width="8" />
            <span class="stock-text">剩余 {{ p.remain_stock }} / {{ p.total_stock }}</span>
          </div>
          <!-- [修复] 限购规则改造：动态展示限购数量 -->
          <div class="limit-tip" v-if="p.remain_stock > 0">
            每人限购 {{ p.limit_per_user || 1 }} 件
          </div>
          <!-- [修复] 任务4: 秒杀前验证码校验 -->
          <div class="captcha-row" v-if="p.remain_stock > 0 && !isLimitReached(p)">
            <span class="captcha-label">安全验证</span>
            <CaptchaInput ref="el => captchaRefs[p.id] = el" />
          </div>
          <el-button type="danger" size="large" style="width:100%;margin-top:12px" @click="doSeckill(p)" :disabled="isSeckillDisabled(p)" :loading="seckilling === p.id">
            {{ getSeckillButtonText(p) }}
          </el-button>
          <!-- [修复] 已抢购提示：动态显示限购数量 -->
          <div class="grabbed-tip" v-if="isLimitReached(p)">每人限购{{ p.limit_per_user || 1 }}件，如需重新购买请先取消已有订单</div>
        </div>
      </div>
    </div>

    <el-dialog v-model="resultVisible" title="秒杀结果" width="420px" :close-on-click-modal="true" :close-on-press-escape="true">
      <div class="result" v-if="seckillResult">
        <el-result
          :icon="seckillResult.status === 'queued' ? 'success' : 'error'"
          :title="seckillResult.status === 'queued' ? '秒杀排队成功！' : '秒杀失败'"
          :sub-title="seckillResult.message"
        >
          <template #extra v-if="seckillResult.status === 'queued'">
            <div class="order-info">
              <p>订单号：<el-tag>{{ seckillResult.order_no }}</el-tag></p>
              <p>订单正在处理中，请稍后在「我的订单」中查看</p>
            </div>
            <el-button type="primary" @click="router.push('/orders')">查看订单</el-button>
          </template>
        </el-result>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getActiveProducts } from '@/api/goodsApi'
import { seckill as doSeckillApi } from '@/api/seckillApi'
import CountDown from '@/components/Seckill/CountDown.vue'
import CaptchaInput from '@/components/Common/CaptchaInput.vue'
import { ElMessage } from 'element-plus'

// [修复] S-23: 添加 useRouter 用于结果弹窗中跳转订单页
const router = useRouter()
const products = ref([])
const loading = ref(false)
// [修复] S-23: seckilling 用于防止重复点击（正在秒杀中的商品ID）
const seckilling = ref(null)
// [修复] 限购规则改造：grabbedIds 改为 grabbedCounts 记录每个商品已购买数量
const grabbedCounts = ref({})
const resultVisible = ref(false)
const seckillResult = ref(null)
/* [修复] 任务4: 验证码组件引用，key 为商品 ID */
const captchaRefs = {}

function progressColor(pct) {
  if (pct > 50) return '#67c23a'
  if (pct > 20) return '#e6a23c'
  return '#e94560'
}

// [修复] 限购规则改造：判断是否已达到限购上限
function isLimitReached(p) {
  const limit = p.limit_per_user || 1
  return (grabbedCounts.value[p.id] || 0) >= limit
}

// [修复] 判断秒杀按钮是否禁用（活动时间外/售罄/已达限购上限）
function isSeckillDisabled(p) {
  if (p.remain_stock <= 0) return true
  if (isLimitReached(p)) return true
  const now = new Date()
  if (p.start_time && new Date(p.start_time) > now) return true
  if (p.end_time && new Date(p.end_time) < now) return true
  return false
}

// [修复] 获取秒杀按钮文字（根据状态显示不同文案）
function getSeckillButtonText(p) {
  if (p.remain_stock <= 0) return '已售罄'
  if (isLimitReached(p)) return '已抢购'
  const now = new Date()
  if (p.start_time && new Date(p.start_time) > now) return '活动未开始'
  if (p.end_time && new Date(p.end_time) < now) return '活动已结束'
  return '立即秒杀'
}

async function loadProducts() {
  loading.value = true
  try {
    const data = await getActiveProducts()
    products.value = Array.isArray(data) ? data : (data?.list || [])
  } catch (e) { /* [修复] 错误已在拦截器中处理 */ console.error('加载商品列表失败:', e) } finally { loading.value = false }
}

async function doSeckill(product) {
  // [修复] S-23: 防重复提交 - 如果正在秒杀中或已达限购上限，直接返回
  if (seckilling.value || isLimitReached(product)) return

  /* [修复] 任务4: 秒杀前先校验验证码 */
  const captchaRef = captchaRefs[product.id]
  if (captchaRef) {
    const result = captchaRef.validate()
    if (!result.valid) {
      ElMessage.warning(result.message)
      return
    }
  }

  // [修复] S-23: 立即设置秒杀中状态，防止重复点击
  seckilling.value = product.id
  try {
    // [修复] 生成幂等性 Key，防止重复提交：使用 crypto.randomUUID() + 时间戳
    const idempotentKey = crypto.randomUUID()
    const data = await doSeckillApi({ product_id: product.id, quantity: 1, idempotent_key: idempotentKey })
    seckillResult.value = data
    resultVisible.value = true
    if (data.status === 'queued') {
      // [修复] 限购规则改造：累加已购买数量
      grabbedCounts.value[product.id] = (grabbedCounts.value[product.id] || 0) + 1
      loadProducts()
    }
    /* [修复] 任务4: 秒杀成功后重置验证码 */
    if (captchaRef) captchaRef.reset()
  } catch (e) {
    console.error('秒杀请求失败:', e)
    // [修复] 限购规则改造：已参与过错误时标记已达上限
    const errMsg = e?.message || e?.toString() || ''
    if (errMsg.includes('已参与过') || errMsg.includes('限购')) {
      const limit = product.limit_per_user || 1
      grabbedCounts.value[product.id] = limit
      ElMessage.warning(`该商品每人限购${limit}件，您已达到上限`)
    }
    /* [修复] 任务4: 秒杀失败也刷新验证码 */
    if (captchaRef) captchaRef.refreshCaptcha()
  } finally {
    // [修复] S-23: 秒杀完成（无论成功失败）后清除秒杀中状态
    seckilling.value = null
  }
}

onMounted(loadProducts)
</script>

<style scoped>
.seckill-home { max-width: 1200px; margin: 0 auto }
.hero-section { text-align: center; padding: 40px 0 32px }
.hero-section h1 { font-size: 36px; margin-bottom: 8px }
.hero-section p { color: #999; font-size: 16px }
.product-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 20px; min-height: 200px }
.product-card { display: flex; gap: 20px; padding: 24px; transition: all .3s }
.product-card:hover { border-color: rgba(233,69,96,.3) !important; transform: translateY(-2px) }
.product-img { width: 80px; height: 80px; display: flex; align-items: center; justify-content: center; background: rgba(233,69,96,.1); border-radius: 8px }
.product-info { flex: 1 }
.product-name { font-size: 18px; font-weight: bold; color: var(--text-primary); margin-bottom: 4px }
.product-desc { color: #999; font-size: 13px; margin-bottom: 12px }
.countdown-row { display: flex; align-items: center; gap: 8px; margin-bottom: 12px }
.price-row { display: flex; align-items: baseline; gap: 8px; margin-bottom: 12px }
.seckill-price { font-size: 28px; color: #e94560; font-weight: bold }
.original-price { font-size: 14px; color: #666; text-decoration: line-through }
.stock-row { margin-bottom: 4px }
.stock-text { font-size: 12px; color: #999; display: block; margin-top: 4px }
/* [修复] 限购规则改造：限购提示样式 */
.limit-tip { font-size: 12px; color: #e6a23c; margin-top: 4px; margin-bottom: 4px }
/* [修复] 已抢购提示样式 */
.grabbed-tip { font-size: 12px; color: #e94560; margin-top: 8px; text-align: center }
/* [修复] 任务4: 验证码区域样式 */
.captcha-row { margin-top: 8px; margin-bottom: 4px }
.captcha-label { font-size: 12px; color: #888; display: block; margin-bottom: 4px }
.order-info { text-align: left; margin: 12px 0 }
.order-info p { margin: 6px 0; color: var(--text-secondary) }
</style>