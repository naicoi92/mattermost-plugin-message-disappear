// Package api exposes the disappearing-messages HTTP and slash-command surface,
// delegating permission and persistence to the ttl service (D2/D4).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/naicoi92/mattermost-plugin-message-disappear/server/ttl"
)

// PluginID is the manifest plugin id; Mattermost serves plugin HTTP under
// /plugins/<id>/ and prefixes incoming paths accordingly.
const PluginID = "com.github.naicoi92.disappearing-messages"

// EventTTLChanged is the WebSocket event emitted whenever a channel's TTL
// changes; the webapp (V2.3) listens for it to update the badge/button.
const EventTTLChanged = "ttl_changed"

// TTLManager is the subset of the TTL service the API surface depends on.
// *ttl.Service satisfies it.
type TTLManager interface {
	SetTTL(ctx context.Context, actorID, channelID string, d time.Duration, setAt time.Time) error
	GetSetting(ctx context.Context, channelID string) (*ttl.TTLSetting, error)
	ClearTTL(ctx context.Context, actorID, channelID string) error
}

// Broadcaster is the subset of the plugin API used to push WebSocket events.
type Broadcaster interface {
	PublishWebSocketEvent(event string, payload map[string]any, broadcast *model.WebsocketBroadcast)
}

// Handler is the HTTP + slash-command API surface for disappearing messages.
type Handler struct {
	ttl TTLManager
	ws  Broadcaster
	mux *http.ServeMux
}

// New creates the API handler and registers its routes.
func New(mgr TTLManager, ws Broadcaster) *Handler {
	h := &Handler{ttl: mgr, ws: ws}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ttl", h.handleSet)
	mux.HandleFunc("GET /ttl/{channel_id}", h.handleGet)
	mux.HandleFunc("DELETE /ttl/{channel_id}", h.handleDelete)
	h.mux = mux
	return h
}

// ServeHTTP routes a plugin HTTP request. Mattermost prefixes the path with
// /plugins/<id>/; strip it so the router sees clean paths (tests omit it).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rest, ok := strings.CutPrefix(r.URL.Path, "/plugins/"+PluginID); ok {
		r.URL.Path = rest
	}
	h.mux.ServeHTTP(w, r)
}

// --- DTOs ---

type setTTLRequest struct {
	ChannelID  string `json:"channel_id"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

type ttlDTO struct {
	DurationSeconds int64  `json:"duration"`
	SetBy           string `json:"set_by"`
	SetAt           int64  `json:"set_at"`
}

type getTTLResponse struct {
	TTL *ttlDTO `json:"ttl"`
}

func toDTO(s *ttl.TTLSetting) *ttlDTO {
	if s == nil {
		return nil
	}
	return &ttlDTO{DurationSeconds: s.DurationSeconds, SetBy: s.SetBy, SetAt: s.SetAt}
}

// --- HTTP handlers ---

func (h *Handler) handleSet(w http.ResponseWriter, r *http.Request) {
	actorID := r.Header.Get("Mattermost-User-ID")
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, "missing Mattermost-User-ID")
		return
	}
	var req setTTLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.ChannelID == "" || req.TTLSeconds < 0 {
		writeError(w, http.StatusBadRequest, "channel_id required and ttl_seconds must be non-negative")
		return
	}

	now := time.Now()
	duration := time.Duration(req.TTLSeconds) * time.Second
	if err := h.ttl.SetTTL(r.Context(), actorID, req.ChannelID, duration, now); err != nil {
		writeDomainError(w, err)
		return
	}
	// SetTTL stored exactly this setting (deterministic); reflect it in the response.
	set := &ttl.TTLSetting{DurationSeconds: req.TTLSeconds, SetBy: actorID, SetAt: now.UnixMilli()}
	h.broadcast(req.ChannelID, toDTO(set))
	writeJSON(w, http.StatusCreated, toDTO(set))
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	actorID := r.Header.Get("Mattermost-User-ID")
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, "missing Mattermost-User-ID")
		return
	}
	channelID := r.PathValue("channel_id")
	setting, err := h.ttl.GetSetting(r.Context(), channelID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, getTTLResponse{TTL: toDTO(setting)}) // nil -> {"ttl": null}
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	actorID := r.Header.Get("Mattermost-User-ID")
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, "missing Mattermost-User-ID")
		return
	}
	channelID := r.PathValue("channel_id")
	if err := h.ttl.ClearTTL(r.Context(), actorID, channelID); err != nil {
		writeDomainError(w, err)
		return
	}
	h.broadcast(channelID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// broadcast emits ttl_changed for a channel to its members.
func (h *Handler) broadcast(channelID string, t *ttlDTO) {
	h.ws.PublishWebSocketEvent(EventTTLChanged,
		map[string]any{"channel_id": channelID, "ttl": t},
		&model.WebsocketBroadcast{ChannelId: channelID})
}

// writeDomainError maps ttl domain errors to HTTP statuses (doc 06 §1).
func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ttl.ErrInvalidTTL):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ttl.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ttl.ErrChannelNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ttl.ErrTooManyRetries):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, struct {
		Message string `json:"message"`
	}{Message: msg})
}
