package ttl

import (
	"context"
	"errors"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// PermissionChecker is the subset of the Mattermost plugin API used for the D2
// permission check. The real plugin.API satisfies it.
type PermissionChecker interface {
	HasPermissionTo(userID string, permission *model.Permission) bool
	HasPermissionToChannel(userID, channelID string, permission *model.Permission) bool
	GetChannel(channelID string) (*model.Channel, *model.AppError)
	GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError)
	LogError(msg string, keyvals ...any)
}

// Domain errors returned by the service. The API layer (V2.2) maps these to
// HTTP statuses: ErrInvalidTTL->400, ErrForbidden->403, ErrChannelNotFound->404.
var (
	ErrForbidden       = errors.New("ttl: actor lacks permission to manage this channel's TTL")
	ErrChannelNotFound = errors.New("ttl: channel not found")
)

// Service is the TTL configuration domain service: permission check (D2),
// validation (D4 range) and default-OFF persistence (D4).
type Service struct {
	store TTLSettingStore
	perm  PermissionChecker
}

// NewService wires a TTL service with its persistence and permission ports.
func NewService(store TTLSettingStore, perm PermissionChecker) *Service {
	return &Service{store: store, perm: perm}
}

// SetTTL validates the TTL and, if the actor may manage the channel (D2),
// persists it. setAt is the authoritative clock (injectable for tests).
func (s *Service) SetTTL(ctx context.Context, actorID, channelID string, d time.Duration, setAt time.Time) error {
	_ = ctx // reserved for future cancellation; store calls are synchronous

	if err := ValidateTTL(d); err != nil {
		return err
	}
	if err := s.checkCanManage(actorID, channelID); err != nil {
		return err
	}
	return s.store.Set(channelID, TTLSetting{
		DurationSeconds: int64(d.Seconds()),
		SetBy:           actorID,
		SetAt:           setAt.UnixMilli(),
	})
}

// GetTTL returns the channel's TTL and whether one is set. Unset channels
// return (0, false, nil) — the default-OFF behaviour (D4).
func (s *Service) GetTTL(ctx context.Context, channelID string) (time.Duration, bool, error) {
	_ = ctx

	setting, err := s.store.Get(channelID)
	if err != nil {
		return 0, false, err
	}
	if setting == nil {
		return 0, false, nil
	}
	return time.Duration(setting.DurationSeconds) * time.Second, true, nil
}

// GetSetting returns the full TTL record for the channel, or nil when no TTL is
// set (default OFF, D4). Used by the API layer which needs set_by/set_at.
func (s *Service) GetSetting(ctx context.Context, channelID string) (*TTLSetting, error) {
	_ = ctx
	return s.store.Get(channelID)
}

// ClearTTL removes the channel's TTL after a permission check (D2).
func (s *Service) ClearTTL(ctx context.Context, actorID, channelID string) error {
	_ = ctx

	if err := s.checkCanManage(actorID, channelID); err != nil {
		return err
	}
	return s.store.Clear(channelID)
}

// checkCanManage authorises a TTL change. Policy: any channel member may set or
// clear a channel's TTL.
//
// Membership is verified with GetChannelMember — a distinct plugin-API call
// from GetChannel (observed to return (nil, nil) for valid channels on
// Mattermost 10.x) and from the permission system (HasPermissionTo /
// HasPermissionToChannel, which under-report for system admins on the same
// version). System and channel admins remain fast paths; membership is
// authoritative.
//
// When every path fails the raw result of each plugin-API call is logged, so a
// misbehaving server is identifiable from the plugin logs.
func (s *Service) checkCanManage(actorID, channelID string) error {
	if s.perm.HasPermissionTo(actorID, model.PermissionManageSystem) {
		return nil
	}
	managePub := s.perm.HasPermissionToChannel(actorID, channelID, model.PermissionManagePublicChannelProperties)
	managePriv := s.perm.HasPermissionToChannel(actorID, channelID, model.PermissionManagePrivateChannelProperties)
	if managePub || managePriv {
		return nil
	}
	member, memErr := s.perm.GetChannelMember(channelID, actorID)
	if memErr == nil && member != nil {
		return nil
	}
	// Diagnostic: every authorisation path failed — log what each call returned.
	ch, chErr := s.perm.GetChannel(channelID)
	s.perm.LogError("disappear: TTL authorisation denied",
		"actor", actorID, "channel_id", channelID,
		"manage_pub", managePub, "manage_priv", managePriv,
		"member_nil", member == nil, "member_err", memErr,
		"channel_nil", ch == nil, "channel_err", chErr)
	return ErrForbidden
}
