package ttl

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

// fakeKV is an in-memory, mutex-guarded KV implementing the CAS semantics of
// the Mattermost plugin KV store.
type fakeKV struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeKV() *fakeKV { return &fakeKV{data: map[string][]byte{}} }

func (f *fakeKV) KVGet(key string) ([]byte, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return bytes.Clone(f.data[key]), nil
}

func (f *fakeKV) KVSetWithOptions(key string, value []byte, opts model.PluginKVSetOptions) (bool, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if opts.Atomic && !bytes.Equal(f.data[key], opts.OldValue) {
		return false, nil // conflict
	}
	f.data[key] = bytes.Clone(value)
	return true, nil
}

func (f *fakeKV) KVDelete(key string) *model.AppError {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	return nil
}

// fakePerm is a programmable PermissionChecker for D2 tests.
type fakePerm struct {
	sysadmin   bool
	managePub  bool
	managePriv bool
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

// --- store: default OFF + atomic concurrent set ---

func TestGetUnsetReturnsDefaultOFF(t *testing.T) {
	got, err := NewKVStore(newFakeKV()).Get("ch1")
	require.NoError(t, err)
	assert.Nil(t, got, "unset channel must read as default OFF")
}

func TestKVStoreConcurrentSetExactlyOneSurvives(t *testing.T) {
	store := NewKVStore(newFakeKV())
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_ = store.Set("ch1", TTLSetting{DurationSeconds: 60, SetBy: "u", SetAt: 1})
		}()
	}
	wg.Wait()

	got, err := store.Get("ch1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(60), got.DurationSeconds)
}

// --- service: permission (D2) ---

func TestSetTTLRejectsInvalidTTLBeforePermission(t *testing.T) {
	// Regular member; validation (400) must fire before the permission (403) check.
	svc := NewService(NewKVStore(newFakeKV()), &fakePerm{channel: &model.Channel{Type: model.ChannelTypeOpen}})
	err := svc.SetTTL(context.Background(), "u1", "ch1", 30*time.Second, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrInvalidTTL)
}

func TestSetTTLAllowsPublicChannelAdmin(t *testing.T) {
	perm := &fakePerm{managePub: true, channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(NewKVStore(newFakeKV()), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLAllowsPrivateChannelAdmin(t *testing.T) {
	perm := &fakePerm{managePriv: true, channel: &model.Channel{Type: model.ChannelTypePrivate}}
	svc := NewService(NewKVStore(newFakeKV()), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLAllowsSysadmin(t *testing.T) {
	perm := &fakePerm{sysadmin: true, channel: &model.Channel{Type: model.ChannelTypePrivate}}
	svc := NewService(NewKVStore(newFakeKV()), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLDeniesRegularMember(t *testing.T) {
	perm := &fakePerm{channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(NewKVStore(newFakeKV()), perm)
	err := svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestSetTTLDMAllowsAnyParticipant(t *testing.T) {
	// DM: any participant may set regardless of channel-property flags (D2).
	perm := &fakePerm{channel: &model.Channel{Type: model.ChannelTypeDirect}}
	svc := NewService(NewKVStore(newFakeKV()), perm)
	require.NoError(t, svc.SetTTL(context.Background(), "u1", "dm1", time.Hour, time.UnixMilli(1)))
}

func TestSetTTLChannelNotFound(t *testing.T) {
	perm := &fakePerm{channel: nil, channelErr: &model.AppError{}}
	svc := NewService(NewKVStore(newFakeKV()), perm)
	err := svc.SetTTL(context.Background(), "u1", "ch1", time.Hour, time.UnixMilli(1))
	assert.ErrorIs(t, err, ErrChannelNotFound)
}

func TestGetTTLAndClearRoundtrip(t *testing.T) {
	perm := &fakePerm{sysadmin: true, channel: &model.Channel{Type: model.ChannelTypeOpen}}
	svc := NewService(NewKVStore(newFakeKV()), perm)

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
