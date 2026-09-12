package model

import (
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"
	"time"
)

// UserPricingRule is a structured override for user/group/model pricing.
// Nullable scope fields allow the same table to represent global, group and
// user-specific rules while keeping legacy JSON settings untouched.
type UserPricingRule struct {
	ID           uint    `gorm:"primaryKey" json:"id"`
	UserID       *int    `gorm:"index" json:"user_id,omitempty"`
	UserGroup    string  `gorm:"type:varchar(64);index" json:"user_group,omitempty"`
	BillingGroup string  `gorm:"type:varchar(64);index" json:"billing_group,omitempty"`
	ModelName    string  `gorm:"type:varchar(255);index" json:"model_name,omitempty"`
	BillingType  string  `gorm:"type:varchar(32);index" json:"billing_type,omitempty"`
	PricingMode  string  `gorm:"type:varchar(16)" json:"pricing_mode"`
	Ratio        float64 `json:"ratio"`
	InputPrice   float64 `json:"input_price"`
	OutputPrice  float64 `json:"output_price"`
	RequestPrice float64 `json:"request_price"`
	ImagePrice   float64 `json:"image_price"`
	VideoPrice   float64 `json:"video_price"`
	Priority     int     `gorm:"index" json:"priority"`
	Enabled      bool    `gorm:"index" json:"enabled"`
	ValidFrom    int64   `gorm:"index" json:"valid_from"`
	ValidTo      int64   `gorm:"index" json:"valid_to"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

const (
	PricingModeRatio = "ratio"
	PricingModeFixed = "fixed"
)

type UserGroupMembership struct {
	UserID    int    `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	Group     string `gorm:"primaryKey;type:varchar(64);autoIncrement:false" json:"group"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func GetUserGroupMemberships(userID int) ([]UserGroupMembership, error) {
	var memberships []UserGroupMembership
	err := DB.Where("user_id = ?", userID).Order("created_at asc").Find(&memberships).Error
	return memberships, err
}

func ReplaceUserGroupMemberships(tx *gorm.DB, userID int, groups []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&UserGroupMembership{}).Error; err != nil {
		return err
	}
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if err := tx.Create(&UserGroupMembership{UserID: userID, Group: group, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *UserPricingRule) Normalize() error {
	r.UserGroup = strings.TrimSpace(r.UserGroup)
	r.BillingGroup = strings.TrimSpace(r.BillingGroup)
	r.ModelName = strings.TrimSpace(r.ModelName)
	r.BillingType = strings.ToLower(strings.TrimSpace(r.BillingType))
	r.PricingMode = strings.ToLower(strings.TrimSpace(r.PricingMode))
	if r.PricingMode != PricingModeRatio && r.PricingMode != PricingModeFixed {
		return errors.New("pricing_mode must be ratio or fixed")
	}
	if r.ValidTo > 0 && r.ValidFrom > r.ValidTo {
		return errors.New("valid_to must not be before valid_from")
	}
	for _, value := range []float64{r.Ratio, r.InputPrice, r.OutputPrice, r.RequestPrice, r.ImagePrice, r.VideoPrice} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.New("pricing values must be finite and non-negative")
		}
	}
	if r.PricingMode == PricingModeRatio && r.Ratio <= 0 {
		return errors.New("ratio pricing requires a positive ratio")
	}
	if r.PricingMode == PricingModeFixed && r.InputPrice == 0 && r.OutputPrice == 0 && r.RequestPrice == 0 && r.ImagePrice == 0 && r.VideoPrice == 0 {
		return errors.New("fixed pricing requires at least one price")
	}
	return nil
}

func (r *UserPricingRule) IsActive(now int64) bool {
	if r == nil || !r.Enabled {
		return false
	}
	if r.ValidFrom > 0 && now < r.ValidFrom {
		return false
	}
	return r.ValidTo <= 0 || now <= r.ValidTo
}

func (r *UserPricingRule) BeforeCreate(_ *gorm.DB) error {
	if r.CreatedAt == 0 {
		r.CreatedAt = time.Now().Unix()
	}
	if r.UpdatedAt == 0 {
		r.UpdatedAt = r.CreatedAt
	}
	return nil
}
