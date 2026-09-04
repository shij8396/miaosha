import LayoutView from '@/components/Layout/LayoutView.vue'

export const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/UserPages/Login.vue'),
    meta: { title: '登录' }
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('@/views/UserPages/Register.vue'),
    meta: { title: '注册' }
  },
  {
    path: '/',
    component: LayoutView,
    redirect: '/seckill',
    children: [
      {
        path: 'seckill',
        name: 'SeckillHome',
        component: () => import('@/views/UserPages/SeckillHome.vue'),
        meta: { title: '秒杀首页', icon: 'Shop' }
      },
      // [修复] 商品详情页路由：秒杀首页商品卡片点击跳转，携带商品ID
      {
        path: 'product/:id',
        name: 'ProductDetail',
        component: () => import('@/views/UserPages/ProductDetail.vue'),
        meta: { title: '商品详情' }
      },
      {
        path: 'orders',
        name: 'MyOrder',
        component: () => import('@/views/UserPages/MyOrder.vue'),
        meta: { title: '我的订单', icon: 'Tickets' }
      },
      // [修复] 运营管理组 - 路由 meta.role 必须匹配角色分组名，而非写死 'admin'
      {
        path: 'admin/goods',
        name: 'GoodsManage',
        component: () => import('@/views/AdminPages/GoodsManage.vue'),
        meta: { title: '商品管理', icon: 'Goods', role: 'operator' }
      },
      {
        path: 'admin/activity',
        name: 'ActivityConfig',
        component: () => import('@/views/AdminPages/ActivityConfig.vue'),
        meta: { title: '活动配置', icon: 'Timer', role: 'operator' }
      },
      {
        path: 'admin/orders',
        name: 'OrderManage',
        component: () => import('@/views/AdminPages/OrderManage.vue'),
        meta: { title: '订单管理', icon: 'Document', role: 'operator' }
      },
      {
        path: 'admin/stock',
        name: 'StockCheck',
        component: () => import('@/views/AdminPages/StockCheck.vue'),
        meta: { title: '库存对账', icon: 'Check', role: 'operator' }
      },
      // [修复] 风控管理组
      {
        path: 'admin/sentinel',
        name: 'SentinelRule',
        component: () => import('@/views/AdminPages/SentinelRule.vue'),
        meta: { title: '限流规则', icon: 'Setting', role: 'risk_control' }
      },
      {
        path: 'admin/blacklist',
        name: 'BlackList',
        component: () => import('@/views/AdminPages/BlackList.vue'),
        meta: { title: '黑名单', icon: 'Warning', role: 'risk_control' }
      },
      {
        path: 'admin/roles',
        name: 'UserRole',
        component: () => import('@/views/AdminPages/UserRole.vue'),
        meta: { title: '权限管理', icon: 'User', role: 'risk_control' }
      },
      // [修复] 监控大屏组
      {
        path: 'screen',
        name: 'BigScreen',
        component: () => import('@/views/DataScreen/SeckillBigScreen.vue'),
        meta: { title: '数据大屏', icon: 'DataAnalysis', role: 'monitor_readonly' }
      },
      {
        path: 'monitor',
        name: 'MonitorPanel',
        component: () => import('@/views/DataScreen/MonitorPanel.vue'),
        meta: { title: '服务监控', icon: 'Monitor', role: 'monitor_readonly' }
      }
    ]
  }
]