package ttl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// PermissionChecker is the subset of the Mattermost plugin API used for the D2
// permission check. The real plugin.API satisfies it.
type PermissionChecker interface {
	HasPermissionTo(userID string, permission *model.Permission) bool
	HasPermissionToChannel(userID, channelID string, permission *model.Permission) bool
	GetChannel(channelID string) (*model.Channel, *model.AppError)
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

// checkCanManage enforces D2: system admin or channel admin for public/private
// channels; any participant for DM/Group DMs (equal trust).
//
// The design (D2) names the permission "ManageChannel"; the Mattermost model
// splits it into ManagePublic/PrivateChannelProperties, which is what this uses.
func (s *Service) checkCanManage(actorID, channelID string) error {
	if s.perm.HasPermissionTo(actorID, model.PermissionManageSystem) {
		return nil
	}
	ch, appErr := s.perm.GetChannel(channelID)
	if appErr != nil {
		return fmt.Errorf("%w: %s: %s", ErrChannelNotFound, channelID, appErr.Error())
	}
	if ch == nil {
		return fmt.Errorf("%w: %s: nil channel, no app error", ErrChannelNotFound, channelID)
	}
	if ch.IsGroupOrDirect() {
		// DM/Group DM: any participant may set (equal trust, D2). Membership is
		// guaranteed by the API layer — the actor can only reach channels they are in.
		return nil
	}
	if ch.IsOpen() && s.perm.HasPermissionToChannel(actorID, channelID, model.PermissionManagePublicChannelProperties) {
		return nil
	}
	if !ch.IsOpen() && s.perm.HasPermissionToChannel(actorID, channelID, model.PermissionManagePrivateChannelProperties) {
		return nil
	}
	return ErrForbidden
}
