package model

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// MaxChannelRatio bounds the channel-level multiplier accepted by the API.
// Keeping the limit finite prevents malformed values from reaching pricing or
// routing calculations.
const MaxChannelRatio = 1_000_000

const (
	channelNameRatioMigrationOptionKey   = "migration.channel_name_ratios.v3"
	channelNameRatioMigrationOptionValue = "completed"
)

var (
	// The dotted form is unambiguous (for example, "0.03-Provider").
	channelNameRatioPattern = regexp.MustCompile(`^\s*((?:0|[1-9][0-9]*)\.[0-9]+)\s*-\s*(\S(?:.*\S)?)\s*$`)
	// Several existing channel names use a compact decimal token with a
	// leading zero ("007-Provider" means 0.07). Keep the token bounded and
	// allow an optional textual suffix such as "012pro-Provider".
	compactChannelNameRatioPattern = regexp.MustCompile(`^\s*(0[0-9]{1,5})([A-Za-z]*)\s*-\s*(\S(?:.*\S)?)\s*$`)
	// Ordinary integer prefixes are only considered after they are matched
	// against the channel group's configured ratio (see the migration below).
	integerChannelNameRatioPattern = regexp.MustCompile(`^\s*([1-9][0-9]*)\s*-\s*(\S(?:.*\S)?)\s*$`)
)

// ParseChannelNameRatio extracts a channel multiplier prefix historically used
// in channel names and returns the cleaned name. Ordinary integer prefixes are
// deliberately rejected because they are commonly channel or account IDs;
// compact decimals are accepted only when they have a leading zero.
func ParseChannelNameRatio(name string) (ratio float64, cleanedName string, ok bool) {
	matches := channelNameRatioPattern.FindStringSubmatch(name)
	if len(matches) == 3 {
		ratio, err := strconv.ParseFloat(matches[1], 64)
		if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > MaxChannelRatio {
			return 0, "", false
		}
		cleanedName, _ = stripLegacyRatioNameMarker(matches[2])
		cleanedName = strings.TrimSpace(cleanedName)
		if cleanedName == "" {
			return 0, "", false
		}
		return ratio, cleanedName, true
	}

	matches = compactChannelNameRatioPattern.FindStringSubmatch(name)
	if len(matches) != 4 {
		return 0, "", false
	}
	// The first zero is the integer part, and the remaining digits are the
	// fractional part: 007 -> 0.07, 0065 -> 0.065, 01 -> 0.1.
	fractionalDigits := matches[1][1:]
	if strings.Trim(fractionalDigits, "0") == "" {
		return 0, "", false
	}
	ratio, err := strconv.ParseFloat("0."+fractionalDigits, 64)
	if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 || ratio > 1 {
		return 0, "", false
	}
	cleanedName, _ = stripLegacyRatioNameMarker(matches[3])
	if matches[2] != "" {
		// Preserve the separator when a textual marker follows the compact
		// numeric token, e.g. 012pro-Provider -> pro-Provider.
		cleanedName = matches[2] + "-" + cleanedName
	}
	cleanedName = strings.TrimSpace(cleanedName)
	if cleanedName == "" {
		return 0, "", false
	}
	return ratio, cleanedName, true
}

// stripLegacyRatioNameMarker removes the literal label some historical names
// placed between their multiplier prefix and an upstream URL, for example
// "007-倍率https://example.com". It only matches a URL-adjacent marker, so a
// normal channel name containing the Chinese word for ratio is left untouched.
func stripLegacyRatioNameMarker(name string) (string, bool) {
	for _, marker := range []string{"倍率https://", "倍率http://"} {
		index := strings.Index(name, marker)
		if index < 0 || (index > 0 && name[index-1] != '-') {
			continue
		}
		return name[:index] + strings.TrimPrefix(name[index:], "倍率"), true
	}
	return name, false
}

// loadChannelRatioPrefixAllowlist returns explicitly approved legacy prefixes.
// Name prefixes are ambiguous by nature, so migration only accepts forms that
// an operator has reviewed for the source data.
func loadChannelRatioPrefixAllowlist(environment string) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, token := range strings.Split(common.GetEnvOrDefaultString(environment, ""), ",") {
		token = strings.TrimSpace(token)
		if token != "" {
			allowed[token] = struct{}{}
		}
	}
	return allowed
}

