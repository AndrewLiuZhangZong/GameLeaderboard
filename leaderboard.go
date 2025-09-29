package leaderboard

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// Player 玩家信息
type Player struct {
	ID        string    `json:"id"`
	Score     int64     `json:"score"`
	Timestamp time.Time `json:"timestamp"`
}

// RankInfo 排名信息
type RankInfo struct {
	PlayerID  string    `json:"player_id"`
	Rank      int       `json:"rank"`
	Score     int64     `json:"score"`
	Timestamp time.Time `json:"timestamp"`
}

// LeaderboardService 排行榜服务接口
type LeaderboardService interface {
	// UpdateScore 更新玩家分数
	UpdateScore(playerID string, incrScore int64, timestamp time.Time)

	// GetPlayerRank 获取玩家当前排名
	GetPlayerRank(playerID string) *RankInfo

	// GetTopN 获取排行榜前N名
	GetTopN(n int) []RankInfo

	// GetPlayerRange 查询自己名次前后共N名玩家的分数和名次
	GetPlayerRange(playerID string, n int) []RankInfo
}

// RedisLeaderboard Redis分布式排行榜实现
type RedisLeaderboard struct {
	redis *redis.Client
	mutex sync.RWMutex
	key   string
}

// Config Redis排行榜配置
type Config struct {
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	LeaderboardKey string
	IsDenseRanking bool // 是否为密集排名
}

// NewLeaderboard 创建Redis分布式排行榜
func NewLeaderboard(config Config) (*RedisLeaderboard, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	key := config.LeaderboardKey
	if key == "" {
		key = "leaderboard"
	}

	return &RedisLeaderboard{
		redis: client,
		key:   key,
	}, nil
}

// UpdateScore 更新玩家分数
func (lb *RedisLeaderboard) UpdateScore(playerID string, incrScore int64, timestamp time.Time) {
	ctx := context.Background()

	// 获取当前分数
	currentScore, err := lb.redis.ZScore(ctx, lb.key, playerID).Result()
	if err != nil && err != redis.Nil {
		fmt.Printf("Warning: failed to get current score for %s: %v\n", playerID, err)
		currentScore = 0
	}

	// 计算新分数
	newScore := currentScore + float64(incrScore)

	// 使用负数保证高分数在高位，加上时间戳保证同分时的时间排序
	// 分数相同时，先得到该分数的玩家排在前面
	scoreWithTimestamp := -(newScore*1e9 + float64(timestamp.UnixNano())/1e9)

	if err := lb.redis.ZAdd(ctx, lb.key, &redis.Z{
		Score:  scoreWithTimestamp,
		Member: playerID,
	}).Err(); err != nil {
		fmt.Printf("Warning: failed to update score for %s: %v\n", playerID, err)
	}
}

// GetPlayerRank 获取玩家排名（标准排名）
func (lb *RedisLeaderboard) GetPlayerRank(playerID string) *RankInfo {
	ctx := context.Background()

	// 获取分数
	score, err := lb.redis.ZScore(ctx, lb.key, playerID).Result()
	if err != nil {
		return nil
	}

	// 获取排名（Redis ZRANK从0开始，所以要+1）
	rank := int64(0)
	if rankCmd := lb.redis.ZRank(ctx, lb.key, playerID); rankCmd.Err() == nil {
		rank = rankCmd.Val() + 1
	}

	// 恢复原始分数
	originalScore := int64(-score / 1e9)

	return &RankInfo{
		PlayerID:  playerID,
		Rank:      int(rank),
		Score:     originalScore,
		Timestamp: time.Now(),
	}
}

// GetTopN 获取前N名（标准排名）
func (lb *RedisLeaderboard) GetTopN(n int) []RankInfo {
	ctx := context.Background()

	// 从Redis获取前N名
	members, err := lb.redis.ZRangeWithScores(ctx, lb.key, 0, int64(n-1)).Result()
	if err != nil {
		return nil
	}

	result := make([]RankInfo, 0, len(members))
	for i, member := range members {
		playerID := member.Member.(string)
		originalScore := int64(-member.Score / 1e9)

		result = append(result, RankInfo{
			PlayerID:  playerID,
			Rank:      i + 1,
			Score:     originalScore,
			Timestamp: time.Now(),
		})
	}

	return result
}

// GetPlayerRange 查询玩家前后N名（标准排名）
func (lb *RedisLeaderboard) GetPlayerRange(playerID string, n int) []RankInfo {
	ctx := context.Background()

	// 获取玩家排名
	playerRank, err := lb.redis.ZRank(ctx, lb.key, playerID).Result()
	if err != nil {
		return nil
	}

	// 计算范围
	start := playerRank - int64(n)
	if start < 0 {
		start = 0
	}
	end := playerRank + int64(n)

	// 获取范围内的所有玩家
	members, err := lb.redis.ZRangeWithScores(ctx, lb.key, start, end).Result()
	if err != nil {
		return nil
	}

	result := make([]RankInfo, 0, len(members))
	for i, member := range members {
		playerID := member.Member.(string)
		originalScore := int64(-member.Score / 1e9)

		result = append(result, RankInfo{
			PlayerID:  playerID,
			Rank:      int(start) + i + 1,
			Score:     originalScore,
			Timestamp: time.Now(),
		})
	}

	return result
}

