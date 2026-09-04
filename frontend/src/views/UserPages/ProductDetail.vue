<template>
  <div class="page-container product-detail" v-loading="loading">
    <div class="back-row">
      <el-button text @click="router.back()">
        <el-icon><ArrowLeft /></el-icon> 返回
      </el-button>
    </div>

    <el-empty v-if="!loading && !product" description="商品不存在或已下架" />

    <div class="detail-body" v-if="product">
      <!-- 左侧图片 -->
      <div class="detail-img glass-card">
        <img v-if="product.image_url" :src="product.image_url" :alt="product.name" class="real-img" @error="product.image_url = ''" />
        <el-icon v-else :size="120"><Goods /></el-icon>
      </div>

      <!-- 右侧信息 -->
      <div class="detail-info glass-card">
        <h2 class="p-name">{{ product.name }}</h2>
        <p class="p-desc">{{ product.description || '暂无描述' }}</p>

        <div class="countdown-row">
          <el-tag size="small" type="warning">距活动结束</el-tag>
          <CountDown :end-time="product.end_time" />
        </div>

        <div class="price-row">
          <span class="seckill-price">¥{{ displayPrice }}</span>
          <span class="original-price">¥{{ product.price }}</span>
        </div>

        <!-- [修复] 商品配置（SKU）选择器：不同配置对应不同价格 -->
        <div class="sku-section" v-if="attrs.length > 0">
          <div class="sku-group" v-for="attr in attrs" :key="attr.name">
            <div class="sku-label">{{ attr.name }}</div>
            <div class="sku-values">
              <el-tag
                v-for="v in attr.values"
                :key="v"
                :type="selected[attr.name] === v ? 'danger' : 'info'"
                :effect="selected[attr.name] === v ? 'dark' : 'plain'"
                class="sku-value-tag"
                @click="selectAttr(attr.name, v)"
              >{{ v }}</el-tag>
            </div>
          </div>
          <div class="sku-match-tip" v-if="!allSelected">请选择完整规格以查看该配置价格</div>
          <div class="sku-match-tip" v-else-if="!matchedSKU">该组合暂无货，请更换规格</div>
        </div>

        <div class="stock-row">
          <el-progress :percentage="stockPercent" :color="progressColor(stockPercent)" :stroke-width="8" />
          <span class="stock-text">剩余 {{ product.remain_stock }} / {{ product.total_stock }}（每人限购 {{ product.limit_per_user || 1 }} 件）</span>
        </div>

        <!-- [创新] 秒杀前安全验证：数学验证码 -->
        <div class="captcha-row" v-if="canBuy">
          <span class="captcha-label">安全验证</span>
          <div class="math-captcha-wrap">
            <span class="math-expr" @click="refreshCaptcha" title="点击刷新验证码">
              {{ captchaData?.expression || '点击获取' }}
            </span>
            <el-input v-model="captchaInput" placeholder="输入结果" maxlength="4" class="captcha-input-field" @keyup.enter="doSeckill" />
            <el-button size="small" text @click="refreshCaptcha" :loading="captchaLoading">
              <el-icon><Refresh /></el-icon>
            </el-button>
          </div>
        </div>

        <el-button
          type="danger"
          size="large"
          class="buy-btn"
          @click="doSeckill"
          :disabled="isDisabled"
          :loading="seckilling"
        >{{ buttonText }}</el-button>
        <div class="grabbed-tip" v-if="limitReached">每人限购{{ product.limit_per_user || 1 }}件，您已达到上限，如需重新购买请先取消已有订单</div>
      </div>
    </div>

    <!-- 秒杀结果弹窗 -->
    <el-dialog v-model="resultVisible" title="秒杀结果" width="420px">
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
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { getProductDetail } from '@/api/goodsApi'
import { seckill as doSeckillApi, getSeckillPath, getCaptcha, getPurchasedCounts } from '@/api/seckillApi'
import CountDown from '@/components/Seckill/CountDown.vue'
import { ElMessage } from 'element-plus'
import { Refresh, ArrowLeft } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

const loading = ref(false)
const product = ref(null)
const skus = ref([])
const attrs = ref([])
// [修复] SKU 选择状态：attr.name -> 用户选中的值
const selected = reactive({})
const grabbedCount = ref(0)
const resultVisible = ref(false)
const seckillResult = ref(null)
const seckilling = ref(false)
// [创新] 数学验证码 + 秒杀地址隐藏
const captchaData = ref(null)
const captchaInput = ref('')
const captchaLoading = ref(false)
const pathToken = ref('')

