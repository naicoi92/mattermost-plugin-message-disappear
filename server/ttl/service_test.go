package ttl

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // pure-Go sqlite driver for the in-memory store tests
)

// newTestStore returns an in-memory sqlite-backed TTL store with the schema
// migrated. A single connection (SetMaxOpenConns(1)) shares the :memory:
// database across every query on the handle, which the concurrent-set test
// relies on (each :memory: connection is otherwise a fresh database).
func newTestStore(t *testing.T) TTLSettingStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store := NewSQLStore(db, "sqlite")
	require.NoError(t, store.Migrate(context.Background()))
	return store
}

// fakePerm is a programmable PermissionChecker for the membership policy tests.
type fakePerm struct {
	sysadmin   bool
	managePub  bool
	managePriv bool
	member     *model.ChannelMember
	memberErr  *model.AppError
	// legacy fields, kept for tests that still drive the GetChannel path.
	channel    *model.Channel
	channelErr *model.AppError
}

func (f *fakePerm) HasPermissionTo(_ string, p *model.Permission) bool {
	return p == model.PermissionManageSystem && f.sysadmin
}

func (f *fakePerm) HasPermissionToChannel(_ string, _ string, p *model.Permission) bool {
	switch p {
	case model.PermissionManagePublicChannelProperties:
		return f.managePub
	case model.PermissionManagePrivateChannelProperties:
		return f.managePriv
	}
	return false
}

func (f *fakePerm) GetChannel(_ string) (*model.Channel, *model.AppError) {
	return f.channel, f.channelErr
}

func (f *fakePerm) GetChannelMember(_ string, _ string) (*model.ChannelMember, *model.AppError) {
	return f.member, f.memberErr
}

func (fakePerm) LogError(_ string, _ ...any) {}

// --- validation & presets ---

func TestValidateTTLBounds(t *testing.T) {
	cases := []struct {
		d      time.Duration
		wantOK bool
	}{
		{30 * time.Second, false}, // below min
		{time.Minute, true},       // exactly min
		{5 * time.Minute, true},
		{MaxTTL, true},                // exactly max (1y)
		{MaxTTL + time.Second, false}, // above max
		{0, false},
	}
	for _, c := range cases {
		err := ValidateTTL(c.d)
		if c.wantOK {
			assert.NoErrorf(t, err, "expected valid for %v", c.d)
		} else {
			assert.ErrorIs(t, err, ErrInvalidTTL)
		}
	}
}

func TestPresetForLabel(t *testing.T) {
	p, ok := PresetForLabel("1h")
	require.True(t, ok)
	assert.Equal(t, time.Hour, p.Duration)

	_, ok = PresetForLabel("nope")
	assert.False(t, ok)
}

// --- store: default OFF + atomic upsert ---

func TestGetUnsetReturnsDefaultOFF(t *testing.T) {
	got, err := newTestStore(t).Get("ch1")
	require.NoError(t, err)
	assert.Nil(t, got, "unset channel must read as default OFF")
}

