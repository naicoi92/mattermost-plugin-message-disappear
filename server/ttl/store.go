// Package ttl implements the per-channel TTL configuration domain: persistence
// (plugin KV store), the TTLService (permission-checked set/get/clear per D2/D4)
// and the allowed TTL presets and validation bounds.
package ttl

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
)

// TTLSetting is the per-channel TTL configuration persisted in the plugin KV
// store. The JSON shape matches the design (doc 05 §1): {duration_s, set_by, set_at}.
type TTLSetting struct {
	DurationSeconds int64  `json:"duration_s"`
	SetBy           string `json:"set_by"`
	SetAt           int64  `json:"set_at"` // unix milliseconds
}

// KV is the subset of the Mattermost plugin KV API the store depends on.
// The real plugin.API (and the generated test mock) satisfies it.
type KV interface {
	KVGet(key string) ([]byte, *model.AppError)
	KVSetWithOptions(key string, value []byte, options model.PluginKVSetOptions) (bool, *model.AppError)
	KVDelete(key string) *model.AppError
}

// TTLSettingStore persists per-channel TTL settings (persistence port; DIP for
// testability — the service depends on this interface, not the KV implementation).
type TTLSettingStore interface {
	// Get returns the channel's TTL, or (nil, nil) when no TTL is set (default OFF, D4).
	Get(channelID string) (*TTLSetting, error)
	// Set atomically writes the TTL setting. Under concurrent sets exactly one
	// value survives (CAS retry serialises the last writer).
	Set(channelID string, setting TTLSetting) error
	// Clear removes the TTL setting (back to default OFF).
	Clear(channelID string) error
}

// ErrTooManyRetries is returned when an atomic KV write conflicts too many times.
var ErrTooManyRetries = errors.New("ttl: too many KV retries, try again later")

const defaultMaxRetries = 20

// kvStore implements TTLSettingStore over the Mattermost plugin KV store.
type kvStore struct {
	kv         KV
	maxRetries int
}

// NewKVStore wraps a Mattermost plugin KV API as a TTLSettingStore.
func NewKVStore(kv KV) TTLSettingStore {
	return &kvStore{kv: kv, maxRetries: defaultMaxRetries}
}

func kvKey(channelID string) string {
	return "ttl:" + channelID
}

// Get returns the channel's TTL, or (nil, nil) when unset (default OFF, D4).
func (s *kvStore) Get(channelID string) (*TTLSetting, error) {
	data, appErr := s.kv.KVGet(kvKey(channelID))
	if appErr != nil {
		return nil, fmt.Errorf("ttl: KVGet %q: %w", channelID, appErr)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var setting TTLSetting
	if err := json.Unmarshal(data, &setting); err != nil {
		return nil, fmt.Errorf("ttl: decode %q: %w", channelID, err)
	}
	return &setting, nil
}

// Set writes the TTL setting with a compare-and-set retry loop. Single-key KV
// writes are atomic, so concurrent sets serialise and exactly one value wins.
func (s *kvStore) Set(channelID string, setting TTLSetting) error {
	key := kvKey(channelID)
	value, err := json.Marshal(setting)
	if err != nil {
		return fmt.Errorf("ttl: encode %q: %w", channelID, err)
	}
	for range s.maxRetries {
		current, appErr := s.kv.KVGet(key)
		if appErr != nil {
			return fmt.Errorf("ttl: KVGet %q: %w", channelID, appErr)
		}
		ok, appErr := s.kv.KVSetWithOptions(key, value, model.PluginKVSetOptions{Atomic: true, OldValue: current})
		if appErr != nil {
			return fmt.Errorf("ttl: KVSet %q: %w", channelID, appErr)
		}
		if ok {
			return nil
		}
		// Conflict: another writer committed first; re-read and retry.
	}
	return ErrTooManyRetries
}

// Clear removes the TTL setting (default OFF).
func (s *kvStore) Clear(channelID string) error {
	if appErr := s.kv.KVDelete(kvKey(channelID)); appErr != nil {
		return fmt.Errorf("ttl: KVDelete %q: %w", channelID, appErr)
	}
	return nil
}
