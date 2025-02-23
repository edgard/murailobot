// Package config provides configuration validation functionality.
// This file specifically handles user authorization and message validation logic.
package config

import (
	"github.com/edgard/murailobot/internal/utils"
)

// IsUserAuthorized determines if a user is authorized to interact with the bot.
// Authorization follows these rules in order:
//  1. Admin is always authorized and cannot be blocked
//  2. Blocked users are denied access regardless of other settings
//  3. If an allowed users list exists, only listed users are authorized
//  4. If no allowed users list exists, all non-blocked users are authorized
//
// The configuration validation ensures:
//   - Admin is never blocked (validator: nefield=Config.Telegram.AdminID)
//   - No user is both allowed and blocked (validator: excluded_with=BlockedUserIDs)
//   - Admin is in allowed users list if present (validator: required_with=Config.Security.AllowedUserIDs)
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

// ValidateChatMessage performs comprehensive validation of a chat message.
// It checks both the message content and user authorization:
//  1. Message must not be empty
//  2. Message length must not exceed configured maximum
//  3. User must be authorized according to security settings
//
// Returns nil if all validations pass, or an appropriate error describing
// which validation failed.
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
