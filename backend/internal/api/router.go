// Package api exposes the REST surface over the service layer.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"goalstakes/internal/ai"
	"goalstakes/internal/auth"
	"goalstakes/internal/domain"
	"goalstakes/internal/middleware"
	"goalstakes/internal/service"
	"goalstakes/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SIWEAuthenticator interface {
	IssueNonce(ctx context.Context, walletAddress string) (string, error)
	VerifyAndLogin(ctx context.Context, rawMessage, signature string) (domain.User, string, error)
}

type Config struct {
	Service           *service.Service
	Sessions          middleware.SessionVerifier
	APIKeys           middleware.APIKeyVerifier
	SIWE              SIWEAuthenticator
	AI                *ai.Manager
	TelegramBotSecret string
	PublicBaseURL     string
}

func NewRouter(cfg Config) http.Handler {
	h := handler{cfg: cfg}
	r := chi.NewRouter()
	r.Use(cors)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound)})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed)})
	})

	r.Get("/openapi.json", h.openapi)
	r.Get("/docs", h.docs)
	r.Get("/agent-skills/{token}", h.agentSkill)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/chains", h.listChains)
		r.Post("/auth/nonce", h.issueNonce)
		r.Post("/auth/siwe", h.verifySIWE)

		r.Group(func(r chi.Router) {
			r.Use(h.requireAuth)
			r.Get("/me", h.me)
			r.Get("/goals", h.listGoals)
			r.Post("/goals", h.createGoal)
			r.Patch("/goals/{goalID}", h.updateGoal)
			r.Patch("/goals/{goalID}/stake", h.setStake)
			r.Delete("/goals/{goalID}", h.archiveGoal)
			r.Get("/goals/{goalID}/progress", h.getProgress)
			r.Get("/goals/{goalID}/violations", h.listViolations)
			r.Post("/goals/{goalID}/violations", h.reportViolation)
			r.Post("/goals/{goalID}/checkins", h.logCheckIn)
			r.Get("/approvals", h.getApprovalStatus)
			r.Post("/approvals", h.recordApproval)
			r.Get("/apikeys", h.listAPIKeys)
			r.Post("/apikeys", h.createAPIKey)
			r.Delete("/apikeys/{apiKeyID}", h.revokeAPIKey)
			r.Get("/agent-links", h.listAgentLinks)
			r.Post("/agent-links", h.createAgentLink)
			r.Delete("/agent-links/{agentLinkID}", h.revokeAgentLink)
			r.Post("/telegram/link-codes", h.createTelegramLinkCode)
			r.Post("/chat", h.chat)
			r.Post("/chat/audio", h.chatAudio)
		})
	})

	r.Route("/internal/telegram", func(r chi.Router) {
		r.Use(h.requireTelegramBot)
		r.Post("/link", h.linkTelegram)
		r.Post("/message", h.telegramMessage)
		r.Post("/audio", h.telegramAudio)
		r.Post("/agent-link", h.telegramAgentLink)
	})

	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type handler struct {
	cfg Config
}

func (h handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			unauthorized(w)
			return
		}
		if strings.HasPrefix(raw, "sk_") && h.cfg.APIKeys != nil {
			key, err := h.cfg.APIKeys.Verify(r.Context(), raw)
			if err == nil {
				next.ServeHTTP(w, r.WithContext(middleware.WithUserID(r.Context(), key.UserID)))
				return
			}
		}
		if h.cfg.Sessions != nil {
			session, err := h.cfg.Sessions.VerifySessionToken(r.Context(), raw)
			if err == nil {
				next.ServeHTTP(w, r.WithContext(middleware.WithUserID(r.Context(), session.UserID)))
				return
			}
		}
		if h.cfg.APIKeys != nil {
			key, err := h.cfg.APIKeys.Verify(r.Context(), raw)
			if err == nil {
				next.ServeHTTP(w, r.WithContext(middleware.WithUserID(r.Context(), key.UserID)))
				return
			}
		}
		unauthorized(w)
	})
}