func TestStoreSetGetClearRoundtrip(t *testing.T) {
	store := newTestStore(t)

	require.NoError(t, store.Set("ch1", TTLSetting{DurationSeconds: 3600, SetBy: "u", SetAt: 1}))
	got, err := store.Get("ch1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(3600), got.DurationSeconds)
	assert.Equal(t, "u", got.SetBy)

	// upsert overwrites in place
	require.NoError(t, store.Set("ch1", TTLSetting{DurationSeconds: 60, SetBy: "u2", SetAt: 2}))
	got, err = store.Get("ch1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(60), got.DurationSeconds, "upsert must replace the existing row")
	assert.Equal(t, "u2", got.SetBy)

	require.NoError(t, store.Clear("ch1"))
	got, err = store.Get("ch1")
	require.NoError(t, err)
	assert.Nil(t, got, "clear must return default OFF")
}

// Distinct concurrent writes must leave exactly one coherent value — no torn
// write. The DB serialises the UPSERTs (single connection); the surviving row
// is one of the legitimate inputs.
func TestStoreConcurrentDistinctSetIsCoherent(t *testing.T) {
	store := newTestStore(t)
	const goroutines = 25
	inputs := make(map[int64]struct{}, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		secs := int64(i + 1) // distinct: 1..25
		inputs[secs] = struct{}{}
		go func() {
			defer wg.Done()
			_ = store.Set("ch1", TTLSetting{DurationSeconds: secs, SetBy: "u", SetAt: secs})
		}()
	}
	wg.Wait()

	got, err := store.Get("ch1")
	require.NoError(t, err)
	require.NotNil(t, got, "exactly one value must survive")
	_, ok := inputs[got.DurationSeconds]
	assert.Truef(t, ok, "surviving value must be one of the distinct inputs, got %d", got.DurationSeconds)
}

// --- service: permission (D2) ---

func TestSetTTLRejectsInvalidTTLBeforePermission(t *testing.T) {
	// Regular member; validation (400) must fire before the permission (403) check.
	svc := NewService(newTestStore(t), &fakePerm{channel: &model.Channel{Type: model.ChannelTypeOpen}})
	err := svc.SetTTL(context.Background(), "u1", "ch1", 30*time.Second, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrInvalidTTL)
}

func TestSetTTLAllowsPublicChannelAdmin(t *testing.T) {
	perm := &fakePerm{managePub: true, channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(newTestStore(t), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLAllowsPrivateChannelAdmin(t *testing.T) {
	perm := &fakePerm{managePriv: true, channel: &model.Channel{Type: model.ChannelTypePrivate}}
	svc := NewService(newTestStore(t), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLAllowsSysadmin(t *testing.T) {
	perm := &fakePerm{sysadmin: true, channel: &model.Channel{Type: model.ChannelTypePrivate}}
	svc := NewService(newTestStore(t), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLDeniesRegularMember(t *testing.T) {
	perm := &fakePerm{channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(newTestStore(t), perm)
	err := svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestSetTTLAllowsChannelMember(t *testing.T) {
	// Any channel member may set a TTL, even without channel-admin permissions.
	perm := &fakePerm{member: &model.ChannelMember{}}
	svc := NewService(newTestStore(t), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLDeniesNonMember(t *testing.T) {
	// A user who is not a channel member (and holds no admin permission) is denied.
	perm := &fakePerm{memberErr: &model.AppError{}}
	svc := NewService(newTestStore(t), perm)
	err := svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestClearTTLDeniesRegularMember(t *testing.T) {
	perm := &fakePerm{channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(newTestStore(t), perm)
	err := svc.ClearTTL(context.Background(), "u1", "ch1")
	assert.ErrorIs(t, err, ErrForbidden)
}

// Regression (Mattermost 10.x): plugin.API.GetChannel returns (nil, nil) for a
// valid team channel. A channel admin must still be authorised via the
// channel-scoped permission, without relying on GetChannel.
func TestSetTTLAllowsAdminWhenGetChannelReturnsNil(t *testing.T) {
	perm := &fakePerm{managePub: true, channel: nil, channelErr: nil}
	svc := NewService(newTestStore(t), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

// Mattermost 10.x GetChannel (nil, nil): a non-admin member is denied as
// forbidden, not "channel not found" (the channel may well exist; only
// GetChannel is broken).
func TestSetTTLDeniesMemberWhenGetChannelReturnsNil(t *testing.T) {
	perm := &fakePerm{channel: nil, channelErr: nil}
	svc := NewService(newTestStore(t), perm)
	err := svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestGetTTLAndClearRoundtrip(t *testing.T) {
	perm := &fakePerm{sysadmin: true, channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(newTestStore(t), perm)

	// default OFF
	d, ok, err := svc.GetTTL(context.Background(), "ch1")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Zero(t, d)

	// set
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", 5*time.Minute, time.UnixMilli(1)))
	d, ok, err = svc.GetTTL(context.Background(), "ch1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 5*time.Minute, d)

	// clear → OFF again
	require.NoError(t, svc.ClearTTL(context.Background(), "u1", "ch1"))
	d, ok, err = svc.GetTTL(context.Background(), "ch1")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Zero(t, d)
}

func TestGetSetting(t *testing.T) {
	perm := &fakePerm{sysadmin: true, channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(newTestStore(t), perm)

	// unset -> nil (default OFF)
	got, err := svc.GetSetting(context.Background(), "ch1")
	require.NoError(t, err)
	assert.Nil(t, got)

	// set -> full record
	err = svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1))
	require.NoError(t, err)
	got, err = svc.GetSetting(context.Background(), "ch1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(3600), got.DurationSeconds)
	assert.Equal(t, "u1", got.SetBy)
}
