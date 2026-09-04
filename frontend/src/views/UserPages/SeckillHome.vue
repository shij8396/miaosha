<template>
  <div class="page-container seckill-home">
    <div class="hero-section fade-in-up">
      <h1 class="gradient-text">限时秒杀</h1>
      <p>超值好货，手慢无！</p>
    </div>

    <div class="product-grid" v-loading="loading">
      <el-empty v-if="!loading && products.length === 0" description="暂无秒杀商品" />
      <div v-for="p in products" :key="p.id" class="product-card glass-card fade-in-up">
        <!-- [修复] 商品图片/名称可点击跳转详情页 -->
        <div class="product-img" @click="goDetail(p)" title="查看商品详情">
          <!-- [修复] 有图片时显示商品图片，无图片时显示默认图标 -->
          <img v-if="p.image_url" :src="p.image_url" :alt="p.name" class="product-real-img" @error="p.image_url = ''" />
          <el-icon v-else :size="48"><Goods /></el-icon>
        </div>
        <div class="product-info">
          <div class="product-name" @click="goDetail(p)" title="查看商品详情">{{ p.name }}</div>
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
          <!-- [创新] 秒杀前安全验证：数学验证码 + 秒杀地址隐藏 -->
          <div class="captcha-row" v-if="p.remain_stock > 0 && !isLimitReached(p)">
            <span class="captcha-label">安全验证</span>
            <div class="math-captcha-wrap">
              <span class="math-expr" @click="refreshCaptcha(p.id)" title="点击刷新验证码">
                {{ captchaData[p.id]?.expression || '点击获取' }}
              </span>
              <el-input
                v-model="captchaInputs[p.id]"
                placeholder="输入结果"
                maxlength="4"
                class="captcha-input-field"
                @keyup.enter="doSeckill(p)"
              />
              <el-button size="small" text @click="refreshCaptcha(p.id)" :loading="captchaLoading[p.id]">
                <el-icon><Refresh /></el-icon>
              </el-button>
            </div>
          </div>
          <el-button type="danger" size="large" style="width:100%;margin-top:12px" @click="doSeckill(p)" :disabled="isSeckillDisabled(p)" :loading="seckilling === p.id">
            {{ getSeckillButtonText(p) }}
          </el-button>
          <!-- [修复] 已抢购提示：动态显示限购数量 -->
          <div class="grabbed-tip" v-if="isLimitReached(p)">每人限购{{ p.limit_per_user || 1 }}件，如需重新购买请先取消已有订单</div>
          <div class="detail-link">
            <el-button text size="small" type="primary" @click="goDetail(p)">查看商品详情 &gt;</el-button>
          </div>
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
import { seckill as doSeckillApi, getSeckillPath, getCaptcha, getPurchasedCounts } from '@/api/seckillApi'
import CountDown from '@/components/Seckill/CountDown.vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'

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
// [创新] 数学验证码：存储每个商品的验证码数据和用户输入
const captchaData = reactive({})
const captchaInputs = reactive({})
const captchaLoading = reactive({})
// [创新] 秒杀地址隐藏：存储每个商品的 Path Token
const pathTokens = reactive({})

// [创新] 从后端获取数学验证码算式
async function refreshCaptcha(productId) {
  captchaLoading[productId] = true
  try {
    const data = await getCaptcha(productId)
    captchaData[productId] = data
    captchaInputs[productId] = ''
  } catch (e) {
    console.error('获取验证码失败:', e)
    ElMessage.warning('获取验证码失败，请稍后重试')
  } finally {
    captchaLoading[productId] = false
  }
}

// [创新] 从后端获取秒杀隐藏路径 Token
async function refreshPathToken(productId) {
  try {
    const data = await getSeckillPath(productId)
    pathTokens[productId] = data.path_token
  } catch (e) {
    console.error('获取秒杀路径失败:', e)
  }
}

// [修复] 跳转商品详情页
function goDetail(p) {
  router.push(`/product/${p.id}`)
}

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
    const tasks = []
    products.value.forEach(p => {
      if (p.remain_stock > 0) {
        tasks.push(refreshCaptcha(p.id))
        tasks.push(refreshPathToken(p.id))
      }
    })
    // [修复] 从后端恢复各商品已购数量（Redis限购计数），
    // 解决"秒杀后查看订单再返回首页，按钮状态丢失仍可点击"的问题
    if (products.value.length > 0) {
      tasks.push(restorePurchasedCounts())
    }
    await Promise.allSettled(tasks)
  } catch (e) { console.error('加载商品列表失败:', e) } finally { loading.value = false }
}

