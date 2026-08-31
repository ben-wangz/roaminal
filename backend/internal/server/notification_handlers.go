package server

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/notifications"
)

type notificationSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		Auth   string `json:"auth"`
		P256dh string `json:"p256dh"`
	} `json:"keys"`
}

type notificationPreferenceRequest struct {
	ConnectionDefinitionID string `json:"connectionDefinitionId"`
	TmuxSessionName        string `json:"tmuxSessionName"`
	Enabled                bool   `json:"enabled"`
	RunningToRelax         bool   `json:"runningToRelax"`
	RunningToError         bool   `json:"runningToError"`
}

type notificationPreferenceCollectionResponse struct {
	Preferences []notifications.Preference `json:"preferences"`
}

func (s *Server) notificationConfig(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, http.StatusOK, s.notifications.Configuration())
}

func (s *Server) listNotificationPreferences(w http.ResponseWriter, r *http.Request, _ string) {
	preferences, err := s.notifications.Preferences(r.Context())
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notificationPreferenceCollectionResponse{Preferences: preferences})
}

func (s *Server) updateNotificationPreference(w http.ResponseWriter, r *http.Request, _ string) {
	var body notificationPreferenceRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	preference, err := s.notifications.SetPreference(r.Context(), notifications.PreferenceInput{
		ConnectionDefinitionID: body.ConnectionDefinitionID, TmuxSessionName: body.TmuxSessionName,
		Enabled: body.Enabled, RunningToRelax: body.RunningToRelax, RunningToError: body.RunningToError,
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preference)
}

func (s *Server) registerNotificationSubscription(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body notificationSubscriptionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	result, err := s.notifications.Register(r.Context(), sessionID, notifications.SubscriptionInput{
		Endpoint: body.Endpoint, AuthKey: body.Keys.Auth, P256dhKey: body.Keys.P256dh,
	})
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) deleteNotificationSubscription(w http.ResponseWriter, r *http.Request, sessionID string) {
	if err := s.notifications.Delete(r.Context(), sessionID, strings.TrimSpace(r.PathValue("subscriptionId"))); err != nil {
		writeNotificationError(w, err)
		return
	}
	writeSuccess(w)
}

func (s *Server) deleteNotificationSubscriptions(w http.ResponseWriter, r *http.Request, sessionID string) {
	if err := s.notifications.DeleteAll(r.Context(), sessionID); err != nil {
		writeNotificationError(w, err)
		return
	}
	writeSuccess(w)
}

func writeNotificationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notifications.ErrInvalidSubscription):
		writeCodedError(w, http.StatusBadRequest, "The browser notification subscription is invalid.", "notification_subscription_invalid", nil)
	case errors.Is(err, notifications.ErrUnavailable):
		retryable := false
		writeCodedErrorWithRetry(w, http.StatusServiceUnavailable, "Browser notifications are not configured.", "notifications_unavailable", nil, &retryable)
	case errors.Is(err, notifications.ErrStoreUnavailable):
		writeCodedError(w, http.StatusServiceUnavailable, "Browser notification storage is unavailable.", "notification_store_unavailable", nil)
	case errors.Is(err, notifications.ErrInvalidPreference):
		writeCodedError(w, http.StatusBadRequest, "The notification preference is invalid.", "notification_preference_invalid", nil)
	case errors.Is(err, notifications.ErrPreferenceStoreUnavailable):
		writeCodedError(w, http.StatusServiceUnavailable, "Notification preference storage is unavailable.", "notification_preference_store_unavailable", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func logNotificationCleanup(operation string, err error) {
	log.Printf("level=INFO event=web_push_subscription_cleanup_failed operation=%q error_type=%T", operation, err)
}
