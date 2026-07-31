package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// CommandTrigger is the slash command trigger (/disappear).
const CommandTrigger = "disappear"

// ExecuteCommand handles `/disappear set <ttl> | off | status` in the current
// channel. Permission is delegated to the ttl service (D2). Responses are
// ephemeral (only the requester sees them).
func (h *Handler) ExecuteCommand(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	parts := strings.Fields(args.Command) // e.g. ["/disappear", "set", "1h"]
	if len(parts) < 2 {
		return ephemeral(args.ChannelId, helpText()), nil
	}
	switch parts[1] {
	case "set":
		return h.cmdSet(args, parts)
	case "off":
		return h.cmdOff(args)
	case "status":
		return h.cmdStatus(args)
	default:
		return ephemeral(args.ChannelId, "Unknown `/disappear` subcommand. "+helpText()), nil
	}
}

func (h *Handler) cmdSet(args *model.CommandArgs, parts []string) (*model.CommandResponse, *model.AppError) {
	if len(parts) < 3 {
		return ephemeral(args.ChannelId, "Usage: `/disappear set <duration>` (e.g. 1h, 30s, 1d, 45m)"), nil
	}
	d, err := parseTTLArg(parts[2])
	if err != nil {
		return ephemeral(args.ChannelId, err.Error()), nil
	}
	now := time.Now()
	if err := h.ttl.SetTTL(context.Background(), args.UserId, args.ChannelId, d, now); err != nil {
		return ephemeral(args.ChannelId, userMessage(err)), nil
	}
	set := &ttl.TTLSetting{DurationSeconds: int64(d.Seconds()), SetBy: args.UserId, SetAt: now.UnixMilli()}
	h.broadcast(args.ChannelId, toDTO(set))
	return ephemeral(args.ChannelId, fmt.Sprintf(":hourglass_flowing_sand: Disappearing messages enabled — auto-delete after %s.", parts[2])), nil
}

func (h *Handler) cmdOff(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if err := h.ttl.ClearTTL(context.Background(), args.UserId, args.ChannelId); err != nil {
		return ephemeral(args.ChannelId, userMessage(err)), nil
	}
	h.broadcast(args.ChannelId, nil)
	return ephemeral(args.ChannelId, "Disappearing messages disabled for this channel."), nil
}

func (h *Handler) cmdStatus(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	setting, err := h.ttl.GetSetting(context.Background(), args.ChannelId)
	if err != nil {
		return ephemeral(args.ChannelId, userMessage(err)), nil
	}
	if setting == nil {
		return ephemeral(args.ChannelId, "Disappearing messages are off for this channel."), nil
	}
	d := time.Duration(setting.DurationSeconds) * time.Second
	return ephemeral(args.ChannelId, fmt.Sprintf(":hourglass_flowing_sand: Disappearing messages on — auto-delete after %s.", d.String())), nil
}

// parseTTLArg resolves a preset label (30s/5m/1h/8h/1d/1w) or a Go duration
// (e.g. 45m, 2h). Note: time.ParseDuration does not accept "d"/"w" suffixes, so
// multi-day custom values use hours (e.g. 48h); presets cover 1d and 1w.
func parseTTLArg(s string) (time.Duration, error) {
	if p, ok := ttl.PresetForLabel(s); ok {
		return p.Duration, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("`%s` is not a valid duration (try 1h, 30s, 1d, 45m)", s)
	}
	return d, nil
}

// userMessage maps a ttl domain error to a user-facing string.
func userMessage(err error) string {
	switch {
	case errors.Is(err, ttl.ErrInvalidTTL):
		return "TTL must be between 1 minute and 1 year."
	case errors.Is(err, ttl.ErrForbidden):
		return "You don't have permission to change this channel's disappearing settings."
	case errors.Is(err, ttl.ErrChannelNotFound):
		return "Channel not found."
	default:
		return "Something went wrong. Please try again."
	}
}

func ephemeral(channelID, text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
		ChannelId:    channelID,
	}
}

func helpText() string {
	return "`/disappear set <duration>` (30s, 5m, 1h, 8h, 1d, 1w, or custom 1m–1y) · `/disappear status` · `/disappear off`"
}
