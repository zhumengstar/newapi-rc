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
