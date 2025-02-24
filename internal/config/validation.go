package config

import (
	"github.com/edgard/murailobot/internal/utils"
)

// IsUserAuthorized checks authorization in order:
// 1. Admin is always authorized and cannot be blocked
// 2. Blocked users are denied access
// 3. If allowed list exists, only listed users are authorized
// 4. Otherwise, all non-blocked users are allowed
func (c *Config) IsUserAuthorized(userID int64) bool {
	if userID == c.Telegram.AdminID {
		utils.DebugLog(componentName, "admin access granted",
			utils.KeyUserID, userID,
			utils.KeyAction, "authorization",
			utils.KeyResult, "authorized")
		return true
	}

	blockedMap := make(map[int64]bool, len(c.Telegram.BlockedUserIDs))
	for _, id := range c.Telegram.BlockedUserIDs {
		blockedMap[id] = true
	}
	if blockedMap[userID] {
		utils.DebugLog(componentName, "access denied - user blocked",
			utils.KeyUserID, userID,
			utils.KeyAction, "authorization",
			utils.KeyResult, "blocked")
		return false
	}

	if len(c.Telegram.AllowedUserIDs) > 0 {
		allowedMap := make(map[int64]bool, len(c.Telegram.AllowedUserIDs))
		for _, id := range c.Telegram.AllowedUserIDs {
			allowedMap[id] = true
		}
		allowed := allowedMap[userID]
		utils.DebugLog(componentName, "checking allowed users list",
			utils.KeyUserID, userID,
			utils.KeyAction, "authorization",
			utils.KeyResult, map[string]interface{}{
				"allowed":          allowed,
				"allowed_list_len": len(c.Telegram.AllowedUserIDs),
			})
		return allowed
	}

	utils.DebugLog(componentName, "access granted - no restrictions",
		utils.KeyUserID, userID,
		utils.KeyAction, "authorization",
		utils.KeyResult, "authorized")
	return true
}

func (c *Config) ValidateChatMessage(userID int64, message string) error {
	if len(message) == 0 {
		return utils.NewError(componentName, utils.ErrValidation, "message is empty", utils.CategoryValidation, nil)
	}

	if !c.IsUserAuthorized(userID) {
		return utils.NewError(componentName, utils.ErrValidation, "user not authorized", utils.CategoryValidation, nil)
	}

	return nil
}
