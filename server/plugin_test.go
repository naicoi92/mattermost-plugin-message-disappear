package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// stubLogInfo registers tolerant LogInfo expectations on the test API.
// LogInfo is variadic (msg string, keyvals ...any); the generated mock may
// record the call as one or two arguments, so both shapes are covered.
func stubLogInfo(api *plugintest.API) {
	api.On("LogInfo", mock.Anything).Maybe().Return()
	api.On("LogInfo", mock.Anything, mock.Anything).Maybe().Return()
}

func TestOnActivate(t *testing.T) {
	p := &Plugin{}
	api := &plugintest.API{}
	stubLogInfo(api)
	p.API = api

	require.NoError(t, p.OnActivate())
}

func TestOnDeactivate(t *testing.T) {
	p := &Plugin{}
	api := &plugintest.API{}
	stubLogInfo(api)
	p.API = api

	require.NoError(t, p.OnDeactivate())
}

func TestMessageHasBeenPostedIsNoOp(t *testing.T) {
	p := &Plugin{}
	post := &model.Post{Id: "post-1", CreateAt: 1, Message: "hello"}

	require.NotPanics(t, func() {
		p.MessageHasBeenPosted(nil, post)
	})
}
