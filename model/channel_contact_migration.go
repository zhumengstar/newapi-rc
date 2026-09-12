package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	channelNameContactMigrationOptionKey   = "migration.channel_name_contacts.v1"
	channelNameContactMigrationOptionValue = "completed"
)

// MigrateChannelNameContacts moves the suffix after the final hyphen into the
// dedicated contact field. Existing contacts are never overwritten.
func MigrateChannelNameContacts(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	var channels []Channel
	if err := db.Where("contact = '' OR contact IS NULL").Find(&channels).Error; err != nil {
		return err
	}
	for _, channel := range channels {
		index := strings.LastIndex(channel.Name, "-")
		if index <= 0 || index == len(channel.Name)-1 {
			continue
		}
		name := strings.TrimSpace(channel.Name[:index])
		contact := strings.TrimSpace(channel.Name[index+1:])
		if name == "" || contact == "" {
			continue
		}
		if err := db.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"name": name, "contact": contact}).Error; err != nil {
			return err
		}
	}
	return nil
}

// MigrateChannelNameContactsIfRequested keeps normal startup free of a
// destructive name rewrite. The migration is intentionally opt-in and is
// recorded once so a retained environment variable cannot affect new rows.
func MigrateChannelNameContactsIfRequested(db *gorm.DB) error {
	if !common.GetEnvOrDefaultBool("MIGRATE_CHANNEL_NAME_CONTACTS", false) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: channelNameContactMigrationOptionKey}).First(&marker).Error
		if err == nil && marker.Value == channelNameContactMigrationOptionValue {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := MigrateChannelNameContacts(tx); err != nil {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&Option{Key: channelNameContactMigrationOptionKey, Value: channelNameContactMigrationOptionValue}).Error
		}
		marker.Value = channelNameContactMigrationOptionValue
		return tx.Save(&marker).Error
	})
}