// [修复] 拉取用户已购数量并恢复 grabbedCounts（页面重新挂载/刷新后仍显示"已抢购"）
async function restorePurchasedCounts() {
  try {
    const ids = products.value.map(p => p.id)
    const counts = await getPurchasedCounts(ids)
    if (counts && typeof counts === 'object') {
      const restored = {}
      Object.entries(counts).forEach(([pid, cnt]) => {
        restored[pid] = Number(cnt) || 0
      })
      grabbedCounts.value = restored
    }
  } catch (e) {
    console.error('恢复已购数量失败:', e)
  }
}

function generateIdempotentKey() {
  if (crypto && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  return 'id-' + Date.now() + '-' + Math.random().toString(36).substring(2, 15)
}

async function doSeckill(product) {
  if (seckilling.value || isLimitReached(product)) return

  const captcha = captchaData[product.id]
  if (!captcha) {
    ElMessage.warning('验证码加载中，请稍候重试')
    refreshCaptcha(product.id)
    return
  }

  const rawInput = captchaInputs[product.id]
  if (rawInput === undefined || rawInput === null || rawInput === '') {
    ElMessage.warning('请输入验证码计算结果')
    return
  }
  const userAnswer = parseInt(rawInput)
  if (isNaN(userAnswer)) {
    ElMessage.warning('验证码格式错误，请输入数字')
    return
  }

  const pathToken = pathTokens[product.id]
  if (!pathToken) {
    ElMessage.warning('秒杀地址已过期，请刷新页面')
    refreshPathToken(product.id)
    return
  }

  seckilling.value = product.id
  try {
    const idempotentKey = generateIdempotentKey()
    const data = await doSeckillApi({
      product_id: product.id,
      quantity: 1,
      idempotent_key: idempotentKey,
      path_token: pathToken,
      captcha_code: userAnswer,
      captcha_id: captcha.captcha_id
    })
    seckillResult.value = data
    resultVisible.value = true
    if (data.status === 'queued') {
      grabbedCounts.value[product.id] = (grabbedCounts.value[product.id] || 0) + 1
      loadProducts()
    }
    refreshCaptcha(product.id)
    refreshPathToken(product.id)
  } catch (e) {
    console.error('秒杀请求失败:', e)
    const errMsg = e?.message || e?.toString() || ''
    if (errMsg.includes('已参与过') || errMsg.includes('限购')) {
      const limit = product.limit_per_user || 1
      grabbedCounts.value[product.id] = limit
      ElMessage.warning(`该商品每人限购${limit}件，您已达到上限`)
    } else if (errMsg.includes('验证码') || errMsg.includes('算术')) {
      ElMessage.error('验证码错误或已过期，请重新计算')
      refreshCaptcha(product.id)
      refreshPathToken(product.id)
    } else if (errMsg.includes('地址') || errMsg.includes('路径')) {
      ElMessage.error('秒杀地址已失效，请刷新页面重试')
      refreshCaptcha(product.id)
      refreshPathToken(product.id)
    } else if (errMsg.includes('库存')) {
      ElMessage.error('库存不足，秒杀失败')
    } else if (errMsg.includes('不在秒杀') || errMsg.includes('活动')) {
      ElMessage.warning('不在秒杀活动时间内')
    } else if (errMsg.includes('过多') || errMsg.includes('排队') || errMsg.includes('限流')) {
      ElMessage.error('当前抢购人数过多，请稍后重试')
    } else {
      ElMessage.error(`秒杀失败: ${errMsg || '未知错误'}`)
      refreshCaptcha(product.id)
      refreshPathToken(product.id)
    }
  } finally {
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
.product-img { width: 80px; height: 80px; display: flex; align-items: center; justify-content: center; background: rgba(233,69,96,.1); border-radius: 8px; overflow: hidden; flex-shrink: 0 }
/* [修复] 商品真实图片样式：填满图片区域并保持比例 */
.product-real-img { width: 100%; height: 100%; object-fit: cover; display: block }
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
/* [修复] 详情跳转：图片/名称可点击 + 详情链接样式 */
.product-img, .product-name { cursor: pointer }
.detail-link { text-align: center; margin-top: 4px }
/* [修复] 任务4: 验证码区域样式 */
.captcha-row { margin-top: 8px; margin-bottom: 4px }
.captcha-label { font-size: 12px; color: #888; display: block; margin-bottom: 4px }
/* [创新] 数学验证码样式 */
.math-captcha-wrap { display: flex; align-items: center; gap: 8px }
.math-expr { display: inline-flex; align-items: center; justify-content: center; min-width: 80px; height: 32px; padding: 0 12px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: #fff; font-size: 16px; font-weight: bold; border-radius: 4px; cursor: pointer; user-select: none; letter-spacing: 2px; font-family: 'Courier New', monospace }
.math-expr:hover { opacity: 0.85 }
.captcha-input-field { width: 100px !important }
.order-info { text-align: left; margin: 12px 0 }
.order-info p { margin: 6px 0; color: var(--text-secondary) }
</style>