// 是否已选择完整规格（所有属性都选了值）
const allSelected = computed(() => attrs.value.length > 0 && attrs.value.every(a => selected[a.name]))
// 匹配的 SKU：spec 完全等于用户所选组合
const matchedSKU = computed(() => {
  if (!allSelected.value) return null
  return skus.value.find(sku => {
    try {
      const spec = JSON.parse(sku.spec)
      return attrs.value.every(a => spec[a.name] === selected[a.name])
    } catch { return false }
  }) || null
})
// 展示价格：选中 SKU 时显示该配置价格，否则显示默认秒杀价
const displayPrice = computed(() => {
  if (matchedSKU.value) return matchedSKU.value.price
  return product.value?.seckill_price
})
const stockPercent = computed(() => {
  if (!product.value || !product.value.total_stock) return 0
  return Math.round(product.value.remain_stock / product.value.total_stock * 100)
})
const limitReached = computed(() => (grabbedCount.value || 0) >= (product.value?.limit_per_user || 1))
const canBuy = computed(() => product.value && product.value.remain_stock > 0 && !limitReached.value)
const isDisabled = computed(() => {
  if (!product.value) return true
  if (product.value.remain_stock <= 0) return true
  if (limitReached.value) return true
  const now = new Date()
  if (product.value.start_time && new Date(product.value.start_time) > now) return true
  if (product.value.end_time && new Date(product.value.end_time) < now) return true
  // [修复] 有规格的商品必须选满规格才能购买
  if (attrs.value.length > 0 && !matchedSKU.value) return true
  return false
})
const buttonText = computed(() => {
  if (!product.value) return '立即秒杀'
  if (product.value.remain_stock <= 0) return '已售罄'
  if (limitReached.value) return '已抢购'
  const now = new Date()
  if (product.value.start_time && new Date(product.value.start_time) > now) return '活动未开始'
  if (product.value.end_time && new Date(product.value.end_time) < now) return '活动已结束'
  if (attrs.value.length > 0 && !matchedSKU.value) return '请选择规格'
  return '立即秒杀'
})

function progressColor(pct) {
  if (pct > 50) return '#67c23a'
  if (pct > 20) return '#e6a23c'
  return '#e94560'
}

// [修复] 选择属性值（单选切换）
function selectAttr(name, value) {
  selected[name] = selected[name] === value ? undefined : value
}

