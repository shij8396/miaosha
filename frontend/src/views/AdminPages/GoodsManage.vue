<template>
  <div class="page-container">
    <h2 class="page-title">商品管理</h2>
    <CardGlass>
      <DataTable :data="filteredProducts" :loading="loading" :total="total" @page-change="onPageChange" @size-change="onSizeChange">
        <template #toolbar>
          <el-button type="danger" @click="showAddDialog">新增商品</el-button>
          <!-- [修复] 任务7: 批量导入商品 -->
          <el-upload
            :auto-upload="false"
            :show-file-list="false"
            accept=".xlsx,.xls,.csv"
            :on-change="handleImportProducts"
          >
            <el-button>批量导入商品</el-button>
          </el-upload>
          <!-- [修复] 任务7: 导出商品 -->
          <el-button @click="exportProducts">导出商品</el-button>
          <el-input v-model="searchKey" placeholder="搜索商品" style="width:200px" clearable />
        </template>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="商品名称" />
        <el-table-column prop="price" label="原价" width="100" />
        <el-table-column prop="seckill_price" label="秒杀价" width="100" />
        <el-table-column prop="remain_stock" label="剩余库存" width="100" />
        <el-table-column prop="total_stock" label="总库存" width="80" />
        <el-table-column prop="status" label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">{{ row.status === 1 ? '上架' : '下架' }}</el-tag>
          </template>
        </el-table-column>
        <!-- [修复] 商品图片预览列 -->
        <el-table-column label="图片" width="80">
          <template #default="{ row }">
            <el-image v-if="row.image_url" :src="row.image_url" style="width:40px;height:40px;border-radius:4px" fit="cover" :preview-src-list="[row.image_url]" />
            <span v-else style="color:#999;font-size:12px">无图片</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160">
          <template #default="{ row }">
            <el-button size="small" link @click="showEditDialog(row)">编辑</el-button>
            <el-button size="small" link :type="row.status===1?'warning':'success'" @click="toggleStatus(row)">
              {{ row.status===1?'下架':'上架' }}
            </el-button>
          </template>
        </el-table-column>
      </DataTable>
    </CardGlass>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑商品' : '新增商品'" width="720px">
      <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
        <el-form-item label="商品名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <!-- [修复] 商品图片上传 -->
        <el-form-item label="商品图片">
          <div class="upload-area">
            <el-upload
              :auto-upload="false"
              :show-file-list="false"
              accept="image/*"
              :on-change="handleImageChange"
              drag
            >
              <img v-if="imagePreview" :src="imagePreview" class="upload-preview" />
              <div v-else class="upload-placeholder">
                <el-icon :size="32"><Upload /></el-icon>
                <div class="upload-text">点击或拖拽上传图片</div>
              </div>
            </el-upload>
            <el-button v-if="imagePreview" size="small" type="danger" link @click="clearImage" style="margin-top:6px">清除图片</el-button>
          </div>
        </el-form-item>
        <el-form-item label="原价" prop="price">
          <el-input-number v-model="form.price" :min="0" :precision="2" style="width:100%" />
        </el-form-item>
        <el-form-item label="秒杀价" prop="seckill_price">
          <el-input-number v-model="form.seckill_price" :min="0" :precision="2" style="width:100%" />
        </el-form-item>
        <el-form-item label="总库存" prop="total_stock">
          <el-input-number v-model="form.total_stock" :min="1" style="width:100%" />
        </el-form-item>
        <el-form-item label="活动时间" required>
          <el-date-picker v-model="timeRange" type="datetimerange" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" format="YYYY-MM-DD HH:mm:ss" value-format="YYYY-MM-DD HH:mm:ss" style="width:100%" />
        </el-form-item>
        <el-form-item label="限购数量">
          <el-input-number v-model="form.limit_per_user" :min="1" :max="99" style="width:100%" />
        </el-form-item>
        <!-- [修复] 商品配置（SKU）：多规格属性 + 每个配置独立价格/库存 -->
        <el-form-item label="商品配置">
          <el-switch v-model="skuEnabled" active-text="多规格" inactive-text="单规格" />
        </el-form-item>
        <template v-if="skuEnabled">
          <el-form-item label="规格属性">
            <div class="sku-attrs">
              <div v-for="(attr, idx) in attrRows" :key="idx" class="attr-row">
                <el-input v-model="attr.name" placeholder="属性名（如 颜色/存储/尺码）" style="width:160px" />
                <el-input v-model="attr.values" placeholder="属性值，逗号分隔（如 黑,白,蓝）" style="flex:1" />
                <el-button text type="danger" @click="attrRows.splice(idx, 1)">删除</el-button>
              </div>
              <div class="attr-actions">
                <el-button size="small" @click="attrRows.push({ name: '', values: '' })">+ 添加属性</el-button>
                <el-button size="small" type="primary" @click="generateSkus">生成规格组合</el-button>
              </div>
              <div class="attr-tip">示例（手机）：属性「存储」值 128G,256G；属性「颜色」值 黑,白 —— 生成 4 个配置各自定价</div>
            </div>
          </el-form-item>
          <el-form-item label="配置明细">
            <div class="sku-table" v-if="skuRows.length > 0">
              <div class="sku-row sku-head">
                <span class="c-spec">规格组合</span><span class="c-price">该配置价格</span><span class="c-stock">展示库存</span><span class="c-op">操作</span>
              </div>
              <div v-for="(row, idx) in skuRows" :key="idx" class="sku-row">
                <span class="c-spec">{{ specText(row.spec) }}</span>
                <el-input-number v-model="row.price" :min="0.01" :precision="2" :controls="false" size="small" style="width:120px" />
                <el-input-number v-model="row.stock" :min="0" :controls="false" size="small" style="width:100px" />
                <el-button text type="danger" size="small" @click="skuRows.splice(idx, 1)">删除</el-button>
              </div>
            </div>
            <div v-else class="attr-tip">点击「生成规格组合」或编辑已有商品时自动载入</div>
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="danger" @click="handleSave" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { getProductList, createProduct, updateProduct, batchImportProducts, uploadImage, getProductDetail } from '@/api/goodsApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage } from 'element-plus'
import * as XLSX from 'xlsx'

