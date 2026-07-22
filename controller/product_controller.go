package controller

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

type ProductController struct {
	productService *service.ProductService
}

func NewProductController(productService *service.ProductService) *ProductController {
	return &ProductController{productService: productService}
}

// CreateProduct 创建商品
// @Summary      创建商品
// @Description  创建新的秒杀商品，自动预热 Redis 库存
// @Tags         商品模块
// @Accept       json
// @Produce      json
// @Param        request body model.CreateProductRequest true "商品创建请求"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=model.Product}  "创建成功"
// @Failure      400  {object}  utils.Response  "参数错误"
// @Router       /api/v1/product [post]
func (ctl *ProductController) CreateProduct(c *gin.Context) {
	var req model.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	product, err := ctl.productService.CreateProduct(&req)
	if err != nil { utils.Error(c, 400, err.Error()); return }
	utils.SuccessWithMessage(c, "创建成功", product)
}

func (ctl *ProductController) UpdateProduct(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { utils.BadRequest(c, "商品ID格式错误"); return }
	var req model.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	if err := ctl.productService.UpdateProduct(id, &req); err != nil { utils.Error(c, 400, err.Error()); return }
	utils.SuccessWithMessage(c, "更新成功", nil)
}

// GetProductList 获取商品列表（分页）
// @Summary      获取商品列表
// @Description  分页查询所有商品，支持按状态筛选
// @Tags         商品模块
// @Accept       json
// @Produce      json
// @Param        page      query     int  false  "页码"  default(1)
// @Param        page_size query     int  false  "每页数量"  default(10)
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=object{list=[]model.Product,total=int,page=int,page_size=int}}  "查询成功"
// @Router       /api/v1/product/list [get]
func (ctl *ProductController) GetProductList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	// [修复] 分页上限限制
	pageSize = utils.ClampPageSize(pageSize)
	products, total, err := ctl.productService.GetProductList(page, pageSize)
	if err != nil { utils.InternalError(c, err.Error()); return }
	utils.Success(c, gin.H{"list": products, "total": total, "page": page, "page_size": pageSize})
}

// GetActiveProducts 获取正在秒杀的商品
// @Summary      获取正在秒杀的商品
// @Description  查询当前时间范围内状态为上架且库存大于 0 的秒杀商品
// @Tags         商品模块
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=[]model.Product}  "查询成功"
// @Router       /api/v1/product/active [get]
func (ctl *ProductController) GetActiveProducts(c *gin.Context) {
	products, err := ctl.productService.GetActiveProducts()
	if err != nil { utils.InternalError(c, err.Error()); return }
	utils.Success(c, products)
}

// GetProductDetail 获取商品详情
// @Summary      获取商品详情
// @Description  根据商品 ID 查询商品详细信息
// @Tags         商品模块
// @Accept       json
// @Produce      json
// @Param        id   path      int64  true  "商品 ID"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=model.Product}  "查询成功"
// @Failure      400  {object}  utils.Response  "商品ID格式错误"
// @Failure      404  {object}  utils.Response  "商品不存在"
// @Router       /api/v1/product/{id} [get]
func (ctl *ProductController) GetProductDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { utils.BadRequest(c, "商品ID格式错误"); return }
	product, err := ctl.productService.GetProductDetail(id)
	if err != nil { utils.NotFound(c, "商品不存在"); return }
	utils.Success(c, product)
}

// [修复] 商品批量导入 - POST /api/v1/product/batch
// 接收前端传入的 JSON 数组，逐条创建商品并自动同步 Redis 库存
func (ctl *ProductController) BatchImportProducts(c *gin.Context) {
	var reqs []model.CreateProductRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if len(reqs) == 0 {
		utils.BadRequest(c, "导入数据为空")
		return
	}
	successCount, failCount := ctl.productService.BatchImportProducts(reqs)
	utils.SuccessWithMessage(c, "批量导入完成", gin.H{
		"success_count": successCount,
		"fail_count":    failCount,
		"total":         len(reqs),
	})
}

// [修复] UploadImage 商品图片上传 - POST /api/v1/product/upload
// 支持 multipart/form-data 上传，存储到本地 ./uploads 目录
// 返回图片访问 URL
func (ctl *ProductController) UploadImage(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "请选择上传文件")
		return
	}
	// 限制文件大小（最大 5MB）
	if file.Size > 5*1024*1024 {
		utils.BadRequest(c, "文件大小不能超过 5MB")
		return
	}
	// 只允许图片格式
	ext := filepath.Ext(file.Filename)
	allowedExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
	if !allowedExts[ext] {
		utils.BadRequest(c, "不支持的图片格式，仅支持 jpg/jpeg/png/gif/webp")
		return
	}
	// 确保上传目录存在
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		utils.InternalError(c, "创建上传目录失败")
		return
	}
	// 生成唯一文件名
	newFileName := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), utils.GenerateTraceID()[:8], ext)
	savePath := filepath.Join(uploadDir, newFileName)
	// 保存文件
	src, err := file.Open()
	if err != nil {
		utils.InternalError(c, "读取文件失败")
		return
	}
	defer src.Close()
	dst, err := os.Create(savePath)
	if err != nil {
		utils.InternalError(c, "创建文件失败")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		utils.InternalError(c, "保存文件失败")
		return
	}
	// 返回图片 URL（相对路径，前端可通过 Nginx 或静态文件服务访问）
	imageURL := fmt.Sprintf("/uploads/%s", newFileName)
	utils.SuccessWithMessage(c, "上传成功", gin.H{"url": imageURL, "filename": newFileName})
}