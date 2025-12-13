package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"kiro/config"

	"kiro/types"
	"kiro/utils"
	"net/http"
	"sync"
	"time"
)

/**
 * TokenCache 存储用户的 Token 缓存信息
 */
type TokenCache struct {
	AccessToken  string
	RefreshToken string
	LastRefresh  time.Time
}

var (
	// tokenMap Token 缓存映射（key: token hash）
	tokenMap = make(map[string]*TokenCache)
	// tokenMutex Token 缓存互斥锁
	tokenMutex sync.RWMutex
)

/**
 * sha256Hash 计算输入文本的 SHA256 哈希值
 */
func sha256Hash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

/**
 * RefreshToken 刷新 token
 */
func RefreshToken(refreshToken string) (string, error) {
	refreshReq := types.RefreshRequest{
		RefreshToken: refreshToken,
	}

	reqBody, err := utils.FastMarshal(refreshReq)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", config.RefreshTokenURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := utils.SharedHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("刷新失败: 状态码 %d, 响应: %s", resp.StatusCode, string(body))
	}

	var refreshResp types.RefreshResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if err := utils.SafeUnmarshal(body, &refreshResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	return refreshResp.AccessToken, nil
}

/**
 * GetOrRefreshToken 获取或刷新 token
 */
func GetOrRefreshToken(token string) (string, error) {
	tokenHash := sha256Hash(token)

	// 检查缓存
	tokenMutex.RLock()
	cached, exists := tokenMap[tokenHash]
	tokenMutex.RUnlock()

	if exists {
		return cached.AccessToken, nil
	}

	// 刷新 token
	accessToken, err := RefreshToken(token)
	if err != nil {
		return "", err
	}

	// 缓存
	tokenMutex.Lock()
	tokenMap[tokenHash] = &TokenCache{
		AccessToken:  accessToken,
		RefreshToken: token,
		LastRefresh:  time.Now(),
	}
	tokenMutex.Unlock()

	return accessToken, nil
}

/**
 * RefreshAllTokens 全局刷新器，遍历并刷新所有缓存的 token
 */
func RefreshAllTokens() {
	tokenMutex.RLock()
	count := len(tokenMap)
	tokenMutex.RUnlock()

	if count == 0 {
		return
	}

	utils.Log("开始 token 刷新周期", utils.LogInt("total_tokens", count))
	refreshCount := 0

	tokenMutex.RLock()
	tokens := make(map[string]*TokenCache)
	for k, v := range tokenMap {
		tokens[k] = v
	}
	tokenMutex.RUnlock()

	for hash, cache := range tokens {
		newToken, err := RefreshToken(cache.RefreshToken)

		if err != nil {
			utils.Log("刷新 token 失败，从缓存中移除",
				utils.LogString("hash_prefix", hash[:8]),
				utils.LogErr(err))
			tokenMutex.Lock()
			delete(tokenMap, hash)
			tokenMutex.Unlock()
			continue
		}

		tokenMutex.Lock()
		if tokenMap[hash] != nil {
			tokenMap[hash].AccessToken = newToken
			tokenMap[hash].LastRefresh = time.Now()
		}
		tokenMutex.Unlock()

		refreshCount++
	}

	utils.Log("token 刷新完成",
		utils.LogInt("refreshed", refreshCount),
		utils.LogInt("total", count))
}

/**
 * StartTokenRefresher 启动定时 token 刷新器
 * 在后台 goroutine 中每 45 分钟自动刷新所有缓存的 token
 */
func StartTokenRefresher() {
	go func() {
		ticker := time.NewTicker(45 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			RefreshAllTokens()
		}
	}()

	utils.Log("Token 自动刷新器已启动", utils.LogString("interval", "45分钟"))
}
