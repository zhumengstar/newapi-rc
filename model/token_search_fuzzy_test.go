package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeLikePatternWithAutoWildcard(t *testing.T) {
	// Auto wildcard enabled (normal user)
	pat, err := sanitizeLikePatternWithAutoWildcard("hello", true)
	require.NoError(t, err)
	assert.Equal(t, "%hello%", pat)

	// Explicit wildcards preserved
	pat, err = sanitizeLikePatternWithAutoWildcard("hello%", true)
	require.NoError(t, err)
	assert.Equal(t, "hello%", pat)

	pat, err = sanitizeLikePatternWithAutoWildcard("%hello", true)
	require.NoError(t, err)
	assert.Equal(t, "%hello", pat)

	// Escapes ! and _
	pat, err = sanitizeLikePatternWithAutoWildcard("hel_lo!world", true)
	require.NoError(t, err)
	assert.Equal(t, "%hel!_lo!!world%", pat)

	// Auto wildcard disabled (over limit user fallback)
	pat, err = sanitizeLikePatternWithAutoWildcard("hello", false)
	require.NoError(t, err)
	assert.Equal(t, "hello", pat)

	// Multiple %% rejected
	_, err = sanitizeLikePatternWithAutoWildcard("%%test", true)
	assert.Error(t, err)

	// Too many % rejected
	_, err = sanitizeLikePatternWithAutoWildcard("%a%b%c%", true)
	assert.Error(t, err)
}

func TestLikeOpBehavior(t *testing.T) {
	// Default (SQLite)
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	assert.Equal(t, "LIKE", mainLikeOp())

	// PostgreSQL
	common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
	assert.Equal(t, "ILIKE", mainLikeOp())

	// Reset to SQLite
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	assert.Equal(t, "LIKE", mainLikeOp())
}
