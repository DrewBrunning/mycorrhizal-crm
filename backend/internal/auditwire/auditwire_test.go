package auditwire

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFor_NilAndBareDBHaveNoMarker(t *testing.T) {
	assert.Nil(t, For(nil))
	assert.Nil(t, For(&gorm.DB{}))
	assert.Nil(t, For(&gorm.DB{Config: &gorm.Config{}}))
}

func TestDefault_InitializeKeepsRootAndForFindsIt(t *testing.T) {
	root := &gorm.DB{Config: &gorm.Config{Plugins: map[string]gorm.Plugin{}}}
	d := &Default{Async: true}
	require.Equal(t, PluginName, d.Name())
	require.NoError(t, d.Initialize(root))
	root.Plugins[PluginName] = d

	got := For(root)
	require.Same(t, d, got)
	assert.Same(t, root, got.Root())
	assert.True(t, got.Async)
}
