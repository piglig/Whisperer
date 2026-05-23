package investigator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByIDAndNewFromTemplate(t *testing.T) {
	tmpl, ok := ByID("journalist")
	require.True(t, ok)

	inv := NewFromTemplate("save-1", tmpl)

	assert.NotEmpty(t, inv.ID)
	assert.Equal(t, "save-1", inv.SaveID)
	assert.Equal(t, "Lyra Marsh", inv.Name)
	assert.Equal(t, "记者", inv.Occupation)
	assert.True(t, inv.Active)
	assert.Contains(t, inv.SkillsJSON, "Library Use")
}

func TestNewQuickOverridesIdentityOnly(t *testing.T) {
	inv := NewQuick("save-1", "新调查员", "医生")

	assert.Equal(t, "新调查员", inv.Name)
	assert.Equal(t, "医生", inv.Occupation)
	assert.Contains(t, inv.InventoryJSON, "便携相机")
	assert.Equal(t, 60, inv.SAN)
}

func TestUnknownTemplateErrorListsKnownIDs(t *testing.T) {
	err := ErrUnknownTemplate("bad")

	assert.Contains(t, err.Error(), "journalist")
	assert.Contains(t, err.Error(), "private_eye")
	assert.Contains(t, err.Error(), "doctor")
}