// GetStatistics 获取排行榜统计
func (lb *RedisLeaderboard) GetStatistics() map[string]interface{} {
	ctx := context.Background()
	playerCount := lb.redis.ZCard(ctx, lb.key).Val()

	return map[string]interface{}{
		"total_players":   playerCount,
		"leaderboard_key": lb.key,
		"redis_addr":      lb.redis.Options().Addr,
	}
}

// Close 关闭Redis连接
func (lb *RedisLeaderboard) Close() error {
	return lb.redis.Close()
}

// DenseRedisLeaderboard Redis密集排名排行榜实现
type DenseRedisLeaderboard struct {
	redis *redis.Client
	mutex sync.RWMutex
	key   string
}

// NewDenseLeaderboard 创建Redis密集排名排行榜
func NewDenseLeaderboard(config Config) (*DenseRedisLeaderboard, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	key := config.LeaderboardKey
	if key == "" {
		key = "leaderboard"
	}

	return &DenseRedisLeaderboard{
		redis: client,
		key:   key,
	}, nil
}

// UpdateScore 更新玩家分数
func (lb *DenseRedisLeaderboard) UpdateScore(playerID string, incrScore int64, timestamp time.Time) {
	ctx := context.Background()

	currentScore, err := lb.redis.ZScore(ctx, lb.key, playerID).Result()
	if err != nil && err != redis.Nil {
		fmt.Printf("Warning: failed to get current score for %s: %v\n", playerID, err)
		currentScore = 0
	}

	newScore := currentScore + float64(incrScore)
	scoreWithTimestamp := -(newScore*1e9 + float64(timestamp.UnixNano())/1e9)

	if err := lb.redis.ZAdd(ctx, lb.key, &redis.Z{
		Score:  scoreWithTimestamp,
		Member: playerID,
	}).Err(); err != nil {
		fmt.Printf("Warning: failed to update score for %s: %v\n", playerID, err)
	}
}

// GetPlayerRank 获取玩家密集排名
func (lb *DenseRedisLeaderboard) GetPlayerRank(playerID string) *RankInfo {
	ctx := context.Background()

	score, err := lb.redis.ZScore(ctx, lb.key, playerID).Result()
	if err != nil {
		return nil
	}

	// 获取原始分数（整数部分）
	originalScore := int64(-score / 1e9)

	// 计算密集排名：统计有多少个玩家的分数大于当前玩家
	countCmd := lb.redis.ZCount(ctx, lb.key, "-inf", fmt.Sprintf("%.f", score))
	count, err := countCmd.Result()
	if err != nil {
		return nil
	}

	// 密集排名：相同分数的玩家获得相同排名
	denseRank := count

	return &RankInfo{
		PlayerID:  playerID,
		Rank:      int(denseRank),
		Score:     originalScore,
		Timestamp: time.Now(),
	}
}

// GetTopN 获取前N名（密集排名）
func (lb *DenseRedisLeaderboard) GetTopN(n int) []RankInfo {
	ctx := context.Background()

	members, err := lb.redis.ZRangeWithScores(ctx, lb.key, 0, int64(n-1)).Result()
	if err != nil {
		return nil
	}

	result := make([]RankInfo, 0, len(members))
	for _, member := range members {
		playerID := member.Member.(string)
		originalScore := int64(-member.Score / 1e9)

		// 计算密集排名
		countCmd := lb.redis.ZCount(ctx, lb.key, "-inf", fmt.Sprintf("%.f", member.Score))
		count, err := countCmd.Result()
		if err != nil {
			continue
		}

		result = append(result, RankInfo{
			PlayerID:  playerID,
			Rank:      int(count),
			Score:     originalScore,
			Timestamp: time.Now(),
		})
	}

	return result
}

// GetPlayerRange 查询玩家前后N名（密集排名）
func (lb *DenseRedisLeaderboard) GetPlayerRange(playerID string, n int) []RankInfo {
	ctx := context.Background()

	playerRank, err := lb.redis.ZRank(ctx, lb.key, playerID).Result()
	if err != nil {
		return nil
	}

	start := playerRank - int64(n)
	if start < 0 {
		start = 0
	}
	end := playerRank + int64(n)

	members, err := lb.redis.ZRangeWithScores(ctx, lb.key, start, end).Result()
	if err != nil {
		return nil
	}

	result := make([]RankInfo, 0, len(members))
	for _, member := range members {
		playerID := playerID
		originalScore := int64(-member.Score / 1e9)

		countCmd := lb.redis.ZCount(ctx, lb.key, "-inf", fmt.Sprintf("%.f", member.Score))
		count, err := countCmd.Result()
		if err != nil {
			continue
		}

		result = append(result, RankInfo{
			PlayerID:  playerID,
			Rank:      int(count),
			Score:     originalScore,
			Timestamp: time.Now(),
		})
	}

	return result
}

// Close 关闭Redis连接
func (lb *DenseRedisLeaderboard) Close() error {
	return lb.redis.Close()
}
