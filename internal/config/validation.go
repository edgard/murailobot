package config

import (
	"github.com/edgard/murailobot/internal/utils"
)

// IsUserAuthorized checks if a user is authorized based on security configuration.
// The configuration validation ensures that:
// 1. Admin is never blocked (via validator tag nefield=Config.Telegram.AdminID)
// 2. No user is both allowed and blocked (via validator tag excluded_with=BlockedUserIDs)
// 3. Admin is in allowed users list if the list is not empty (via validator tag required_with=Config.Security.AllowedUserIDs)
func (c *Config) IsUserAuthorized(userID int64) bool {
	// Fast path: admin is always authorized
	if userID == c.Telegram.AdminID {
		utils.WriteDebugLog(componentName, "admin access granted",
			utils.KeyUserID, userID,
			utils.KeyAction, "authorization",
			utils.KeyResult, "authorized")
		return true
	}

	// Fast path: check blocked users first
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

	// If allowed users list exists, user must be in it
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

	// If no allowed users list, everyone except blocked users is allowed
	utils.WriteDebugLog(componentName, "access granted - no restrictions",
		utils.KeyUserID, userID,
		utils.KeyAction, "authorization",
		utils.KeyResult, "authorized")
	return true
}

// ValidateChatMessage validates a chat message and its metadata.
// This combines message length and user authorization checks.
func (c *Config) ValidateChatMessage(userID int64, message string) error {
	// Check message length
	if len(message) == 0 {
		return utils.NewError(componentName, utils.ErrValidation, "message is empty", utils.CategoryValidation, nil)
	}

	if len(message) > c.MaxMessageSize {
		return utils.Errorf(componentName, utils.ErrValidation, utils.CategoryValidation,
			"message exceeds maximum length of %d characters", c.MaxMessageSize)
	}

	// Check user authorization
	if !c.IsUserAuthorized(userID) {
		return utils.NewError(componentName, utils.ErrValidation, "user not authorized", utils.CategoryValidation, nil)
	}

	return nil
}