async function loadDetail() {
  const id = Number(route.params.id)
  if (!id) return
  loading.value = true
  try {
    const data = await getProductDetail(id)
    product.value = data
    skus.value = (data?.skus || []).filter(s => s.status === 1)
    attrs.value = data?.attrs || []
    // 恢复限购状态 + 预取验证码/路径Token
    const tasks = [refreshCaptcha(), refreshPathToken()]
    try {
      const counts = await getPurchasedCounts([id])
      if (counts && typeof counts === 'object') {
        grabbedCount.value = Number(counts[id]) || 0
      }
    } catch (e) { console.error('恢复已购数量失败:', e) }
    await Promise.allSettled(tasks)
  } catch (e) {
    console.error('加载商品详情失败:', e)
    ElMessage.error('加载商品详情失败：' + (e?.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

async function refreshCaptcha() {
  const pid = product.value?.id
  if (!pid) return
  captchaLoading.value = true
  try {
    captchaData.value = await getCaptcha(pid)
    captchaInput.value = ''
  } catch (e) {
    console.error('获取验证码失败:', e)
  } finally {
    captchaLoading.value = false
  }
}

async function refreshPathToken() {
  const pid = product.value?.id
  if (!pid) return
  try {
    const data = await getSeckillPath(pid)
    pathToken.value = data.path_token
  } catch (e) {
    console.error('获取秒杀路径失败:', e)
  }
}

function generateIdempotentKey() {
  if (crypto && crypto.randomUUID) return crypto.randomUUID()
  return 'id-' + Date.now() + '-' + Math.random().toString(36).substring(2, 15)
}

async function doSeckill() {
  if (seckilling.value) return
  if (!product.value) return
  const pid = product.value.id

  // 前端前置校验：必须选满规格
  if (attrs.value.length > 0) {
    if (!allSelected.value) { ElMessage.warning('请选择完整商品规格'); return }
    if (!matchedSKU.value) { ElMessage.warning('该规格组合暂无货，请更换规格'); return }
  }

  if (!captchaData.value) {
    ElMessage.warning('验证码加载中，请稍候重试')
    refreshCaptcha()
    return
  }
  if (captchaInput.value === undefined || captchaInput.value === null || captchaInput.value === '') {
    ElMessage.warning('请输入验证码计算结果')
    return
  }
  const userAnswer = parseInt(captchaInput.value)
  if (isNaN(userAnswer)) {
    ElMessage.warning('验证码格式错误，请输入数字')
    return
  }
  if (!pathToken.value) {
    ElMessage.warning('秒杀地址已过期，请刷新页面')
    refreshPathToken()
    return
  }

  seckilling.value = true
  try {
    const data = await doSeckillApi({
      product_id: pid,
      quantity: 1,
      idempotent_key: generateIdempotentKey(),
      path_token: pathToken.value,
      captcha_code: userAnswer,
      captcha_id: captchaData.value.captcha_id,
      // [修复] 传递所选 SKU 配置：后端按配置覆盖价格（不同配置不同价格）
      sku_id: matchedSKU.value?.id || 0
    })
    seckillResult.value = data
    resultVisible.value = true
    if (data.status === 'queued') {
      grabbedCount.value = (grabbedCount.value || 0) + 1
      loadDetail()
    }
    refreshCaptcha()
    refreshPathToken()
  } catch (e) {
    console.error('秒杀请求失败:', e)
    const errMsg = e?.message || e?.toString() || ''
    if (errMsg.includes('已参与过') || errMsg.includes('限购')) {
      grabbedCount.value = product.value.limit_per_user || 1
      ElMessage.warning(`该商品每人限购${product.value.limit_per_user || 1}件，您已达到上限`)
    } else if (errMsg.includes('验证码') || errMsg.includes('算术')) {
      ElMessage.error('验证码错误或已过期，请重新计算')
      refreshCaptcha()
      refreshPathToken()
    } else if (errMsg.includes('地址') || errMsg.includes('路径') || errMsg.includes('配置')) {
      ElMessage.error(errMsg)
      refreshPathToken()
    } else if (errMsg.includes('库存')) {
      ElMessage.error('库存不足，秒杀失败')
    } else if (errMsg.includes('过多') || errMsg.includes('排队') || errMsg.includes('限流')) {
      ElMessage.error('当前抢购人数过多，请稍后重试')
    } else {
      ElMessage.error(`秒杀失败: ${errMsg || '未知错误'}`)
      refreshCaptcha()
      refreshPathToken()
    }
  } finally {
    seckilling.value = false
  }
}

onMounted(loadDetail)
watch(() => route.params.id, (nv) => { if (nv) loadDetail() })
</script>

<style scoped>
.product-detail { max-width: 1200px; margin: 0 auto }
.back-row { margin-bottom: 16px }
.detail-body { display: flex; gap: 24px; align-items: stretch }
.detail-img { width: 420px; min-height: 420px; display: flex; align-items: center; justify-content: center; background: rgba(233,69,96,.08); border-radius: 12px; overflow: hidden; flex-shrink: 0 }
.real-img { width: 100%; height: 100%; object-fit: contain; display: block }
.detail-info { flex: 1; padding: 32px }
.p-name { font-size: 24px; color: var(--text-primary); margin: 0 0 8px }
.p-desc { color: #999; font-size: 14px; margin: 0 0 16px; line-height: 1.6 }
.countdown-row { display: flex; align-items: center; gap: 8px; margin-bottom: 16px }
.price-row { display: flex; align-items: baseline; gap: 12px; margin-bottom: 20px; background: rgba(233,69,96,.06); padding: 12px 16px; border-radius: 8px }
.seckill-price { font-size: 36px; color: #e94560; font-weight: bold }
.original-price { font-size: 16px; color: #666; text-decoration: line-through }
/* [修复] SKU 属性选择器样式 */
.sku-section { margin-bottom: 20px }
.sku-group { margin-bottom: 14px }
.sku-label { font-size: 13px; color: var(--text-secondary); margin-bottom: 8px }
.sku-values { display: flex; flex-wrap: wrap; gap: 8px }
.sku-value-tag { cursor: pointer; font-size: 13px }
.sku-value-tag:hover { opacity: .85 }
.sku-match-tip { font-size: 12px; color: #e6a23c; margin-top: 4px }
.stock-row { margin-bottom: 16px }
.stock-text { font-size: 12px; color: #999; display: block; margin-top: 4px }
.captcha-row { margin-bottom: 12px }
.captcha-label { font-size: 12px; color: #888; display: block; margin-bottom: 4px }
.math-captcha-wrap { display: flex; align-items: center; gap: 8px }
.math-expr { display: inline-flex; align-items: center; justify-content: center; min-width: 80px; height: 32px; padding: 0 12px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: #fff; font-size: 16px; font-weight: bold; border-radius: 4px; cursor: pointer; user-select: none; letter-spacing: 2px; font-family: 'Courier New', monospace }
.math-expr:hover { opacity: 0.85 }
.captcha-input-field { width: 120px !important }
.buy-btn { width: 100% }
.grabbed-tip { font-size: 12px; color: #e94560; margin-top: 8px; text-align: center }
.order-info { text-align: left; margin: 12px 0 }
.order-info p { margin: 6px 0; color: var(--text-secondary) }
@media (max-width: 900px) {
  .detail-body { flex-direction: column }
  .detail-img { width: 100%; min-height: 260px }
}
</style>