const products = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const searchKey = ref('')
const dialogVisible = ref(false)
const isEdit = ref(false)
const saving = ref(false)
const formRef = ref(null)
const timeRange = ref([])
const editId = ref(null)
// [修复] 图片上传状态
const imageFile = ref(null)
const imagePreview = ref('')
// [修复] SKU 多规格配置状态：属性行 + 规格组合明细
const skuEnabled = ref(false)
const attrRows = ref([])
const skuRows = ref([])

// [修复] 规格组合文本展示：{"颜色":"黑","存储":"256G"} → 颜色:黑 存储:256G
function specText(spec) {
  if (!spec) return ''
  return Object.entries(spec).map(([k, v]) => `${k}:${v}`).join(' ')
}

// [修复] 依据属性行生成笛卡尔积规格组合（上限 50 组合防误操作）
function generateSkus() {
  const valid = attrRows.value
    .map(a => ({ name: (a.name || '').trim(), values: (a.values || '').split(/[，,]/).map(v => v.trim()).filter(Boolean) }))
    .filter(a => a.name && a.values.length > 0)
  if (valid.length === 0) {
    ElMessage.warning('请先填写至少一行有效的规格属性（属性名 + 属性值）')
    return
  }
  let combos = [{}]
  for (const attr of valid) {
    const next = []
    for (const base of combos) {
      for (const v of attr.values) {
        next.push({ ...base, [attr.name]: v })
      }
    }
    combos = next
    if (combos.length > 50) { combos = combos.slice(0, 50); break }
  }
  // 保留已存在组合的价格/库存，新生成的默认用秒杀价
  const oldMap = new Map(skuRows.value.map(r => [JSON.stringify(r.spec), r]))
  skuRows.value = combos.map(spec => {
    const old = oldMap.get(JSON.stringify(spec))
    return old ? { ...old, spec } : { spec, price: form.seckill_price || 0.01, stock: 0 }
  })
  ElMessage.success(`已生成 ${skuRows.value.length} 个规格组合`)
}

