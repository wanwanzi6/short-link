package db

import (
	"log"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/wanwanzi6/short-link/internal/model"
)

// Filter 是全局布隆过滤器实例
// 用于快速判断 short_code 是否可能存在，防止缓存穿透
var Filter *bloom.BloomFilter

// InitBloomFilter 初始化布隆过滤器
//
// 参数说明：
//   - capacity: 1000000 (100万) 预估存储的短码数量
//   - fpRate: 0.0001 (0.01%) 误报率
//
// 布隆过滤器原理：
//   - 使用多个哈希函数，将元素映射到位数组的多个位置
//   - 添加元素后，这些位置被标记为 1
//   - 查询时，如果所有哈希位置都是 1，说明元素可能存在
//   - 存在 false positive（不存在但判断为存在），但不会 false negative
//
// 容量与误报率关系：
//   - 100万容量 + 0.01% 误报率，需要约 1.44 MB 内存
//   - 误报率越低，需要的内存和哈希函数越多
func InitBloomFilter() error {
	// 创建布隆过滤器：
	// - capacity: 100万预估容量
	// - fpRate: 0.01% 误报率
	Filter = bloom.NewWithEstimates(1000000, 0.0001)
	log.Println("Bloom filter initialized with capacity=1000000, fpRate=0.01%")
	return nil
}

// WarmupBloomFilter 从数据库加载所有已存在的 short_code 到布隆过滤器
//
// 预热流程：
//   1. 遍历 short_links 表，读取所有 short_code
//   2. 逐个添加到布隆过滤器
//
// 注意：
//   - 这是启动时的一次性操作，用于避免启动后新添加的短码被误判
//   - 数据量大时可能较慢，但只执行一次
func WarmupBloomFilter() error {
	var codes []string

	// 读取所有已存在的 short_code
	// 使用 PLUCK 只查询 short_code 字段，减少内存占用
	if err := DB.Model(&model.URL{}).Pluck("short_code", &codes).Error; err != nil {
		return err
	}

	// 添加到布隆过滤器
	for _, code := range codes {
		Filter.Add([]byte(code))
	}

	log.Printf("Bloom filter warmed up with %d existing short codes", len(codes))
	return nil
}