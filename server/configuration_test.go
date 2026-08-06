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
	assert.Equal(t, purgeSoft, purgeDecision(false, false, "10.0.0", []string{"10."}), "EnablePurge off -> soft")
	assert.Equal(t, purgeSoft, purgeDecision(true, true, "10.5.0", []string{"10."}), "Enterprise -> soft (legal-hold safety)")
	assert.Equal(t, purgeSoft, purgeDecision(true, true, "9.0.0", []string{"10."}), "EE + untested schema -> still soft (legal-hold trumps schema guard)")
	assert.Equal(t, purgeSoft, purgeDecision(true, false, "9.0.0", []string{"10."}), "untested version -> soft (no-op footgun removed)")
	assert.Equal(t, purgeSoft, purgeDecision(true, false, "10.0.0", nil), "empty allowlist -> soft (enable purge must never be a no-op)")
	assert.Equal(t, purgeHard, purgeDecision(true, false, "10.5.0", []string{"10.", "11."}), "Team + allowed -> hard")
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
	license *model.License
	errors  []string
	infos   []string
}

func (f *fakeVersionLogger) GetServerVersion() string      { return f.version }
func (f *fakeVersionLogger) GetLicense() *model.License    { return f.license }
func (f *fakeVersionLogger) LogInfo(msg string, _ ...any)  { f.infos = append(f.infos, msg) }
func (f *fakeVersionLogger) LogError(msg string, _ ...any) { f.errors = append(f.errors, msg) }

func newConfigPurger(t *testing.T, enablePurge, isEnterprise bool, allowlist, version string) (*configPurger, *fakeHardPurger, *fakeSoft, *fakeVersionLogger) {
	t.Helper()
	holder := &configHolder{}
	holder.set(configuration{EnablePurge: enablePurge, PurgeSchemaAllowlistRaw: allowlist})
	hard, soft := &fakeHardPurger{}, &fakeSoft{}
	vl := &fakeVersionLogger{version: version}
	if isEnterprise {
		vl.license = &model.License{} // non-nil = Enterprise-licensed
	}
	return &configPurger{cfg: holder, hard: hard, soft: soft, api: vl}, hard, soft, vl
}

func TestConfigPurgerHardPath(t *testing.T) {
	cp, hard, soft, vl := newConfigPurger(t, true, false, "10.,11.", "10.5.0") // Team + allowed
	n, err := cp.Purge(context.Background(), []string{"p1", "p2"})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 1, hard.called)
	assert.Equal(t, []string{"p1", "p2"}, hard.ids)
	assert.Empty(t, soft.deleted, "soft not used on hard path")
	assert.Empty(t, vl.errors)
}

func TestConfigPurgerUntestedSchemaSoftDeletes(t *testing.T) {
	// EnablePurge on but schema not allowlisted -> must soft-delete, not skip.
	// Skipping made "enable purge" a no-op (messages never cleaned). (Bug A fix.)
	cp, hard, soft, vl := newConfigPurger(t, true, false, "10.,11.", "9.0.0") // untested schema
	n, err := cp.Purge(context.Background(), []string{"p1"})
	require.NoError(t, err, "untested schema soft-deletes; no sentinel error")
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, hard.called, "hard purge not used on untested schema")
	assert.Equal(t, []string{"p1"}, soft.deleted, "soft-delete is the safe fallback")
	assert.Empty(t, vl.errors, "soft fallback is not an error")
}

func TestConfigPurgerSoftPath(t *testing.T) {
	cp, hard, soft, vl := newConfigPurger(t, false, false, "10.", "10.0.0") // EnablePurge off
	n, err := cp.Purge(context.Background(), []string{"p1", "p2"})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 0, hard.called, "hard not used on soft path")
	assert.Equal(t, []string{"p1", "p2"}, soft.deleted)
	assert.Empty(t, vl.errors)
}

func TestConfigPurgerEnterpriseUsesSoft(t *testing.T) {
	// Enterprise-licensed: hard purge would bypass legal-hold, so soft-delete (D11).
	cp, hard, soft, vl := newConfigPurger(t, true, true, "10.,11.", "10.5.0")
	n, err := cp.Purge(context.Background(), []string{"p1"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, hard.called, "hard purge not used on Enterprise (legal-hold safety)")
	assert.Equal(t, []string{"p1"}, soft.deleted)
	assert.Empty(t, vl.errors)
}

func TestConfigPurgerEmptyBatchIsNoOp(t *testing.T) {
	cp, hard, _, _ := newConfigPurger(t, true, false, "10.", "10.0.0")
	n, err := cp.Purge(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, hard.called)
}
