package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite" // pure-Go sqlite for the injected in-memory DB

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/api"
)

// stubLogInfo registers tolerant LogInfo expectations on the test API.
// LogInfo is variadic (msg string, keyvals ...any); the generated mock may
// record the call as one or two arguments, so both shapes are covered.
func stubLogInfo(api *plugintest.API) {
	api.On("LogInfo", mock.Anything).Maybe().Return()
	api.On("LogInfo", mock.Anything, mock.Anything).Maybe().Return()
}

// activatedPlugin returns a Plugin with OnActivate run against a mocked API
// (no DB driver, so the expire index + sweeper are not wired — only the TTL/API).
func activatedPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := &Plugin{}
	mp := &plugintest.API{}
	stubLogInfo(mp)
	mp.On("RegisterCommand", mock.Anything).Maybe().Return(nil)
	mp.On("LoadPluginConfiguration", mock.Anything).Maybe().Return(nil)
	p.API = mp

	// Inject an in-memory DB so OnActivate wires the TTL service + expire index.
	// The sweeper stays off: tests set no Driver.
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	p.db = db
	p.sqlDriver = "sqlite"

	require.NoError(t, p.OnActivate())
	return p
}

func TestOnActivate(t *testing.T) {
	p := activatedPlugin(t)
	require.NotNil(t, p.ttlService, "OnActivate must wire the TTL service")
	require.NotNil(t, p.apiHandler, "OnActivate must wire the API handler")
}

func TestOnDeactivate(t *testing.T) {
	p := &Plugin{}
	mp := &plugintest.API{}
	stubLogInfo(mp)
	p.API = mp
	require.NoError(t, p.OnDeactivate())
}

func TestServeHTTPDelegatesToAPI(t *testing.T) {
	p := activatedPlugin(t)
	r := httptest.NewRequest(http.MethodGet, "/ttl/ch1", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusOK, w.Code) // unset channel -> {"ttl": null}
}

func TestExecuteCommandDelegates(t *testing.T) {
	p := activatedPlugin(t)
	resp, appErr := p.ExecuteCommand(nil, &model.CommandArgs{UserId: "u1", ChannelId: "ch1", Command: "/disappear status"})

	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeEphemeral, resp.ResponseType)
	assert.Contains(t, resp.Text, "off", "unset channel status is 'off'")
}

func TestDisappearCommand(t *testing.T) {
	cmd := (&Plugin{}).disappearCommand()
	assert.Equal(t, api.CommandTrigger, cmd.Trigger)
	assert.True(t, cmd.AutoComplete)
}

func TestMessageHasBeenPostedIsNoOpWhenExpiryDisabled(t *testing.T) {
	p := &Plugin{} // expiryService is nil (no DB)
	require.NotPanics(t, func() {
		p.MessageHasBeenPosted(nil, &model.Post{Id: "post-1", CreateAt: 1, Message: "hello"})
	})
}

// When the expire index is wired, MessageHasBeenPosted runs OnPostCreated through it.
func TestMessageHasBeenPostedRunsExpiryWhenWired(t *testing.T) {
	p := activatedPlugin(t)
	// expiryService is wired by activatedPlugin; TTL unset -> OnPostCreated skips indexing, no panic.
	require.NotPanics(t, func() {
		p.MessageHasBeenPosted(nil, &model.Post{Id: "p1", ChannelId: "c1", CreateAt: 1000})
	})
}
