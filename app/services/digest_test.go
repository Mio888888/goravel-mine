package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSHA256HelpersKeepExpectedFormats(t *testing.T) {
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	require.Equal(t, emptySHA256, sha256Hex(nil))
	require.Equal(t, "sha256:"+emptySHA256, digestBytes(nil))
	require.Len(t, sha256Hex([]byte("goravel")), 64)
	require.True(t, strings.HasPrefix(digestBytes([]byte("goravel")), "sha256:"))
}
