package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	MySQL      MySQLConfig      `mapstructure:"mysql"`
	Redis      RedisConfig      `mapstructure:"redis"`
	RabbitMQ   RabbitMQConfig   `mapstructure:"rabbitmq"`
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	Etcd       EtcdConfig       `mapstructure:"etcd"`
	Sentinel   SentinelConfig   `mapstructure:"sentinel"`
	Log        LogConfig        `mapstructure:"log"`
	DingTalk   DingTalkConfig   `mapstructure:"dingtalk"`
	Reconciler ReconcilerConfig `mapstructure:"reconciler"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	CORS       CORSConfig       `mapstructure:"cors"`
	Tracing    TracingConfig    `mapstructure:"tracing"` // [修复] Jaeger链路追踪配置
}

type ServerConfig struct {
	Name            string `mapstructure:"name"`
	Port            int    `mapstructure:"port"`
	Mode            string `mapstructure:"mode"`
	InstanceID      string `mapstructure:"instance_id"`
	ReadTimeout     int    `mapstructure:"read_timeout"`
	WriteTimeout    int    `mapstructure:"write_timeout"`
	IdleTimeout     int    `mapstructure:"idle_timeout"`  // [修复] 空闲连接超时（秒），防止慢客户端占用连接
	MaxBodyBytes    int64  `mapstructure:"max_body_bytes"` // [修复] 请求体最大字节数，默认 1MB
	SignSecret      string `mapstructure:"sign_secret"`   // [修复] API 签名密钥
	PerformanceMode bool   `mapstructure:"performance_mode"` // [优化] 压测模式：跳过非必要中间件
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
	Issuer      string `mapstructure:"issuer"`
}

type MySQLConfig struct {
	Master               MySQLNodeConfig   `mapstructure:"master"`
	Slaves               []MySQLNodeConfig `mapstructure:"slaves"`
	OrderTableShardCount int               `mapstructure:"order_table_shard_count"`
}

type MySQLNodeConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	Database        string `mapstructure:"database"`
	Charset         string `mapstructure:"charset"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

func (m *MySQLNodeConfig) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database, m.Charset)
}

type RedisConfig struct {
	Addrs        []string `mapstructure:"addrs"`
	Password     string   `mapstructure:"password"`
	PoolSize     int      `mapstructure:"pool_size"`
	MinIdleConns int      `mapstructure:"min_idle_conns"`
	DialTimeout  int      `mapstructure:"dial_timeout"`
	ReadTimeout  int      `mapstructure:"read_timeout"`
	WriteTimeout int      `mapstructure:"write_timeout"`
}

type RabbitMQConfig struct {
	URLs            []string               `mapstructure:"urls"`
	Vhost           string                 `mapstructure:"vhost"`
	Heartbeat       int                    `mapstructure:"heartbeat"` // [修复] 心跳间隔（秒），防止连接被服务端关闭
	Exchange        RabbitMQExchangeConfig `mapstructure:"exchange"`
	Queue           RabbitMQQueueConfig    `mapstructure:"queue"`
	Consumer        RabbitMQConsumerConfig `mapstructure:"consumer"`
	DelayTTLMs      int                    `mapstructure:"delay_ttl_ms"`
	ChannelPoolSize int                    `mapstructure:"channel_pool_size"` // [P0-1] MQ Channel 连接池大小
}

type RabbitMQExchangeConfig struct {
	Order      string `mapstructure:"order"`
	DeadLetter string `mapstructure:"dead_letter"`
	Delay      string `mapstructure:"delay"` // [修复] 延迟队列交换机，类型 x-delayed-message
}

// [修复] 添加CORS安全配置结构体
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"` // 允许的域名白名单
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
}

type RabbitMQQueueConfig struct {
	Order      string `mapstructure:"order"`
	Delay      string `mapstructure:"delay"`
	DeadLetter string `mapstructure:"dead_letter"`
	DelayRoute string `mapstructure:"delay_route"` // [修复] 延迟队列路由键，用于绑定延迟交换机
}

type RabbitMQConsumerConfig struct {
	Concurrency int `mapstructure:"concurrency"`
	Prefetch    int `mapstructure:"prefetch"`
}

