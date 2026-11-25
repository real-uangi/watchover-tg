package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

const telegramAPI = "https://api.telegram.org/"

type Telegram struct{ client *http.Client }

func NewTelegram() domain.Telegram { return &Telegram{client: externalClient(20 * time.Second)} }
func (t *Telegram) call(ctx context.Context, token, method string, payload any, result any, mutating bool) error {
	raw, e := json.Marshal(payload)
	if e != nil {
		return domain.ErrInvalid
	}
	return t.request(ctx, token, method, "application/json", bytes.NewReader(raw), result, mutating)
}
func (t *Telegram) request(ctx context.Context, token, method, contentType string, body io.Reader, result any, mutating bool) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, telegramAPI+"bot"+token+"/"+method, body)
	if e != nil {
		return domain.ErrInvalid
	}
	req.Header.Set("Content-Type", contentType)
	response, e := t.client.Do(req)
	if e != nil {
		if mutating {
			return domain.ErrUnknown
		}
		return domain.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.ErrUnavailable
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if e := decoder.Decode(&envelope); e != nil {
		if mutating {
			return domain.ErrUnknown
		}
		return domain.ErrUnavailable
	}
	if !envelope.OK {
		return domain.ErrUnavailable
	}
	if result != nil {
		if len(envelope.Result) == 0 || json.Unmarshal(envelope.Result, result) != nil {
			if mutating {
				return domain.ErrUnknown
			}
			return domain.ErrUnavailable
		}
	} else {
		var success bool
		if json.Unmarshal(envelope.Result, &success) != nil || !success {
			return domain.ErrUnknown
		}
	}
	return nil
}
func (t *Telegram) Me(ctx context.Context, token string) (domain.TelegramUser, error) {
	var out domain.TelegramUser
	e := t.call(ctx, token, "getMe", map[string]any{}, &out, false)
	return out, e
}
func (t *Telegram) Chat(ctx context.Context, token string, id int64) (domain.TelegramChat, error) {
	var out domain.TelegramChat
	e := t.call(ctx, token, "getChat", map[string]any{"chat_id": id}, &out, false)
	return out, e
}
func (t *Telegram) Member(ctx context.Context, token string, chatID, userID int64) (domain.TelegramMember, error) {
	var out domain.TelegramMember
	e := t.call(ctx, token, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &out, false)
	return out, e
}
func (t *Telegram) Webhook(ctx context.Context, token, webhookURL, secret string, register bool) error {
	if !register {
		return t.call(ctx, token, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil, true)
	}
	return t.call(ctx, token, "setWebhook", map[string]any{"url": webhookURL, "secret_token": secret, "max_connections": 1, "drop_pending_updates": false, "allowed_updates": []string{"message", "edited_message", "channel_post", "edited_channel_post", "callback_query", "my_chat_member", "chat_member", "chat_join_request"}}, nil, true)
}
func (t *Telegram) WebhookInfo(ctx context.Context, token string) (domain.WebhookInfo, error) {
	var out domain.WebhookInfo
	e := t.call(ctx, token, "getWebhookInfo", map[string]any{}, &out, false)
	out.URL = strings.ReplaceAll(out.URL, token, "[redacted]")
	out.LastError = strings.ReplaceAll(out.LastError, token, "[redacted]")
	return out, e
}
func (t *Telegram) Invite(ctx context.Context, token string, chatID int64, expires time.Time) (string, error) {
	var out struct {
		Link string `json:"invite_link"`
	}
	e := t.call(ctx, token, "createChatInviteLink", map[string]any{"chat_id": chatID, "creates_join_request": true, "expire_date": expires.Unix()}, &out, true)
	return out.Link, e
}
func (t *Telegram) Join(ctx context.Context, token string, chatID, userID int64, approve bool) error {
	method := "declineChatJoinRequest"
	if approve {
		method = "approveChatJoinRequest"
	}
	return t.call(ctx, token, method, map[string]any{"chat_id": chatID, "user_id": userID}, nil, true)
}
func (t *Telegram) Act(ctx context.Context, token string, a domain.MemberAction) error {
	payload := map[string]any{"chat_id": a.ChatID, "user_id": a.UserID}
	method := ""
	switch a.Action {
	case "ban":
		method = "banChatMember"
		payload["revoke_messages"] = true
		payload["until_date"] = a.UntilDate
	case "kick":
		method = "unbanChatMember"
		payload["only_if_banned"] = false
	case "unban":
		method = "unbanChatMember"
		payload["only_if_banned"] = true
	case "mute", "unmute":
		method = "restrictChatMember"
		permissions := map[string]bool{}
		for _, key := range []string{"can_send_messages", "can_send_audios", "can_send_documents", "can_send_photos", "can_send_videos", "can_send_video_notes", "can_send_voice_notes", "can_send_polls", "can_send_other_messages", "can_add_web_page_previews", "can_change_info", "can_invite_users", "can_pin_messages", "can_manage_topics"} {
			permissions[key] = a.Action == "unmute"
		}
		payload["permissions"] = permissions
		payload["use_independent_chat_permissions"] = true
		payload["until_date"] = a.UntilDate
	default:
		return domain.ErrInvalid
	}
	return t.call(ctx, token, method, payload, nil, true)
}
func (t *Telegram) Send(ctx context.Context, token string, m domain.SendMessage) (domain.TelegramMessage, error) {
	var out domain.TelegramMessage
	if m.Kind == "text" {
		payload := map[string]any{"chat_id": m.ChatID, "text": m.Text}
		if m.ReplyToMessageID != 0 {
			payload["reply_parameters"] = map[string]any{"message_id": m.ReplyToMessageID}
		}
		e := t.call(ctx, token, "sendMessage", payload, &out, true)
		return out, e
	}
	method, field := "sendDocument", "document"
	if m.Kind == "photo" {
		method, field = "sendPhoto", "photo"
	} else if m.Kind != "document" {
		return out, domain.ErrInvalid
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	values := map[string]string{"chat_id": strconv.FormatInt(m.ChatID, 10), "caption": m.Text}
	if m.ReplyToMessageID != 0 {
		reply, _ := json.Marshal(map[string]any{"message_id": m.ReplyToMessageID})
		values["reply_parameters"] = string(reply)
	}
	for key, value := range values {
		if e := writer.WriteField(key, value); e != nil {
			return out, domain.ErrInvalid
		}
	}
	file, e := writer.CreateFormFile(field, path.Base(m.FileName))
	if e != nil {
		return out, domain.ErrInvalid
	}
	if _, e := io.Copy(file, m.File); e != nil {
		return out, domain.ErrInvalid
	}
	if e := writer.Close(); e != nil {
		return out, domain.ErrInvalid
	}
	e = t.request(ctx, token, method, writer.FormDataContentType(), &buffer, &out, true)
	return out, e
}
func (t *Telegram) Reply(ctx context.Context, token string, reply domain.Reply) (domain.TelegramMessage, error) {
	var out domain.TelegramMessage
	payload := map[string]any{"chat_id": reply.ChatID, "text": reply.Text}
	if len(reply.Buttons) > 0 {
		payload["reply_markup"] = map[string]any{"inline_keyboard": reply.Buttons}
	}
	e := t.call(ctx, token, "sendMessage", payload, &out, true)
	return out, e
}
func (t *Telegram) AnswerCallback(ctx context.Context, token, id string) error {
	return t.call(ctx, token, "answerCallbackQuery", map[string]any{"callback_query_id": id}, nil, true)
}
func (t *Telegram) Download(ctx context.Context, token string, message domain.Message) (domain.File, error) {
	const maxDownload = 20 << 20
	if message.FileSize > maxDownload {
		return domain.File{}, domain.ErrInvalid
	}
	var file struct {
		Path string `json:"file_path"`
		Size int64  `json:"file_size"`
	}
	if e := t.call(ctx, token, "getFile", map[string]any{"file_id": message.FileID}, &file, false); e != nil {
		return domain.File{}, e
	}
	if file.Path == "" || file.Size > maxDownload {
		return domain.File{}, domain.ErrInvalid
	}
	parts := strings.Split(file.Path, "/")
	for i, part := range parts {
		if part == ".." || part == "." || part == "" {
			return domain.File{}, domain.ErrUnavailable
		}
		parts[i] = url.PathEscape(part)
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, telegramAPI+"file/bot"+token+"/"+strings.Join(parts, "/"), nil)
	if e != nil {
		return domain.File{}, domain.ErrUnavailable
	}
	response, e := t.client.Do(req)
	if e != nil {
		return domain.File{}, domain.ErrUnavailable
	}
	if response.StatusCode != http.StatusOK || response.ContentLength > maxDownload {
		response.Body.Close()
		return domain.File{}, domain.ErrUnavailable
	}
	size := file.Size
	if response.ContentLength >= 0 {
		size = response.ContentLength
	}
	mimeType := "application/octet-stream"
	if message.Kind == "photo" {
		mimeType = "image/jpeg"
	}
	return domain.File{Body: response.Body, MIMEType: mimeType, Name: path.Base(message.FileName), Size: size}, nil
}
