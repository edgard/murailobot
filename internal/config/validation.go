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
		utils.WriteDebugLog(componentName, "admin access granted",
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
		utils.WriteDebugLog(componentName, "access denied - user blocked",
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
		utils.WriteDebugLog(componentName, "checking allowed users list",
			utils.KeyUserID, userID,
			utils.KeyAction, "authorization",
			utils.KeyResult, map[string]interface{}{
				"allowed":          allowed,
				"allowed_list_len": len(c.Telegram.AllowedUserIDs),
			})
		return allowed
	}

	utils.WriteDebugLog(componentName, "access granted - no restrictions",
		utils.KeyUserID, userID,
		utils.KeyAction, "authorization",
		utils.KeyResult, "authorized")
	return true
}

func (c *Config) ValidateChatMessage(userID int64, message string) error {
	if len(message) == 0 {
		return utils.NewError(componentName, utils.ErrValidation, "message is empty", utils.CategoryValidation, nil)
	}

	if len(message) > c.MaxMessageSize {
		return utils.Errorf(componentName, utils.ErrValidation, utils.CategoryValidation,
			"message exceeds maximum length of %d characters", c.MaxMessageSize)
	}

	if !c.IsUserAuthorized(userID) {
		return utils.NewError(componentName, utils.ErrValidation, "user not authorized", utils.CategoryValidation, nil)
	}

	return nil
}