func (h handler) requireTelegramBot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(h.cfg.TelegramBotSecret) == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram bot secret is not configured"})
			return
		}
		raw, ok := bearerToken(r)
		if !ok || subtle.ConstantTimeCompare([]byte(raw), []byte(h.cfg.TelegramBotSecret)) != 1 {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h handler) issueNonce(w http.ResponseWriter, r *http.Request) {
	if h.cfg.SIWE == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "siwe auth is not configured"})
		return
	}
	var req struct {
		WalletAddress string `json:"wallet_address"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	nonce, err := h.cfg.SIWE.IssueNonce(r.Context(), req.WalletAddress)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nonce": nonce})
}

func (h handler) listChains(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.cfg.Service.ListChains())
}

func (h handler) verifySIWE(w http.ResponseWriter, r *http.Request) {
	if h.cfg.SIWE == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "siwe auth is not configured"})
		return
	}
	var req struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, token, err := h.cfg.SIWE.VerifyAndLogin(r.Context(), req.Message, req.Signature)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "token": token})
}

func (h handler) me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"user_id": userID.String()})
}

func (h handler) listGoals(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goals, err := h.cfg.Service.ListGoals(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goals)
}

func (h handler) createGoal(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req service.CreateGoalInput
	if !decodeJSON(w, r, &req) {
		return
	}
	goal, err := h.cfg.Service.CreateGoal(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, goal)
}

func (h handler) updateGoal(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	var req service.UpdateGoalInput
	if !decodeJSON(w, r, &req) {
		return
	}
	goal, err := h.cfg.Service.UpdateGoal(r.Context(), userID, goalID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}

func (h handler) setStake(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	var req service.SetStakeInput
	if !decodeJSON(w, r, &req) {
		return
	}
	goal, err := h.cfg.Service.SetStake(r.Context(), userID, goalID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}

func (h handler) archiveGoal(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	if err := h.cfg.Service.ArchiveGoal(r.Context(), userID, goalID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) getProgress(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	progress, err := h.cfg.Service.GetProgress(r.Context(), userID, goalID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

func (h handler) listViolations(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	violations, err := h.cfg.Service.ListViolations(r.Context(), userID, goalID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, violations)
}

func (h handler) reportViolation(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	var req service.ReportViolationInput
	if !decodeJSON(w, r, &req) {
		return
	}
	violation, err := h.cfg.Service.ReportViolation(r.Context(), userID, goalID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, violation)
}

func (h handler) logCheckIn(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	goalID, ok := pathUUID(w, r, "goalID")
	if !ok {
		return
	}
	var req service.LogCheckInInput
	if !decodeJSON(w, r, &req) {
		return
	}
	checkIn, err := h.cfg.Service.LogCheckIn(r.Context(), userID, goalID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, checkIn)
}

func (h handler) getApprovalStatus(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	status, err := h.cfg.Service.GetApprovalStatus(r.Context(), userID, r.URL.Query().Get("chain"), r.URL.Query().Get("token_symbol"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h handler) recordApproval(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req service.RecordApprovalInput
	if !decodeJSON(w, r, &req) {
		return
	}
	status, err := h.cfg.Service.RecordApproval(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h handler) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	keys, err := h.cfg.Service.ListAPIKeys(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (h handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := h.cfg.Service.CreateAPIKey(r.Context(), userID, req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	apiKeyID, ok := pathUUID(w, r, "apiKeyID")
	if !ok {
		return
	}
	if err := h.cfg.Service.RevokeAPIKey(r.Context(), userID, apiKeyID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) listAgentLinks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	links, err := h.cfg.Service.ListAgentLinks(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, links)
}

func (h handler) createAgentLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req service.CreateAgentLinkInput
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := h.cfg.Service.CreateAgentLink(r.Context(), userID, req, h.publicBaseURL(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h handler) revokeAgentLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	agentLinkID, ok := pathUUID(w, r, "agentLinkID")
	if !ok {
		return
	}
	if err := h.cfg.Service.RevokeAgentLink(r.Context(), userID, agentLinkID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) agentSkill(w http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "token")
	token, ok := strings.CutSuffix(raw, ".md")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound)})
		return
	}
	body, err := h.cfg.Service.AgentSkillMarkdown(r.Context(), token, h.publicBaseURL(r))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (h handler) publicBaseURL(r *http.Request) string {
	if configured := strings.TrimRight(strings.TrimSpace(h.cfg.PublicBaseURL), "/"); configured != "" {
		return configured
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + host
}

func (h handler) createTelegramLinkCode(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r.Context())
	var req struct{}
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := h.cfg.Service.CreateTelegramLinkCode(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h handler) linkTelegram(w http.ResponseWriter, r *http.Request) {
	var req service.LinkTelegramChatInput
	if !decodeJSON(w, r, &req) {
		return
	}
	link, err := h.cfg.Service.LinkTelegramChat(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reply":         "Linked to Goal Stakes.",
		"telegram_link": link,
	})
}

func (h handler) telegramMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChatID    int64  `json:"chat_id"`
		ChatKind  string `json:"chat_kind,omitempty"`
		MessageID int64  `json:"message_id,omitempty"`
		Text      string `json:"text"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	link, err := h.cfg.Service.ResolveTelegramChat(r.Context(), req.ChatID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]string{"reply": "Link this Telegram chat first with /link <code> from Goal Stakes Settings."})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	reply, err := h.handleTelegramText(r.Context(), link.UserID, req.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reply": reply})
}