// [修复] 编辑时从商品详情载入已有 SKU 配置
async function loadSKUs(productId) {
  skuEnabled.value = false
  attrRows.value = []
  skuRows.value = []
  try {
    const detail = await getProductDetail(productId)
    const skus = (detail?.skus || []).filter(s => s.status === 1)
    if (skus.length > 0) {
      skuEnabled.value = true
      skuRows.value = skus.map(s => {
        let spec = {}
        try { spec = JSON.parse(s.spec) } catch { /* 兼容脏数据 */ }
        return { spec, price: s.price, stock: s.stock }
      })
      // 从详情 attrs 反推属性行，便于继续追加属性后重新生成
      attrRows.value = (detail?.attrs || []).map(a => ({ name: a.name, values: (a.values || []).join(',') }))
    }
  } catch (e) {
    console.error('载入SKU配置失败:', e)
  }
}

// 客户端搜索过滤（在当前页内过滤）
const filteredProducts = computed(() => {
  if (!searchKey.value.trim()) return products.value
  const keyword = searchKey.value.trim().toLowerCase()
  return products.value.filter(p =>
    p.name?.toLowerCase().includes(keyword) ||
    String(p.id).includes(keyword)
  )
})

const form = reactive({
  name: '', description: '', price: 0, seckill_price: 0, total_stock: 100,
  start_time: '', end_time: '', limit_per_user: 1
})
const rules = {
  name: [{ required: true, message: '请输入商品名称', trigger: 'blur' }],
  price: [{ required: true, message: '请输入原价', trigger: 'blur' }],
  seckill_price: [{ required: true, message: '请输入秒杀价', trigger: 'blur' }],
  total_stock: [{ required: true, message: '请输入库存', trigger: 'blur' }]
}

async function loadProducts() {
  loading.value = true
  try {
    const data = await getProductList({ page: page.value, page_size: pageSize.value })
    products.value = data?.list || []
    total.value = data?.total || 0
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('加载商品列表失败:', e) } finally { loading.value = false }
}

function onPageChange(p) { page.value = p; loadProducts() }
function onSizeChange(s) { pageSize.value = s; loadProducts() }

function showAddDialog() {
  isEdit.value = false; editId.value = null
  Object.assign(form, { name: '', description: '', price: 0, seckill_price: 0, total_stock: 100, limit_per_user: 1 })
  timeRange.value = []
  clearImage()
  // [修复] 重置 SKU 配置状态
  skuEnabled.value = false
  attrRows.value = [{ name: '', values: '' }]
  skuRows.value = []
  dialogVisible.value = true
}

function showEditDialog(row) {
  isEdit.value = true; editId.value = row.id
  Object.assign(form, {
    name: row.name, description: row.description, price: row.price,
    seckill_price: row.seckill_price, total_stock: row.total_stock, limit_per_user: row.limit_per_user
  })
  // [修复] 后端返回 ISO 格式时间（如 2026-01-01T00:00:00+08:00），需转换为 YYYY-MM-DD HH:mm:ss 供 date-picker 和后端使用
  const fmtTime = (t) => {
    if (!t) return ''
    const d = new Date(t)
    if (isNaN(d.getTime())) return t
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }
  timeRange.value = [fmtTime(row.start_time), fmtTime(row.end_time)]
  // [修复] 编辑时显示已有图片
  imagePreview.value = row.image_url || ''
  imageFile.value = null
  // [修复] 编辑时异步载入已有 SKU 配置
  loadSKUs(row.id)
  dialogVisible.value = true
}

// [修复] 图片上传处理
function handleImageChange(file) {
  const isImage = file.raw?.type?.startsWith('image/')
  if (!isImage) { ElMessage.warning('请选择图片文件'); return }
  const isLt2M = file.raw.size / 1024 / 1024 < 2
  if (!isLt2M) { ElMessage.warning('图片大小不能超过 2MB'); return }
  imageFile.value = file.raw
  imagePreview.value = URL.createObjectURL(file.raw)
}

function clearImage() {
  imageFile.value = null
  imagePreview.value = ''
}

