<!-- [修复] 任务4: 图形验证码防刷组件 -->
<template>
  <div class="captcha-input">
    <div class="captcha-row">
      <el-input
        ref="inputRef"
        v-model="inputValue"
        placeholder="请输入验证码"
        maxlength="4"
        class="captcha-input-field"
        @input="onInput"
        @keyup.enter="onEnter"
      />
      <div class="captcha-canvas-wrap" @click="refreshCaptcha" title="点击刷新验证码">
        <canvas ref="canvasRef" :width="canvasWidth" :height="canvasHeight" class="captcha-canvas"></canvas>
        <span class="captcha-hint">点击刷新</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'

/* [修复] 任务4: Canvas 尺寸 */
const canvasWidth = 120
const canvasHeight = 40
const canvasRef = ref(null)
const inputRef = ref(null) // [修复] 输入框引用，用于自动聚焦
const inputValue = ref('')
/* [修复] 任务4: 存储当前验证码值 */
let captchaCode = ''

/* [修复] 任务4: 生成 4 位随机数字验证码 */
function generateCode() {
  let code = ''
  for (let i = 0; i < 4; i++) {
    code += Math.floor(Math.random() * 10)
  }
  return code
}

/* [修复] 任务4: 绘制验证码到 Canvas */
function drawCaptcha() {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  const w = canvasWidth
  const h = canvasHeight

  // 清空画布
  ctx.clearRect(0, 0, w, h)

  // [修复] 任务4: 绘制背景色
  ctx.fillStyle = 'rgba(20,20,40,0.9)'
  ctx.fillRect(0, 0, w, h)

  // [修复] 任务4: 绘制干扰线（随机 5 条）
  for (let i = 0; i < 5; i++) {
    ctx.beginPath()
    ctx.moveTo(Math.random() * w, Math.random() * h)
    ctx.lineTo(Math.random() * w, Math.random() * h)
    ctx.strokeStyle = `rgba(${Math.floor(Math.random() * 200 + 55)},${Math.floor(Math.random() * 200 + 55)},${Math.floor(Math.random() * 200 + 55)},0.5)`
    ctx.lineWidth = Math.random() * 1.5 + 0.5
    ctx.stroke()
  }

  // [修复] 任务4: 绘制噪点（随机 30 个）
  for (let i = 0; i < 30; i++) {
    ctx.fillStyle = `rgba(${Math.floor(Math.random() * 255)},${Math.floor(Math.random() * 255)},${Math.floor(Math.random() * 255)},0.6)`
    ctx.fillRect(Math.floor(Math.random() * w), Math.floor(Math.random() * h), 2, 2)
  }

  // [修复] 任务4: 绘制 4 位数字，每个数字随机旋转和偏移
  captchaCode = generateCode()
  const fontSize = 22
  ctx.font = `bold ${fontSize}px "Arial"`
  ctx.textBaseline = 'middle'

  for (let i = 0; i < 4; i++) {
    const char = captchaCode[i]
    const x = 15 + i * 25 + Math.random() * 6 - 3
    const y = h / 2 + Math.random() * 8 - 4
    const angle = (Math.random() - 0.5) * 0.5

    ctx.save()
    ctx.translate(x, y)
    ctx.rotate(angle)
    // [修复] 任务4: 每个数字使用随机颜色
    const r = Math.floor(Math.random() * 150 + 100)
    const g = Math.floor(Math.random() * 150 + 100)
    const b = Math.floor(Math.random() * 150 + 100)
    ctx.fillStyle = `rgb(${r},${g},${b})`
    ctx.fillText(char, 0, 0)
    ctx.restore()
  }
}

/* [修复] 刷新验证码：清空输入并自动聚焦 */
function refreshCaptcha() {
  inputValue.value = ''
  drawCaptcha()
  // [修复] 刷新后自动聚焦输入框，优化交互体验
  nextTick(() => {
    inputRef.value?.focus()
  })
}

/* [修复] 任务4: 输入时自动转为大写（实际是数字，无需转换） */
function onInput() {
  // 输入验证码时的处理
}

/* [修复] 回车键快捷提交 */
function onEnter() {
  // 由父组件的 validate() 处理，此处仅预留
}

/* [修复] 任务4: 暴露 validate() 方法供父组件调用 */
function validate() {
  if (!inputValue.value || inputValue.value.length !== 4) {
    return { valid: false, message: '请输入4位验证码' }
  }
  if (inputValue.value !== captchaCode) {
    refreshCaptcha() // 验证失败自动刷新
    return { valid: false, message: '验证码错误，请重新输入' }
  }
  return { valid: true, message: '验证通过' }
}

/* [修复] 任务4: 重置验证码（供外部调用） */
function reset() {
  inputValue.value = ''
  refreshCaptcha()
}

defineExpose({ validate, reset, refreshCaptcha })

onMounted(() => {
  nextTick(() => drawCaptcha())
})
</script>

<style scoped>
.captcha-input { width: 100% }
.captcha-row { display: flex; gap: 8px; align-items: center }
.captcha-input-field { flex: 1 }
.captcha-canvas-wrap { position: relative; cursor: pointer; flex-shrink: 0; border-radius: 6px; overflow: hidden; border: 1px solid rgba(255,255,255,.1); transition: all .3s }
.captcha-canvas-wrap:hover { border-color: rgba(233,69,96,.4); transform: scale(1.05) }
.captcha-canvas-wrap:active { transform: scale(0.95) }
.captcha-canvas { display: block }
.captcha-hint { position: absolute; bottom: 2px; left: 0; right: 0; text-align: center; font-size: 10px; color: rgba(255,255,255,.3); pointer-events: none }
</style>