type telegramAudioRequest struct {
	ChatID         int64
	ChatKind       string
	MessageID      int64
	ConversationID domain.UUID
	Audio          ai.AudioInput
}

func (h handler) telegramAudio(w http.ResponseWriter, r *http.Request) {
	req, ok := parseTelegramAudioRequest(w, r)
	if !ok {
		return
	}
	link, err := h.cfg.Service.ResolveTelegramChat(r.Context(), req.ChatID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]string{"reply": "Link this Telegram chat first with /link <code> from Goal Stakes Settings."})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if h.cfg.AI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ai manager is not configured"})
		return
	}
	result, err := h.cfg.AI.ChatAudio(r.Context(), link.UserID, ai.AudioChatInput{
		Audio:          req.Audio,
		ConversationID: req.ConversationID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h handler) telegramAgentLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChatID   int64  `json:"chat_id"`
		ChatKind string `json:"chat_kind,omitempty"`
		Name     string `json:"name,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	link, err := h.cfg.Service.ResolveTelegramChat(r.Context(), req.ChatID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]string{"reply": "Link this Telegram chat first with /link <code> from Goal Stakes Settings."})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "telegram"
	}
	created, err := h.cfg.Service.CreateAgentLink(r.Context(), link.UserID, service.CreateAgentLinkInput{Name: name}, h.publicBaseURL(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"reply":      "Own-agent skill link: " + created.SkillURL,
		"skill_url":  created.SkillURL,
		"agent_link": created.AgentLink,
	})
}

func parseTelegramAudioRequest(w http.ResponseWriter, r *http.Request) (telegramAudioRequest, bool) {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	mediaType := contentType
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		mediaType = parsed
	}
	if strings.EqualFold(mediaType, "multipart/form-data") {
		return parseTelegramMultipartAudioRequest(w, r)
	}
	return parseTelegramBinaryAudioRequest(w, r)
}

func parseTelegramMultipartAudioRequest(w http.ResponseWriter, r *http.Request) (telegramAudioRequest, bool) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return telegramAudioRequest{}, false
	}
	chatID, ok := int64FormField(w, r, "chat_id", true)
	if !ok {
		return telegramAudioRequest{}, false
	}
	messageID, ok := int64FormField(w, r, "message_id", false)
	if !ok {
		return telegramAudioRequest{}, false
	}
	conversationID, ok := uuidFormField(w, r, "conversation_id")
	if !ok {
		return telegramAudioRequest{}, false
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "audio file is required"})
		return telegramAudioRequest{}, false
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read audio file"})
		return telegramAudioRequest{}, false
	}
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !supportedAudioContentType(contentType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported audio content type"})
		return telegramAudioRequest{}, false
	}
	return telegramAudioRequest{
		ChatID:         chatID,
		ChatKind:       strings.TrimSpace(r.FormValue("chat_kind")),
		MessageID:      messageID,
		ConversationID: conversationID,
		Audio: ai.AudioInput{
			Filename:    header.Filename,
			ContentType: contentType,
			Data:        raw,
		},
	}, true
}

func parseTelegramBinaryAudioRequest(w http.ResponseWriter, r *http.Request) (telegramAudioRequest, bool) {
	chatID, ok := int64QueryField(w, r, "chat_id", true)
	if !ok {
		return telegramAudioRequest{}, false
	}
	messageID, ok := int64QueryField(w, r, "message_id", false)
	if !ok {
		return telegramAudioRequest{}, false
	}
	conversationID, ok := uuidQueryField(w, r, "conversation_id")
	if !ok {
		return telegramAudioRequest{}, false
	}
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !supportedAudioContentType(contentType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported audio content type"})
		return telegramAudioRequest{}, false
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read audio body"})
		return telegramAudioRequest{}, false
	}
	return telegramAudioRequest{
		ChatID:         chatID,
		ChatKind:       strings.TrimSpace(r.URL.Query().Get("chat_kind")),
		MessageID:      messageID,
		ConversationID: conversationID,
		Audio: ai.AudioInput{
			Filename:    strings.TrimSpace(r.URL.Query().Get("filename")),
			ContentType: contentType,
			Data:        raw,
		},
	}, true
}

func int64FormField(w http.ResponseWriter, r *http.Request, name string, required bool) (int64, bool) {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		if required {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": name + " is required"})
			return 0, false
		}
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return 0, false
	}
	return value, true
}

func int64QueryField(w http.ResponseWriter, r *http.Request, name string, required bool) (int64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		if required {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": name + " is required"})
			return 0, false
		}
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return 0, false
	}
	return value, true
}

func uuidFormField(w http.ResponseWriter, r *http.Request, name string) (domain.UUID, bool) {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		return domain.UUID{}, true
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return domain.UUID{}, false
	}
	return parsed, true
}

func uuidQueryField(w http.ResponseWriter, r *http.Request, name string) (domain.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return domain.UUID{}, true
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return domain.UUID{}, false
	}
	return parsed, true
}

func (h handler) handleTelegramText(ctx context.Context, userID domain.UUID, text string) (string, error) {
	text = strings.TrimSpace(text)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "Send /goals, /create, /done, /violate, /progress, /archive, or a normal goal message.", nil
	}
	command := telegramCommand(fields[0])
	if command == "" {
		return h.telegramFreeText(ctx, userID, text)
	}
	args := fields[1:]
	switch command {
	case "/goals":
		return h.telegramListGoals(ctx, userID)
	case "/create":
		return h.telegramCreateGoal(ctx, userID, args)
	case "/done":
		return h.telegramLogCheckIn(ctx, userID, args)
	case "/violate":
		return h.telegramReportViolation(ctx, userID, args)
	case "/progress":
		return h.telegramProgress(ctx, userID, args)
	case "/archive":
		return h.telegramArchive(ctx, userID, args)
	default:
		return "Unknown command. Send /goals, /create, /done, /violate, /progress, /archive, or a normal goal message.", nil
	}
}

func telegramCommand(raw string) string {
	if !strings.HasPrefix(raw, "/") {
		return ""
	}
	command, _, _ := strings.Cut(raw, "@")
	return strings.ToLower(strings.TrimSpace(command))
}

func (h handler) telegramListGoals(ctx context.Context, userID domain.UUID) (string, error) {
	goals, err := h.cfg.Service.ListGoals(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(goals) == 0 {
		return "No active goals.", nil
	}
	lines := make([]string, 0, len(goals))
	for _, goal := range goals {
		lines = append(lines, fmt.Sprintf("%s - %s | %s | %s | %s %s on %s", goal.ID, goal.Title, goal.Type, goal.Cadence, formatTelegramAmount(goal.StakeAmount), goal.TokenSymbol, goal.Chain))
	}
	return strings.Join(lines, "\n"), nil
}

func (h handler) telegramCreateGoal(ctx context.Context, userID domain.UUID, args []string) (string, error) {
	if len(args) < 6 {
		return "Usage: /create do|avoid daily|weekly amount USDC|USDT chain title", nil
	}
	amount, err := parseTelegramBaseUnits(args[2])
	if err != nil {
		return "Usage: /create do|avoid daily|weekly amount USDC|USDT chain title. " + err.Error(), nil
	}
	title := strings.TrimSpace(strings.Join(args[5:], " "))
	if title == "" {
		return "Usage: /create do|avoid daily|weekly amount USDC|USDT chain title", nil
	}
	goal, err := h.cfg.Service.CreateGoal(ctx, userID, service.CreateGoalInput{
		Title:       title,
		Type:        domain.GoalType(strings.ToLower(args[0])),
		Cadence:     domain.Cadence(strings.ToLower(args[1])),
		StakeAmount: amount,
		TokenSymbol: strings.ToUpper(args[3]),
		Chain:       args[4],
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Created goal %s: %s", goal.Title, goal.ID), nil
}

func (h handler) telegramLogCheckIn(ctx context.Context, userID domain.UUID, args []string) (string, error) {
	if len(args) < 1 {
		return "Usage: /done goal_id optional note", nil
	}
	goalID, err := uuid.Parse(args[0])
	if err != nil {
		return "Usage: /done goal_id optional note", nil
	}
	checkIn, err := h.cfg.Service.LogCheckIn(ctx, userID, goalID, service.LogCheckInInput{
		Note: strings.TrimSpace(strings.Join(args[1:], " ")),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Check-in recorded for %s.", checkIn.Period), nil
}

func (h handler) telegramReportViolation(ctx context.Context, userID domain.UUID, args []string) (string, error) {
	if len(args) < 1 {
		return "Usage: /violate goal_id optional reason", nil
	}
	goalID, err := uuid.Parse(args[0])
	if err != nil {
		return "Usage: /violate goal_id optional reason", nil
	}
	violation, err := h.cfg.Service.ReportViolation(ctx, userID, goalID, service.ReportViolationInput{
		Reason: strings.TrimSpace(strings.Join(args[1:], " ")),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Violation recorded for %s with status %s.", violation.Period, violation.Status), nil
}

func (h handler) telegramProgress(ctx context.Context, userID domain.UUID, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: /progress goal_id", nil
	}
	goalID, err := uuid.Parse(args[0])
	if err != nil {
		return "Usage: /progress goal_id", nil
	}
	progress, err := h.cfg.Service.GetProgress(ctx, userID, goalID)
	if err != nil {
		return "", err
	}
	completed := "no"
	if progress.CurrentPeriodCompleted {
		completed = "yes"
	}
	lines := []string{
		fmt.Sprintf("%s: %s", progress.Goal.Title, progress.Goal.ID),
		fmt.Sprintf("period: %s, completed: %s", progress.CurrentPeriod, completed),
		fmt.Sprintf("violations: %d", len(progress.Violations)),
	}
	if len(progress.Violations) > 0 {
		last := progress.Violations[len(progress.Violations)-1]
		lines = append(lines, fmt.Sprintf("latest violation: %s", last.Status))
	}
	return strings.Join(lines, "\n"), nil
}

func (h handler) telegramArchive(ctx context.Context, userID domain.UUID, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: /archive goal_id", nil
	}
	goalID, err := uuid.Parse(args[0])
	if err != nil {
		return "Usage: /archive goal_id", nil
	}
	if err := h.cfg.Service.ArchiveGoal(ctx, userID, goalID); err != nil {
		return "", err
	}
	return "Goal archived.", nil
}

func (h handler) telegramFreeText(ctx context.Context, userID domain.UUID, text string) (string, error) {
	if h.cfg.AI == nil {
		return "AI chat is not configured. Use /goals, /create, /done, /violate, /progress, or /archive.", nil
	}
	result, err := h.cfg.AI.Chat(ctx, userID, ai.ChatInput{Message: text})
	if err != nil {
		return "", err
	}
	return result.Reply, nil
}

func parseTelegramBaseUnits(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("stake must be a positive decimal with up to 6 decimals")
	}
	whole, frac, hasFrac := strings.Cut(value, ".")
	if whole == "" || !allDigits(whole) {
		return "", fmt.Errorf("stake must be a positive decimal with up to 6 decimals")
	}
	if !hasFrac {
		frac = ""
	}
	if strings.Contains(frac, ".") || len(frac) > 6 || (frac != "" && !allDigits(frac)) {
		return "", fmt.Errorf("stake must be a positive decimal with up to 6 decimals")
	}
	frac = (frac + "000000")[:6]
	combined := strings.TrimLeft(whole+frac, "0")
	if combined == "" {
		return "", fmt.Errorf("stake must be positive")
	}
	return combined, nil
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatTelegramAmount(raw string) string {
	value := strings.TrimLeft(raw, "0")
	if value == "" {
		return "0"
	}
	if len(value) <= 6 {
		value = strings.Repeat("0", 6-len(value)+1) + value
	}
	whole := value[:len(value)-6]
	frac := strings.TrimRight(value[len(value)-6:], "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

func (h handler) chat(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ai manager is not configured"})
		return
	}
	userID, _ := middleware.UserID(r.Context())
	var req ai.ChatInput
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := h.cfg.AI.Chat(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h handler) chatAudio(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ai manager is not configured"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "audio file is required"})
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read audio file"})
		return
	}
	conversationID := domain.UUID{}
	if rawConversationID := strings.TrimSpace(r.FormValue("conversation_id")); rawConversationID != "" {
		parsed, err := uuid.Parse(rawConversationID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid conversation_id"})
			return
		}
		conversationID = parsed
	}
	contentType := header.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	if !supportedAudioContentType(contentType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported audio content type"})
		return
	}
	userID, _ := middleware.UserID(r.Context())
	result, err := h.cfg.AI.ChatAudio(r.Context(), userID, ai.AudioChatInput{
		Audio: ai.AudioInput{
			Filename:    header.Filename,
			ContentType: contentType,
			Data:        raw,
		},
		ConversationID: conversationID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func supportedAudioContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/octet-stream" || strings.HasPrefix(mediaType, "audio/")
}

func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (domain.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return domain.UUID{}, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := http.StatusText(status)
	switch {
	case errors.Is(err, ai.ErrDisabled):
		status = http.StatusServiceUnavailable
		message = err.Error()
	case errors.Is(err, auth.ErrUnauthorized):
		status = http.StatusUnauthorized
		message = http.StatusText(status)
	case errors.Is(err, service.ErrInvalid):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, service.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, service.ErrChargeFailed):
		status = http.StatusBadGateway
		message = err.Error()
	case errors.Is(err, service.ErrExpired):
		status = http.StatusGone
		message = err.Error()
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	}
	if status >= http.StatusInternalServerError {
		log.Printf("api: internal error: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func unauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": http.StatusText(http.StatusUnauthorized)})
}