// loadCompactChannelRatioAllowlist returns the reviewed compact decimal
// prefixes (for example, "007" for a historical 0.07 multiplier). The
// compact spelling could also be an external account ID, so it is never
// migrated automatically.
func loadCompactChannelRatioAllowlist() map[string]struct{} {
	return loadChannelRatioPrefixAllowlist("MIGRATE_CHANNEL_NAME_RATIO_COMPACT_PREFIXES")
}

// loadIntegerChannelRatioAllowlist returns the explicitly approved integer
// prefixes for the ambiguous legacy form. Integer prefixes are never inferred
// from a group ratio alone because a future channel ID can coincidentally
// match that value.
func loadIntegerChannelRatioAllowlist() map[string]struct{} {
	return loadChannelRatioPrefixAllowlist("MIGRATE_CHANNEL_NAME_RATIO_INTEGER_PREFIXES")
}

// parseChannelNameRatioForMigration only accepts the unambiguous dotted form
// by default. Compact forms require a reviewed exact prefix. This intentionally
// differs from ParseChannelNameRatio, which remains available for inspection
// and dry-run tooling.
func parseChannelNameRatioForMigration(name string, compactAllowedPrefixes map[string]struct{}) (ratio float64, cleanedName string, ok bool) {
	if channelNameRatioPattern.MatchString(name) {
		return ParseChannelNameRatio(name)
	}

	matches := compactChannelNameRatioPattern.FindStringSubmatch(name)
	if len(matches) != 4 {
		return 0, "", false
	}
	if _, allowed := compactAllowedPrefixes[matches[1]]; !allowed {
		return 0, "", false
	}
	return ParseChannelNameRatio(name)
}

// parseIntegerChannelRatioWhenGroupMatches handles the one legacy form that
// cannot be distinguished from an ID by its spelling alone. It is accepted
// only when an operator explicitly allows the prefix and the configured group
// multiplier is the same value.
func parseIntegerChannelRatioWhenGroupMatches(name string, group string, groupRatios map[string]float64, allowedPrefixes map[string]struct{}) (ratio float64, cleanedName string, ok bool) {
	groupRatio, exists := groupRatios[group]
	if !exists || math.IsNaN(groupRatio) || math.IsInf(groupRatio, 0) || groupRatio <= 0 || groupRatio > MaxChannelRatio {
		return 0, "", false
	}
	matches := integerChannelNameRatioPattern.FindStringSubmatch(name)
	if len(matches) != 3 {
		return 0, "", false
	}
	if _, allowed := allowedPrefixes[matches[1]]; !allowed {
		return 0, "", false
	}
	ratio, err := strconv.ParseFloat(matches[1], 64)
	if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 || ratio > MaxChannelRatio {
		return 0, "", false
	}
	tolerance := 1e-12 * math.Max(1, math.Abs(groupRatio))
	if math.Abs(ratio-groupRatio) > tolerance {
		return 0, "", false
	}
	cleanedName = strings.TrimSpace(matches[2])
	if cleanedName == "" {
		return 0, "", false
	}
	return ratio, cleanedName, true
}

func loadChannelGroupRatios(tx *gorm.DB) map[string]float64 {
	var option Option
	if err := tx.Where(&Option{Key: "GroupRatio"}).First(&option).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("load group ratios for channel name migration: %v", err))
		}
		return nil
	}
	var groupRatios map[string]float64
	if err := common.Unmarshal([]byte(option.Value), &groupRatios); err != nil {
		common.SysError(fmt.Sprintf("parse group ratios for channel name migration: %v", err))
		return nil
	}
	return groupRatios
}

// ChannelRatioMigrationResult summarizes a one-time, idempotent name cleanup.
type ChannelRatioMigrationResult struct {
	Scanned  int
	Migrated int
	Cleaned  int
}

