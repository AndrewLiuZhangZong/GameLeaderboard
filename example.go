package leaderboard

import (
	"fmt"
	"time"
)

func TestLeaderboard() {
	fmt.Println("=== 基于Redis的分布式排行榜系统 ===\n")

	// 创建Redis配置
	config := Config{
		RedisAddr:      "localhost:6379",
		RedisPassword:  "",
		RedisDB:        0,
		LeaderboardKey: "global_leaderboard",
		IsDenseRanking: false,
	}

	fmt.Println("1. 标准排名示例")
	// 创建标准排行榜
	standardLB, err := NewLeaderboard(config)
	if err != nil {
		fmt.Printf("创建Redis排行榜失败: %v\n", err)
		fmt.Println("提示: 请确保Redis正在运行（redis-server）")
		return
	}
	defer standardLB.Close()

	// 测试数据
	now := time.Now()
	standardLB.UpdateScore("玩家A", 100, now)
	standardLB.UpdateScore("玩家B", 200, now.Add(time.Second))
	standardLB.UpdateScore("玩家C", 150, now.Add(2*time.Second))
	standardLB.UpdateScore("玩家D", 200, now.Add(3*time.Second)) // 相同分数，时间更晚

	time.Sleep(100 * time.Millisecond) // 等待Redis写入

	fmt.Println("前3名玩家（标准排名）：")
	top3 := standardLB.GetTopN(3)
	for _, player := range top3 {
		fmt.Printf("  第%d名: %s (分数: %d)\n", player.Rank, player.PlayerID, player.Score)
	}

	rankInfo := standardLB.GetPlayerRank("玩家B")
	if rankInfo != nil {
		fmt.Printf("\n玩家B的排名: 第%d名 (分数: %d)\n", rankInfo.Rank, rankInfo.Score)
	}

	fmt.Println("\n玩家B前后1名：")
	rangeInfo := standardLB.GetPlayerRange("玩家B", 1)
	for _, player := range rangeInfo {
		fmt.Printf("  第%d名: %s (分数: %d)\n", player.Rank, player.PlayerID, player.Score)
	}

	fmt.Println("2. 密集排名示例")
	// 创建密集排名排行榜
	config.LeaderboardKey = "dense_leaderboard"
	denseLB, err := NewDenseLeaderboard(config)
	if err != nil {
		fmt.Printf("创建密集排行榜失败: %v\n", err)
		return
	}
	defer denseLB.Close()

	// 添加测试数据（题目示例）
	denseLB.UpdateScore("玩家A", 100, now)
	denseLB.UpdateScore("玩家B", 100, now.Add(time.Second))
	denseLB.UpdateScore("玩家C", 95, now.Add(2*time.Second))
	denseLB.UpdateScore("玩家D", 95, now.Add(3*time.Second))
	denseLB.UpdateScore("玩家E", 90, now.Add(4*time.Second))
	denseLB.UpdateScore("玩家F", 89, now.Add(5*time.Second))

	time.Sleep(100 * time.Millisecond)

	fmt.Println("密集排名结果：")
	allPlayers := denseLB.GetTopN(6)
	for _, player := range allPlayers {
		fmt.Printf("  第%d名: %s (分数: %d)\n", player.Rank, player.PlayerID, player.Score)
	}

	fmt.Println("3. 分布式特性演示")
	fmt.Println("Redis排行榜的优势：")
	fmt.Println("- ✅ 多节点共享同一排序结果")
	fmt.Println("- ✅ 自动处理并发更新")
	fmt.Println("- ✅ 高性能 O(log N) 查询")
	fmt.Println("- ✅ 数据持久化")
	fmt.Println("- ✅ 支持集群部署")

	stats := standardLB.GetStatistics()
	fmt.Printf("\n统计信息: %+v\n", stats)

	fmt.Println("\n=== 使用说明 ===")
	fmt.Println("标准排名: 相同分数的玩家排名不同")
	fmt.Println("密集排名: 相同分数的玩家排名相同")
	fmt.Println("适合大型游戏和分布式部署")
}
