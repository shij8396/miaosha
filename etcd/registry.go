package etcd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/log"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	etcdClient *clientv3.Client
	leaseID    clientv3.LeaseID
	regOnce    sync.Once
)

func Init(cfg *config.EtcdConfig) error {
	var initErr error
	regOnce.Do(func() {
		cli, err := clientv3.New(clientv3.Config{Endpoints: cfg.Endpoints, DialTimeout: time.Duration(cfg.DialTimeout) * time.Second})
		if err != nil {
			initErr = fmt.Errorf("Etcd连接失败: %w", err)
			return
		}
		etcdClient = cli
		log.L().Info("Etcd客户端初始化成功")
	})
	return initErr
}

func RegisterService(cfg *config.ServerConfig, etcdCfg *config.EtcdConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(etcdCfg.DialTimeout)*time.Second)
	defer cancel()
	resp, err := etcdClient.Grant(ctx, etcdCfg.LeaseTTL)
	if err != nil {
		return fmt.Errorf("创建Etcd租约失败: %w", err)
	}
	leaseID = resp.ID
	key := fmt.Sprintf("%s%s", etcdCfg.ServicePrefix, cfg.InstanceID)
	// [修复] 从环境变量 POD_IP 读取服务注册地址（K8s环境），未设置时 fallback 到 127.0.0.1
	ip := os.Getenv("POD_IP")
	if ip == "" {
		ip = "127.0.0.1"
		log.L().Warn("POD_IP 环境变量未设置，使用默认地址 127.0.0.1 注册服务")
	}
	value := fmt.Sprintf("%s:%d", ip, cfg.Port)
	_, err = etcdClient.Put(ctx, key, value, clientv3.WithLease(leaseID))
	if err != nil {
		return fmt.Errorf("服务注册到Etcd失败: %w", err)
	}
	go keepAlive(key, etcdCfg.LeaseTTL)
	log.L().Infow("服务已注册到Etcd", "key", key, "value", value)
	return nil
}

func keepAlive(key string, ttl int64) {
	ticker := time.NewTicker(time.Duration(ttl/3) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		etcdClient.KeepAliveOnce(ctx, leaseID)
		cancel()
	}
}

func Deregister(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := etcdClient.Delete(ctx, key)
	return err
}

func GetClient() *clientv3.Client { return etcdClient }
func Close() error {
	if etcdClient != nil {
		return etcdClient.Close()
	}
	return nil
}
