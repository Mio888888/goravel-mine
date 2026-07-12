package services

import (
	"encoding/json"
	"fmt"
	"time"

	"goravel/app/facades"
)

const currentUserInfoCacheTTL = 2 * time.Minute

func currentUserInfoCacheKey(connection string, userID uint64) string {
	return fmt.Sprintf(
		"current_user_info:%d:%s:%d:%d",
		currentUserInfoCacheVersion(),
		connection,
		currentUserInfoVersionForUser(connection, userID),
		userID,
	)
}

func currentUserInfoCacheVersion() int64 {
	return facades.Cache().GetInt64("current_user_info_version", 0)
}

func currentUserInfoVersionKey(connection string, userID uint64) string {
	return fmt.Sprintf("current_user_info_user_version:%s:%d", connection, userID)
}

func currentUserInfoVersionForUser(connection string, userID uint64) int64 {
	return facades.Cache().GetInt64(currentUserInfoVersionKey(connection, userID), 0)
}

func cachedCurrentUserInfo(connection string, userID uint64) (UserInfo, bool) {
	raw := facades.Cache().GetString(currentUserInfoCacheKey(connection, userID))
	if raw == "" {
		return UserInfo{}, false
	}

	var info UserInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		InvalidateCurrentUserInfoForConnection(connection, userID)
		return UserInfo{}, false
	}

	return info, true
}

func cacheCurrentUserInfo(connection string, info UserInfo) {
	raw, err := json.Marshal(info)
	if err != nil {
		return
	}
	_ = facades.Cache().Put(currentUserInfoCacheKey(connection, info.ID), string(raw), currentUserInfoCacheTTL)
}

func InvalidateCurrentUserInfo(userIDs ...uint64) {
	InvalidateAllCurrentUserInfo()
}

func InvalidateCurrentUserInfoForConnection(connection string, userIDs ...uint64) {
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		key := currentUserInfoVersionKey(connection, userID)
		if _, err := facades.Cache().Increment(key); err != nil {
			_ = facades.Cache().Put(key, int64(2), currentUserInfoCacheTTL)
		}
	}
}

func InvalidateAllCurrentUserInfo() {
	if _, err := facades.Cache().Increment("current_user_info_version"); err != nil {
		_ = facades.Cache().Put("current_user_info_version", int64(2), currentUserInfoCacheTTL)
	}
}
