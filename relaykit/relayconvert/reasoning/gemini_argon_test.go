package reasoning

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiDeepCompatibility(t *testing.T) {
	t.Run("gemini-4-argon with explicit effort", func(t *testing.T) {
		intent := Intent{
			Effort: EffortMedium,
			Mode:   ModeEnabled,
		}
		rendered, err := RenderGemini("gemini-4-argon", intent, nil, 0)
		require.NoError(t, err)
		require.NotNil(t, rendered.Config)
		assert.Equal(t, "medium", rendered.Config.ThinkingLevel)
	})

	t.Run("gemini-4-argon with disabled thinking gracefully falls back without error", func(t *testing.T) {
		intent := Intent{
			Mode:   ModeDisabled,
			Effort: EffortNone,
		}
		rendered, err := RenderGemini("gemini-4-argon", intent, nil, 0)
		require.NoError(t, err)
		require.NotNil(t, rendered.Config)
		assert.Equal(t, "low", rendered.Config.ThinkingLevel)
	})

	t.Run("unknown future gemini model auto-heals into thinking level", func(t *testing.T) {
		intent := Intent{
			Effort: EffortHigh,
			Mode:   ModeEnabled,
		}
		rendered, err := RenderGemini("gemini-9-ultra-future", intent, nil, 0)
		require.NoError(t, err)
		require.NotNil(t, rendered.Config)
		assert.Equal(t, "high", rendered.Config.ThinkingLevel)
	})

	t.Run("non-standard effort string gracefully maps", func(t *testing.T) {
		intent := Intent{
			Effort: Effort("moderate"),
			Mode:   ModeEnabled,
		}
		rendered, err := RenderGemini("gemini-4-argon", intent, nil, 0)
		require.NoError(t, err)
		require.NotNil(t, rendered.Config)
		assert.Equal(t, "medium", rendered.Config.ThinkingLevel)
	})

	t.Run("gemini-4 cross budget healing in validation", func(t *testing.T) {
		budget := 4096
		config := &dto.GeminiThinkingConfig{
			ThinkingBudget: &budget,
		}
		effort, err := ValidateGeminiThinkingConfig("gemini-4-argon", config)
		require.NoError(t, err)
		assert.NotEmpty(t, effort)
		assert.NotEmpty(t, config.ThinkingLevel)
		assert.Nil(t, config.ThinkingBudget)
	})
}

