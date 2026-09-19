package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstrainPriorityToGroupRange(t *testing.T) {
	firstGroup := int64(9)
	secondGroup := int64(19)

	assert.Equal(t, int64(1), constrainPriorityToGroupRange(&firstGroup, -1))
	assert.Equal(t, int64(9), constrainPriorityToGroupRange(&firstGroup, 99))
	assert.Equal(t, int64(14), constrainPriorityToGroupRange(&secondGroup, 14))
	assert.Equal(t, int64(19), constrainPriorityToGroupRange(&secondGroup, 99))
	assert.Equal(t, int64(1), constrainPriorityToGroupRange(nil, 99))
}

func TestRemapPriorityByBase(t *testing.T) {
	// Target group base: GPTPro-企业级 (reference priority 49, base 40)
	targetBase := int64(49)

	// Case 1: Switching from gpt特惠组 (81, offset 1) -> maps to 41 (40 + 1)
	assert.Equal(t, int64(41), remapPriorityByBase(targetBase, 81))

	// Case 2: Switching from mid-tier (85, offset 5) -> maps to 45 (40 + 5)
	assert.Equal(t, int64(45), remapPriorityByBase(targetBase, 85))

	// Case 3: Switching from top-tier (89, offset 9) -> maps to 49 (40 + 9)
	assert.Equal(t, int64(49), remapPriorityByBase(targetBase, 89))

	// Case 4: Already in target range (45, offset 5) -> maps to 45
	assert.Equal(t, int64(45), remapPriorityByBase(targetBase, 45))

	// Case 5: Zero or negative target base returns current priority untouched
	assert.Equal(t, int64(81), remapPriorityByBase(0, 81))
	assert.Equal(t, int64(81), remapPriorityByBase(-10, 81))
}

func TestIsPriorityInGroupRange(t *testing.T) {
	enterpriseGroup := int64(49) // Range [41, 49]

	assert.True(t, isPriorityInGroupRange(&enterpriseGroup, 41))
	assert.True(t, isPriorityInGroupRange(&enterpriseGroup, 45))
	assert.True(t, isPriorityInGroupRange(&enterpriseGroup, 49))

	assert.False(t, isPriorityInGroupRange(&enterpriseGroup, 40))
	assert.False(t, isPriorityInGroupRange(&enterpriseGroup, 50))
	assert.False(t, isPriorityInGroupRange(&enterpriseGroup, 81))

	// Nil or zero groupPriority allows any priority
	assert.True(t, isPriorityInGroupRange(nil, 81))
	zeroGroup := int64(0)
	assert.True(t, isPriorityInGroupRange(&zeroGroup, 81))
}