type KafkaConfig struct {
	Brokers  []string           `mapstructure:"brokers"`
	Topic    string             `mapstructure:"topic"`
	Producer KafkaProducerConfig `mapstructure:"producer"`
}

type KafkaProducerConfig struct {
	Acks            string `mapstructure:"acks"`
	Compression     string `mapstructure:"compression"`
	MaxMessageBytes int    `mapstructure:"max_message_bytes"`
	BatchSize       int    `mapstructure:"batch_size"`
	LingerMs        int    `mapstructure:"linger_ms"`
}

type EtcdConfig struct {
	Endpoints     []string `mapstructure:"endpoints"`
	DialTimeout   int      `mapstructure:"dial_timeout"`
	LeaseTTL      int64    `mapstructure:"lease_ttl"`
	ServicePrefix string   `mapstructure:"service_prefix"`
}

type SentinelConfig struct {
	Enabled        bool                         `mapstructure:"enabled"`
	Dashboard      string                       `mapstructure:"dashboard"`
	AppName        string                       `mapstructure:"app_name"`
	LogDir         string                       `mapstructure:"log_dir"`
	GlobalQPS      int                          `mapstructure:"global_qps"`
	UserQPS        int                          `mapstructure:"user_qps"`
	SeckillQPS     int                          `mapstructure:"seckill_qps"`
	CircuitBreaker SentinelCircuitBreakerConfig `mapstructure:"circuit_breaker"`
	HotParam       SentinelHotParamConfig       `mapstructure:"hot_param"`
}

type SentinelCircuitBreakerConfig struct {
	MaxRTMs           int     `mapstructure:"max_rt_ms"`
	MaxRTRatio        float64 `mapstructure:"max_rt_ratio"`
	MinRequestAmount  int     `mapstructure:"min_request_amount"`
	StatIntervalMs    int     `mapstructure:"stat_interval_ms"`
	RecoveryTimeoutMs int     `mapstructure:"recovery_timeout_ms"`
}

