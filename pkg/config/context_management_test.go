package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ContextManagementFromEnvironment(t *testing.T) {
	t.Setenv("CLOUDQUE_AGENT_CONTEXT_MANAGEMENT_MODE", "shadow")
	t.Setenv("CLOUDQUE_AGENT_CONTEXT_MANAGEMENT_SHADOW_SAMPLE_BASIS_POINTS", "2500")
	t.Setenv("CLOUDQUE_AGENT_CONTEXT_MANAGEMENT_ROLLOUT_VERSION", "test-v1")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(configPath, []byte(`
app: {name: test, port: 8080}
database:
  mysql: {host: localhost, port: 3306, username: root, database: test}
  redis: {host: localhost, port: 6379}
jwt: {secret: test-secret}
log: {filename: test.log}
email: {host: smtp.test, port: 587, username: test, password: test}
milvus: {host: localhost, port: 19530}
external:
  markitdown: {url: "http://localhost"}
  minio: {endpoint: localhost, access_key: access, secret_key: secret, bucket: test}
security: {encryption_key: "12345678901234567890123456789012"}
`), 0o600)
	require.NoError(t, err)

	loaded, err := Load(configPath)

	require.NoError(t, err)
	assert.Equal(t, "shadow", loaded.Agent.ContextManagement.Mode)
	assert.Equal(t, uint16(2500), loaded.Agent.ContextManagement.ShadowSampleBasisPoints)
	assert.Equal(t, "test-v1", loaded.Agent.ContextManagement.RolloutVersion)
}

func TestContextManagementConfig_EnabledWritebackFallbackIsValid(t *testing.T) {
	cfg := ContextManagementConfig{
		Mode:             "enabled",
		WritebackEnabled: false,
	}
	require.NoError(t, cfg.Validate())
}
