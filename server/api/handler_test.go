package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"

	_ "modernc.org/sqlite" // pure-Go sqlite driver for the end-to-end store
)

// Compile-time: the real ttl.Service satisfies the TTLManager port the API depends on.
var _ TTLManager = (*ttl.Service)(nil)

// --- fakes ---

type fakeTTL struct {
	mu       sync.Mutex
	settings map[string]*ttl.TTLSetting
	setErr   error
	getErr   error
	clearErr error
}

func newFakeTTL() *fakeTTL { return &fakeTTL{settings: map[string]*ttl.TTLSetting{}} }

func (f *fakeTTL) SetTTL(_ context.Context, _ string, channelID string, d time.Duration, setAt time.Time) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings[channelID] = &ttl.TTLSetting{DurationSeconds: int64(d.Seconds()), SetBy: "actor", SetAt: setAt.UnixMilli()}
	return nil
}

func (f *fakeTTL) GetSetting(_ context.Context, channelID string) (*ttl.TTLSetting, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings[channelID], nil
}

func (f *fakeTTL) ClearTTL(_ context.Context, _ string, channelID string) error {
	if f.clearErr != nil {
		return f.clearErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.settings, channelID)
	return nil
}

type fakeBroadcaster struct {
	mu     sync.Mutex
	events []broadcastEvent
}

type broadcastEvent struct {
	event     string
	payload   map[string]any
	channelID string
}

func (b *fakeBroadcaster) PublishWebSocketEvent(event string, payload map[string]any, bc *model.WebsocketBroadcast) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, broadcastEvent{event: event, payload: payload, channelID: bc.ChannelId})
}

func (b *fakeBroadcaster) last() *broadcastEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return nil
	}
	e := b.events[len(b.events)-1]
	return &e
}

// --- HTTP ---

func TestPostTTLValid(t *testing.T) {
	mgr, ws := newFakeTTL(), &fakeBroadcaster{}
	h := New(mgr, ws)

	body := strings.NewReader(`{"channel_id":"ch1","ttl_seconds":3600}`)
	r := httptest.NewRequest(http.MethodPost, "/ttl", body)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code)
	var got ttlDTO
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, int64(3600), got.DurationSeconds)
	assert.Equal(t, "u1", got.SetBy)

	ev := ws.last()
	require.NotNil(t, ev)
	assert.Equal(t, EventTTLChanged, ev.event)
	assert.Equal(t, "ch1", ev.channelID)
	assert.NotNil(t, ev.payload["ttl"])
}

func TestPostTTLNoAuth(t *testing.T) {
	h := New(newFakeTTL(), &fakeBroadcaster{})
	r := httptest.NewRequest(http.MethodPost, "/ttl", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// newSQLStore + stubPerm drive the REAL ttl.Service end-to-end through HTTP, so
// the handler's status mapping is verified against genuine validation/permission.
func newSQLStore(t *testing.T) ttl.TTLSettingStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store := ttl.NewSQLStore(db, "sqlite")
	require.NoError(t, store.Migrate(context.Background()))
	return store
}

type stubPerm struct{ channel *model.Channel }

func (stubPerm) HasPermissionTo(string, *model.Permission) bool                { return false }
func (stubPerm) HasPermissionToChannel(string, string, *model.Permission) bool { return false }
func (s stubPerm) GetChannel(string) (*model.Channel, *model.AppError)         { return s.channel, nil }
func (stubPerm) GetChannelMember(string, string) (*model.ChannelMember, *model.AppError) {
	return nil, nil
}

func (stubPerm) LogError(_ string, _ ...any) {}

func TestPostTTLEndToEndValidationAndPermission(t *testing.T) {
	svc := ttl.NewService(newSQLStore(t), stubPerm{channel: &model.Channel{Type: model.ChannelTypeOpen}})
	h := New(svc, &fakeBroadcaster{})

	// invalid range (< 1m) -> 400 via real validation
	r := httptest.NewRequest(http.MethodPost, "/ttl", strings.NewReader(`{"channel_id":"ch1","ttl_seconds":30}`))
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// regular member on a public channel -> 403 via real permission check
	r = httptest.NewRequest(http.MethodPost, "/ttl", strings.NewReader(`{"channel_id":"ch1","ttl_seconds":3600}`))
	r.Header.Set("Mattermost-User-ID", "u1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPostTTLForbidden(t *testing.T) {
	mgr := newFakeTTL()
	mgr.setErr = ttl.ErrForbidden
	h := New(mgr, &fakeBroadcaster{})
	body := strings.NewReader(`{"channel_id":"ch1","ttl_seconds":3600}`)
	r := httptest.NewRequest(http.MethodPost, "/ttl", body)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetTTLSetAndUnset(t *testing.T) {
	mgr := newFakeTTL()
	mgr.settings["ch1"] = &ttl.TTLSetting{DurationSeconds: 300, SetBy: "u2", SetAt: 7}
	h := New(mgr, &fakeBroadcaster{})

	// set
	r := httptest.NewRequest(http.MethodGet, "/ttl/ch1", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var resp getTTLResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.TTL)
	assert.Equal(t, int64(300), resp.TTL.DurationSeconds)

	// unset -> {"ttl": null}
	r = httptest.NewRequest(http.MethodGet, "/ttl/ch2", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var resp2 getTTLResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp2))
	assert.Nil(t, resp2.TTL)
}

func TestDeleteTTLClears(t *testing.T) {
	mgr, ws := newFakeTTL(), &fakeBroadcaster{}
	mgr.settings["ch1"] = &ttl.TTLSetting{DurationSeconds: 60, SetBy: "u", SetAt: 1}
	h := New(mgr, ws)

	r := httptest.NewRequest(http.MethodDelete, "/ttl/ch1", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusNoContent, w.Code)
	_, ok := mgr.settings["ch1"]
	assert.False(t, ok)
	require.NotNil(t, ws.last())
	assert.Nil(t, ws.last().payload["ttl"]) // cleared
}

func TestServeHTTPStripsPluginPrefix(t *testing.T) {
	mgr, ws := newFakeTTL(), &fakeBroadcaster{}
	h := New(mgr, ws)

	body := strings.NewReader(`{"channel_id":"ch1","ttl_seconds":3600}`)
	r := httptest.NewRequest(http.MethodPost, "/plugins/"+PluginID+"/ttl", body)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated, w.Code) // prefix stripped -> routed
}

// --- slash command ---

func cmdArgs(command string) *model.CommandArgs {
	return &model.CommandArgs{UserId: "u1", ChannelId: "ch1", Command: command}
}

func TestSlashSet(t *testing.T) {
	mgr, ws := newFakeTTL(), &fakeBroadcaster{}
	h := New(mgr, ws)

	resp, appErr := h.ExecuteCommand(cmdArgs("/disappear set 1h"))
	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeEphemeral, resp.ResponseType)
	assert.Contains(t, resp.Text, "enabled")
	assert.NotNil(t, mgr.settings["ch1"])
	require.NotNil(t, ws.last())
}