type SentinelHotParamConfig struct {
	Threshold   int `mapstructure:"threshold"`
	DurationSec int `mapstructure:"duration_sec"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
	// [修复] 日志文件配置
	MaxSize    int  `mapstructure:"max_size"`    // 单个日志文件最大大小（MB）
	MaxBackups int  `mapstructure:"max_backups"` // 最多保留的旧日志文件数
	MaxAge     int  `mapstructure:"max_age"`     // 最多保留天数
	Compress   bool `mapstructure:"compress"`    // 是否压缩旧日志
}

type DingTalkConfig struct {
	Enabled    bool                `mapstructure:"enabled"`
	WebhookURL string              `mapstructure:"webhook_url"`
	Secret     string              `mapstructure:"secret"`
	Alert      DingTalkAlertConfig `mapstructure:"alert"`
}

type DingTalkAlertConfig struct {
	ErrorRateThreshold    float64 `mapstructure:"error_rate_threshold"`
	QPSThreshold          int     `mapstructure:"qps_threshold"`
	MQBacklogThreshold    int     `mapstructure:"mq_backlog_threshold"`
	CircuitBreakerTrigger bool    `mapstructure:"circuit_breaker_trigger"`
}

type ReconcilerConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	IntervalSec int  `mapstructure:"interval_sec"`
	BatchSize   int  `mapstructure:"batch_size"`
}

type PrometheusConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	MetricsPath string `mapstructure:"metrics_path"`
	MetricsPort int    `mapstructure:"metrics_port"`
}

// [修复] TracingConfig Jaeger全链路追踪配置
type TracingConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Endpoint string `mapstructure:"endpoint"` // OTLP HTTP endpoint, e.g. "localhost:4318"
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMu     sync.RWMutex                 // 配置热更新读写锁
	onChangeFns  []func(*Config)              // 配置变更回调
	viperInstance *viper.Viper                // 保留 viper 实例，用于热更新回调
)

func GetConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalConfig
}

func LoadConfig(configPath string) (*Config, error) {
	var loadErr error
	configOnce.Do(func() {
		v := viper.New()
		if configPath == "" {
			v.AddConfigPath(".")
			v.AddConfigPath("./config")
		} else {
			v.SetConfigFile(configPath)
		}
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.SetEnvPrefix("MIAOSHA")
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
		if err := v.ReadInConfig(); err != nil {
			loadErr = fmt.Errorf("读取配置文件失败: %w", err)
			return
		}
		log.Printf("[Config] 已加载配置文件: %s", v.ConfigFileUsed())
		cfg := &Config{}
		if err := v.Unmarshal(cfg); err != nil {
			loadErr = fmt.Errorf("解析配置文件失败: %w", err)
			return
		}
		fillDefaults(cfg)
		if cfg.Server.InstanceID == "" {
			hostname, _ := os.Hostname()
			cfg.Server.InstanceID = fmt.Sprintf("%s:%d", hostname, cfg.Server.Port)
		}
		configMu.Lock()
		globalConfig = cfg
		configMu.Unlock()
		viperInstance = v // 保存 viper 实例，用于热更新

		// [修复] 启动时验证关键配置项，防止运行时报错
		if err := validateConfig(cfg); err != nil {
			loadErr = fmt.Errorf("配置验证失败: %w", err)
			return
		}
	})
	return globalConfig, loadErr
}

func fillDefaults(cfg *Config) {
	if cfg.Server.Port == 0 { cfg.Server.Port = 8080 }
	if cfg.Server.Mode == "" { cfg.Server.Mode = "release" }
	if cfg.Server.ReadTimeout == 0 { cfg.Server.ReadTimeout = 30 }
	if cfg.Server.WriteTimeout == 0 { cfg.Server.WriteTimeout = 30 }
	if cfg.Server.IdleTimeout == 0 { cfg.Server.IdleTimeout = 60 }   // [修复] 默认空闲超时 60 秒
	if cfg.Server.MaxBodyBytes == 0 { cfg.Server.MaxBodyBytes = 1048576 } // [修复] 默认 1MB 请求体限制
	if cfg.JWT.ExpireHours == 0 { cfg.JWT.ExpireHours = 24 }
	if cfg.MySQL.OrderTableShardCount == 0 { cfg.MySQL.OrderTableShardCount = 16 }
	if cfg.RabbitMQ.DelayTTLMs == 0 { cfg.RabbitMQ.DelayTTLMs = 1800000 }
	if cfg.RabbitMQ.ChannelPoolSize == 0 { cfg.RabbitMQ.ChannelPoolSize = 20 } // [P0-1] 默认连接池大小 20
	if cfg.RabbitMQ.Heartbeat == 0 { cfg.RabbitMQ.Heartbeat = 30 } // [修复] 默认心跳30秒
	if cfg.RabbitMQ.Consumer.Concurrency == 0 { cfg.RabbitMQ.Consumer.Concurrency = 10 }
	if cfg.RabbitMQ.Consumer.Prefetch == 0 { cfg.RabbitMQ.Consumer.Prefetch = 50 }
	if cfg.Reconciler.IntervalSec == 0 { cfg.Reconciler.IntervalSec = 300 }
	if cfg.Reconciler.BatchSize == 0 { cfg.Reconciler.BatchSize = 100 }
	if cfg.Prometheus.MetricsPath == "" { cfg.Prometheus.MetricsPath = "/metrics" }
	if cfg.Prometheus.MetricsPort == 0 { cfg.Prometheus.MetricsPort = 9090 }
	// [修复] 延迟队列交换机和路由键默认值
	if cfg.RabbitMQ.Exchange.Delay == "" { cfg.RabbitMQ.Exchange.Delay = "miaosha.delay.exchange" }
	if cfg.RabbitMQ.Queue.DelayRoute == "" { cfg.RabbitMQ.Queue.DelayRoute = "miaosha.order.delay" }
	// [修复] CORS 安全默认值，生产环境应在配置文件中明确指定
	if len(cfg.CORS.AllowedOrigins) == 0 { cfg.CORS.AllowedOrigins = []string{"http://localhost:3000", "http://localhost:5173"} }
	if len(cfg.CORS.AllowedMethods) == 0 { cfg.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"} }
	if len(cfg.CORS.AllowedHeaders) == 0 { cfg.CORS.AllowedHeaders = []string{"Content-Type", "Authorization", "X-Trace-ID"} }
}

// [修复] validateConfig 启动时验证关键配置项，防止因配置错误导致运行时 panic
// 仅验证运行时不可变的配置项（如 Secret、连接池最大值），可变的限流阈值等不在此验证
func validateConfig(cfg *Config) error {
	if cfg.JWT.Secret == "" {
		return fmt.Errorf("JWT Secret 不能为空，请在 config.yaml 中配置 jwt.secret")
	}
	if cfg.JWT.ExpireHours <= 0 || cfg.JWT.ExpireHours > 720 {
		return fmt.Errorf("JWT ExpireHours 不合法（当前值: %d），应在 1-720 之间", cfg.JWT.ExpireHours)
	}
	if cfg.Server.SignSecret == "" {
		return fmt.Errorf("API 签名密钥不能为空，请在 config.yaml 中配置 server.sign_secret")
	}
	if cfg.MySQL.Master.Host == "" {
		return fmt.Errorf("MySQL 主库地址不能为空，请在 config.yaml 中配置 mysql.master.host")
	}
	if cfg.MySQL.Master.MaxOpenConns > 0 && (cfg.MySQL.Master.MaxOpenConns < 10 || cfg.MySQL.Master.MaxOpenConns > 1000) {
		log.Printf("[Config] MySQL Master MaxOpenConns 不合法（%d），已自动跳过", cfg.MySQL.Master.MaxOpenConns)
	}
	if cfg.MySQL.Master.MaxIdleConns > 0 && (cfg.MySQL.Master.MaxIdleConns < 2 || cfg.MySQL.Master.MaxIdleConns > 100) {
		log.Printf("[Config] MySQL Master MaxIdleConns 不合法（%d），已自动跳过", cfg.MySQL.Master.MaxIdleConns)
	}
	if cfg.Redis.Addrs == nil || len(cfg.Redis.Addrs) == 0 {
		return fmt.Errorf("Redis 地址不能为空，请在 config.yaml 中配置 redis.addrs")
	}
	if cfg.RabbitMQ.URLs == nil || len(cfg.RabbitMQ.URLs) == 0 {
		return fmt.Errorf("RabbitMQ 连接地址不能为空，请在 config.yaml 中配置 rabbitmq.urls")
	}
	if cfg.RabbitMQ.Consumer.Concurrency < 1 || cfg.RabbitMQ.Consumer.Concurrency > 100 {
		return fmt.Errorf("RabbitMQ Concurrency 不合法（当前值: %d），应在 1-100 之间", cfg.RabbitMQ.Consumer.Concurrency)
	}
	if cfg.Prometheus.MetricsPort <= 0 || cfg.Prometheus.MetricsPort > 65535 {
		return fmt.Errorf("Prometheus MetricsPort 不合法（当前值: %d），应在 1-65535 之间", cfg.Prometheus.MetricsPort)
	}
	return nil
}

// WatchConfig 启动配置文件热更新监听，修改 config.yaml 后自动重载配置
// onChange 回调会在配置变更后被调用，用于通知其他模块配置已更新
// 注意：仅支持运行时可变配置（如限流阈值、日志级别），不可变配置（如端口、数据库连接）需重启生效
func WatchConfig(onChange func(*Config)) {
	if viperInstance == nil {
		log.Println("[Config] viper 实例未初始化，跳过配置热更新监听")
		return
	}

	// 注册变更回调
	if onChange != nil {
		onChangeFns = append(onChangeFns, onChange)
	}

	// Viper 原生文件监听，检测到配置文件变更后自动重载
	viperInstance.WatchConfig()
	viperInstance.OnConfigChange(func(e fsnotify.Event) {
		log.Printf("[Config] 检测到配置文件变更: %s，正在热更新...", e.Name)

		// 重新解析配置文件
		newCfg := &Config{}
		if err := viperInstance.Unmarshal(newCfg); err != nil {
			log.Printf("[Config] 配置文件热更新失败: %v", err)
			return
		}
		fillDefaults(newCfg)
		if newCfg.Server.InstanceID == "" {
			hostname, _ := os.Hostname()
			newCfg.Server.InstanceID = fmt.Sprintf("%s:%d", hostname, newCfg.Server.Port)
		}

		// 原子替换全局配置
		configMu.Lock()
		globalConfig = newCfg
		configMu.Unlock()

		log.Println("[Config] 配置热更新完成")

		// 通知所有注册的回调
		for _, fn := range onChangeFns {
			fn(newCfg)
		}
	})
}