package main

import (
	"context"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionAllowed(t *testing.T) {
	assert.False(t, versionAllowed("10.5.0", nil), "empty allowlist -> fail-safe")
	assert.False(t, versionAllowed("10.5.0", []string{}), "empty allowlist -> fail-safe")
	assert.True(t, versionAllowed("10.5.0", []string{"10.", "11."}), "prefix match")
	assert.False(t, versionAllowed("9.9.0", []string{"10.", "11."}), "no match")
	assert.True(t, versionAllowed("11.0.0", []string{"10.", "11."}))
	assert.False(t, versionAllowed("100.0.0", []string{"10."}), "no false-positive: '100.x' must not match '10.'")
	assert.True(t, versionAllowed("11.0.0-rc1", []string{"10.", "11."}), "pre-release suffix matches by prefix")
}

func TestPurgeDecision(t *testing.T) {
	assert.Equal(t, "soft", purgeDecision(false, "10.0.0", []string{"10."}), "EnablePurge off -> soft")
	assert.Equal(t, "skip", purgeDecision(true, "9.0.0", []string{"10."}), "untested version -> skip (fail-safe)")
	assert.Equal(t, "skip", purgeDecision(true, "10.0.0", nil), "empty allowlist -> skip")
	assert.Equal(t, "hard", purgeDecision(true, "10.5.0", []string{"10.", "11."}), "allowed -> hard")
}

// --- configPurger fakes ---

type fakeHardPurger struct {
	called int
	ids    []string
	err    error
}

func (f *fakeHardPurger) Purge(_ context.Context, postIDs []string) (int, error) {
	f.called++
	f.ids = append(f.ids, postIDs...)
	return len(postIDs), f.err
}

type fakeSoft struct {
	deleted []string
	err     *model.AppError
}

func (f *fakeSoft) DeletePost(id string) *model.AppError {
	f.deleted = append(f.deleted, id)
	return f.err
}

type fakeVersionLogger struct {
	version string
	errors  []string
	infos   []string
}

func (f *fakeVersionLogger) GetServerVersion() string      { return f.version }
func (f *fakeVersionLogger) LogInfo(msg string, _ ...any)  { f.infos = append(f.infos, msg) }
func (f *fakeVersionLogger) LogError(msg string, _ ...any) { f.errors = append(f.errors, msg) }

func newConfigPurger(t *testing.T, enablePurge bool, allowlist, version string) (*configPurger, *fakeHardPurger, *fakeSoft, *fakeVersionLogger) {
	t.Helper()
	holder := &configHolder{}
	holder.set(configuration{EnablePurge: enablePurge, PurgeSchemaAllowlistRaw: allowlist})
	hard, soft, vl := &fakeHardPurger{}, &fakeSoft{}, &fakeVersionLogger{version: version}
	return &configPurger{cfg: holder, hard: hard, soft: soft, api: vl}, hard, soft, vl
}

func TestConfigPurgerHardPath(t *testing.T) {
	cp, hard, soft, vl := newConfigPurger(t, true, "10.,11.", "10.5.0")
	n, err := cp.Purge(context.Background(), []string{"p1", "p2"})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 1, hard.called)
	assert.Equal(t, []string{"p1", "p2"}, hard.ids)
	assert.Empty(t, soft.deleted, "soft not used on hard path")
	assert.Empty(t, vl.errors)
}

func TestConfigPurgerSkipPath(t *testing.T) {
	cp, hard, soft, vl := newConfigPurger(t, true, "10.,11.", "9.0.0") // untested schema
	n, err := cp.Purge(context.Background(), []string{"p1"})
	require.ErrorIs(t, err, errSchemaSkipped, "skip returns the sentinel so the sweeper keeps the rows for retry")
	assert.Equal(t, 0, n, "skip touches no data")
	assert.Equal(t, 0, hard.called)
	assert.Empty(t, soft.deleted)
	require.Len(t, vl.errors, 1, "skip is alerted")
}

func TestConfigPurgerSoftPath(t *testing.T) {
	cp, hard, soft, vl := newConfigPurger(t, false, "10.", "10.0.0") // EnablePurge off
	n, err := cp.Purge(context.Background(), []string{"p1", "p2"})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 0, hard.called, "hard not used on soft path")
	assert.Equal(t, []string{"p1", "p2"}, soft.deleted)
	assert.Empty(t, vl.errors)
}

func TestConfigPurgerEmptyBatchIsNoOp(t *testing.T) {
	cp, hard, _, _ := newConfigPurger(t, true, "10.", "10.0.0")
	n, err := cp.Purge(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, hard.called)
}