func TestSlashSetPresetOneDay(t *testing.T) {
	mgr := newFakeTTL()
	h := New(mgr, &fakeBroadcaster{})
	_, appErr := h.ExecuteCommand(cmdArgs("/disappear set 1d"))
	require.Nil(t, appErr)
	// "1d" preset resolves to 24h even though time.ParseDuration rejects "d".
	require.NotNil(t, mgr.settings["ch1"])
	assert.Equal(t, int64(24*3600), mgr.settings["ch1"].DurationSeconds)
}

func TestSlashSetMapsInvalidTTL(t *testing.T) {
	mgr := newFakeTTL()
	mgr.setErr = ttl.ErrInvalidTTL
	h := New(mgr, &fakeBroadcaster{})
	resp, appErr := h.ExecuteCommand(cmdArgs("/disappear set 1h"))
	require.Nil(t, appErr)
	assert.Contains(t, resp.Text, "1 minute")
}

func TestSlashSetForbidden(t *testing.T) {
	mgr := newFakeTTL()
	mgr.setErr = ttl.ErrForbidden
	h := New(mgr, &fakeBroadcaster{})
	resp, appErr := h.ExecuteCommand(cmdArgs("/disappear set 1h"))
	require.Nil(t, appErr)
	assert.Contains(t, resp.Text, "permission")
}

func TestSlashStatusOnOff(t *testing.T) {
	mgr := newFakeTTL()
	mgr.settings["ch1"] = &ttl.TTLSetting{DurationSeconds: 3600, SetBy: "u", SetAt: 1}
	h := New(mgr, &fakeBroadcaster{})

	// status (on)
	resp, _ := h.ExecuteCommand(cmdArgs("/disappear status"))
	assert.Contains(t, resp.Text, "on")

	// off
	resp, _ = h.ExecuteCommand(cmdArgs("/disappear off"))
	assert.Contains(t, resp.Text, "disabled")
	_, ok := mgr.settings["ch1"]
	assert.False(t, ok)

	// status (off)
	resp, _ = h.ExecuteCommand(cmdArgs("/disappear status"))
	assert.Contains(t, resp.Text, "off")
}

func TestSlashNoSubcommandShowsHelp(t *testing.T) {
	h := New(newFakeTTL(), &fakeBroadcaster{})
	resp, appErr := h.ExecuteCommand(cmdArgs("/disappear"))
	require.Nil(t, appErr)
	assert.Contains(t, resp.Text, "/disappear set")
}

func TestPostTTLMapsNotFound(t *testing.T) {
	mgr := newFakeTTL()
	mgr.setErr = ttl.ErrChannelNotFound
	h := New(mgr, &fakeBroadcaster{})
	r := httptest.NewRequest(http.MethodPost, "/ttl", strings.NewReader(`{"channel_id":"ch1","ttl_seconds":3600}`))
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetTTLMapsError(t *testing.T) {
	mgr := newFakeTTL()
	mgr.getErr = ttl.ErrChannelNotFound
	h := New(mgr, &fakeBroadcaster{})
	r := httptest.NewRequest(http.MethodGet, "/ttl/ch1", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteTTLMapsForbidden(t *testing.T) {
	mgr := newFakeTTL()
	mgr.clearErr = ttl.ErrForbidden
	h := New(mgr, &fakeBroadcaster{})
	r := httptest.NewRequest(http.MethodDelete, "/ttl/ch1", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSlashSetCustomDuration(t *testing.T) {
	mgr := newFakeTTL()
	h := New(mgr, &fakeBroadcaster{})
	_, appErr := h.ExecuteCommand(cmdArgs("/disappear set 45m"))
	require.Nil(t, appErr)
	require.NotNil(t, mgr.settings["ch1"])
	assert.Equal(t, int64(45*60), mgr.settings["ch1"].DurationSeconds) // time.ParseDuration path
}