async function handleSave() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  if (!timeRange.value || timeRange.value.length !== 2) {
    ElMessage.warning('请选择活动时间')
    return
  }
  saving.value = true
  try {
    const payload = { ...form, start_time: timeRange.value[0], end_time: timeRange.value[1] }
    // [修复] 组装 SKU 配置：启用多规格时提交组合明细；关闭多规格时提交空数组清空配置
    if (skuEnabled.value) {
      if (skuRows.value.length === 0) {
        ElMessage.warning('已启用多规格，请先「生成规格组合」并填写价格')
        saving.value = false
        return
      }
      const invalid = skuRows.value.find(r => !r.price || r.price <= 0)
      if (invalid) {
        ElMessage.warning(`规格「${specText(invalid.spec)}」价格必须大于 0`)
        saving.value = false
        return
      }
      payload.skus = skuRows.value.map(r => ({ spec: r.spec, price: r.price, stock: r.stock || 0 }))
    } else if (isEdit.value) {
      payload.skus = [] // 编辑时显式清空旧配置
    }
    // [修复] 上传图片并获取URL
    if (imageFile.value) {
      try {
        const uploadRes = await uploadImage(imageFile.value)
        payload.image_url = uploadRes?.url || uploadRes?.data?.url || ''
      } catch (e) {
        console.error('图片上传失败:', e)
        ElMessage.warning('图片上传失败，将保存商品信息但不包含图片')
      }
    }
    if (isEdit.value) {
      await updateProduct(editId.value, payload)
      ElMessage.success('更新成功')
    } else {
      await createProduct(payload)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    loadProducts()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('保存商品失败:', e) } finally { saving.value = false }
}

async function toggleStatus(row) {
  try {
    await updateProduct(row.id, { status: row.status === 1 ? 0 : 1 })
    ElMessage.success('操作成功')
    loadProducts()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('切换商品状态失败:', e) }
}

/* [修复] 任务7: 批量导入商品 - 读取 Excel/CSV 文件 */
async function handleImportProducts(file) {
  try {
    const data = await file.raw.arrayBuffer()
    const workbook = XLSX.read(data, { type: 'array' })
    const sheetName = workbook.SheetNames[0]
    const worksheet = workbook.Sheets[sheetName]
    const jsonData = XLSX.utils.sheet_to_json(worksheet)

    if (!jsonData || jsonData.length === 0) {
      ElMessage.warning('文件中没有数据')
      return
    }

    // 调用批量导入接口
    await batchImportProducts(jsonData)
    ElMessage.success(`导入完成，共 ${jsonData.length} 条商品`)
    loadProducts()
  } catch (e) {
    ElMessage.error('文件解析失败，请检查文件格式')
    console.error('导入商品异常:', e)
  }
}

/* [修复] 任务7: 导出商品为 CSV */
function exportProducts() {
  try {
    const headers = ['ID', '商品名称', '原价', '秒杀价', '剩余库存', '总库存', '状态']
    const rows = products.value.map(p => [
      p.id,
      p.name,
      p.price,
      p.seckill_price,
      p.remain_stock,
      p.total_stock,
      p.status === 1 ? '上架' : '下架'
    ])
    const bom = '\uFEFF'
    const csvContent = bom + [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', `商品列表_${new Date().toISOString().slice(0, 10)}.csv`)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
    ElMessage.success('导出成功')
  } catch (e) {
    ElMessage.error('导出失败')
  }
}

onMounted(loadProducts)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
/* [修复] SKU 配置编辑区域样式 */
.sku-attrs { width: 100% }
.attr-row { display: flex; align-items: center; gap: 8px; margin-bottom: 8px }
.attr-actions { display: flex; gap: 8px; margin-bottom: 8px }
.attr-tip { font-size: 12px; color: #999; line-height: 1.5 }
.sku-table { width: 100%; border: 1px solid var(--border-color, #ebeef5); border-radius: 6px; padding: 4px 12px }
.sku-row { display: flex; align-items: center; gap: 12px; padding: 8px 0; border-bottom: 1px dashed var(--border-color, #ebeef5) }
.sku-row:last-child { border-bottom: none }
.sku-head { color: #999; font-size: 12px }
.sku-head .c-spec, .sku-head .c-price, .sku-head .c-stock, .sku-head .c-op { font-weight: normal }
.c-spec { flex: 1; font-size: 13px; color: var(--text-primary) }
.c-price, .c-stock { width: 130px; flex-shrink: 0 }
.c-op { width: 60px; flex-shrink: 0 }
/* [修复] 图片上传区域样式 */
.upload-area { width: 100% }
.upload-preview { width: 100%; max-height: 180px; object-fit: contain; border-radius: 8px }
.upload-placeholder { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 20px 0; color: #999 }
.upload-text { margin-top: 8px; font-size: 13px; color: #999 }
</style>