func migrateChannelNameRatiosTx(tx *gorm.DB) (result ChannelRatioMigrationResult, err error) {
	groupRatios := loadChannelGroupRatios(tx)
	compactRatioAllowlist := loadCompactChannelRatioAllowlist()
	integerRatioAllowlist := loadIntegerChannelRatioAllowlist()
	var channels []Channel
	if err := tx.Model(&Channel{}).
		Select("id", "name", "group", "channel_ratio").
		Where("channel_ratio IS NULL").
		Find(&channels).Error; err != nil {
		return result, err
	}
	result.Scanned = len(channels)

	for _, channel := range channels {
		ratio, cleanedName, ok := parseChannelNameRatioForMigration(channel.Name, compactRatioAllowlist)
		if !ok {
			ratio, cleanedName, ok = parseIntegerChannelRatioWhenGroupMatches(channel.Name, channel.Group, groupRatios, integerRatioAllowlist)
		}
		if !ok {
			continue
		}
		update := tx.Model(&Channel{}).
			Where("id = ? AND name = ? AND channel_ratio IS NULL", channel.Id, channel.Name).
			Updates(map[string]any{
				"name":          cleanedName,
				"channel_ratio": ratio,
			})
		if update.Error != nil {
			return result, update.Error
		}
		if update.RowsAffected == 1 {
			result.Migrated++
		}
	}

	// The first migration version had already copied several ratios before the
	// URL-adjacent label was identified. Clean that narrow legacy spelling for
	// channels which already have an explicit channel_ratio as well.
	var namedChannels []Channel
	if err := tx.Model(&Channel{}).
		Select("id", "name", "channel_ratio").
		Where("channel_ratio IS NOT NULL").
		Find(&namedChannels).Error; err != nil {
		return result, err
	}
	for _, channel := range namedChannels {
		cleanedName, ok := stripLegacyRatioNameMarker(channel.Name)
		if !ok || cleanedName == channel.Name {
			continue
		}
		update := tx.Model(&Channel{}).
			Where("id = ? AND name = ? AND channel_ratio IS NOT NULL", channel.Id, channel.Name).
			Updates(map[string]any{"name": cleanedName})
		if update.Error != nil {
			return result, update.Error
		}
		if update.RowsAffected == 1 {
			result.Cleaned++
		}
	}
	return result, nil
}

// MigrateChannelNameRatios copies reviewed multiplier prefixes into
// channel_ratio and removes them from the name. Existing ratios are never
// overwritten. Dotted decimals migrate directly; compact prefixes require
// MIGRATE_CHANNEL_NAME_RATIO_COMPACT_PREFIXES, and ordinary integer prefixes
// additionally require MIGRATE_CHANNEL_NAME_RATIO_INTEGER_PREFIXES plus an
// exact group-ratio match. The compare-and-set predicate makes manually
// rerunning the operation safe after a partial batch.
func MigrateChannelNameRatios(db *gorm.DB) (result ChannelRatioMigrationResult, err error) {
	if db == nil {
		return result, fmt.Errorf("database is not initialized")
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		result, err = migrateChannelNameRatiosTx(tx)
		return err
	})
	return result, err
}

// migrateChannelNameRatiosOnce persists a completion marker in the same
// transaction as the data update. It prevents a retained opt-in environment
// variable from mutating channels created after the intended one-time import.
func migrateChannelNameRatiosOnce(db *gorm.DB) (result ChannelRatioMigrationResult, skipped bool, err error) {
	if db == nil {
		return result, skipped, fmt.Errorf("database is not initialized")
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var marker Option
		markerErr := tx.Where(&Option{Key: channelNameRatioMigrationOptionKey}).First(&marker).Error
		if markerErr == nil && marker.Value == channelNameRatioMigrationOptionValue {
			skipped = true
			return nil
		}
		if markerErr != nil && !errors.Is(markerErr, gorm.ErrRecordNotFound) {
			return markerErr
		}

		migrationResult, migrationErr := migrateChannelNameRatiosTx(tx)
		if migrationErr != nil {
			return migrationErr
		}
		result = migrationResult

		if errors.Is(markerErr, gorm.ErrRecordNotFound) {
			return tx.Create(&Option{
				Key:   channelNameRatioMigrationOptionKey,
				Value: channelNameRatioMigrationOptionValue,
			}).Error
		}
		marker.Value = channelNameRatioMigrationOptionValue
		return tx.Save(&marker).Error
	})
	return result, skipped, err
}

// MigrateChannelNameRatiosIfRequested runs the one-time name cleanup only when
// an operator explicitly opts in. This keeps normal application startup free
// of surprising channel-name mutations while still making the migration
// reusable from the regular database initialization path.
func MigrateChannelNameRatiosIfRequested(db *gorm.DB) error {
	if !common.GetEnvOrDefaultBool("MIGRATE_CHANNEL_NAME_RATIOS", false) {
		return nil
	}

	result, skipped, err := migrateChannelNameRatiosOnce(db)
	if err != nil {
		return fmt.Errorf("migrate channel name ratios: %w", err)
	}
	if skipped {
		return nil
	}
	common.SysLog(fmt.Sprintf(
		"channel name ratio migration completed: scanned=%d migrated=%d cleaned=%d",
		result.Scanned,
		result.Migrated,
		result.Cleaned,
	))
	return nil
}
