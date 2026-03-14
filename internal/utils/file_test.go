package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePath_HomeTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result, err := ResolvePath("~/Documents/test")
	require.NoError(t, err)

	expected := filepath.Join(home, "Documents", "test")
	assert.Equal(t, expected, result)
}

func TestResolvePath_AbsolutePath(t *testing.T) {
	result, err := ResolvePath("/tmp/test/path")
	require.NoError(t, err)

	assert.Equal(t, "/tmp/test/path", result)
}
