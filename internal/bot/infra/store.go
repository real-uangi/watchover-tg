package infra

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/real-uangi/allingo/common/db"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type settingsRecord struct {
	ID         int    `gorm:"primaryKey"`
	Ciphertext []byte `gorm:"type:bytea;not null"`
}

func (settingsRecord) TableName() string { return "bot_settings" }

type profileRecord struct {
	ID      string `gorm:"type:uuid;primaryKey"`
	Data    string `gorm:"type:jsonb;not null"`
	Request []byte `gorm:"type:bytea;not null"`
}

func (profileRecord) TableName() string { return "bot_profiles" }

type chatRecord struct {
	ID   int64  `gorm:"primaryKey;autoIncrement:false"`
	Data string `gorm:"type:jsonb;not null"`
}

func (chatRecord) TableName() string { return "bot_chats" }

type userRecord struct {
	ID        int64 `gorm:"primaryKey;autoIncrement:false"`
	Search    string
	UpdatedAt time.Time `gorm:"index"`
	Data      string    `gorm:"type:jsonb;not null"`
}

func (userRecord) TableName() string { return "bot_users" }

type grantRecord struct {
	UserID    int64  `gorm:"primaryKey;autoIncrement:false"`
	ProfileID string `gorm:"type:uuid;primaryKey"`
	Data      string `gorm:"type:jsonb;not null"`
}

func (grantRecord) TableName() string { return "bot_grants" }

type memberRecord struct {
	ChatID       int64 `gorm:"primaryKey;autoIncrement:false"`
	UserID       int64 `gorm:"primaryKey;autoIncrement:false;index"`
	LastUpdateID int64
	UpdatedAt    time.Time `gorm:"autoUpdateTime:false"`
	Data         string    `gorm:"type:jsonb;not null"`
}

func (memberRecord) TableName() string { return "bot_members" }

type conversationRecord struct {
	UserID    int64     `gorm:"primaryKey;autoIncrement:false"`
	ProfileID string    `gorm:"type:uuid;not null"`
	ExpiresAt time.Time `gorm:"index"`
}

func (conversationRecord) TableName() string { return "bot_conversations" }

type inviteRecord struct {
	ID     string `gorm:"type:uuid;primaryKey"`
	ChatID int64  `gorm:"index"`
	UserID int64  `gorm:"index"`
	Link   string `gorm:"uniqueIndex;not null"`
	Data   string `gorm:"type:jsonb;not null"`
}

func (inviteRecord) TableName() string { return "bot_invites" }

type messageRecord struct {
	ID                string `gorm:"type:uuid;primaryKey"`
	ChatID            int64  `gorm:"uniqueIndex:bot_message_telegram,where:telegram_message_id <> 0;index:bot_message_page,priority:1"`
	TelegramMessageID int64  `gorm:"uniqueIndex:bot_message_telegram,where:telegram_message_id <> 0"`
	SenderID          int64
	SenderName        string
	Kind              string
	Text              string
	FileID            string
	FileName          string
	MIMEType          string
	FileSize          int64
	Direction         string
	Status            string
	ReplyToMessageID  int64
	Date              time.Time `gorm:"index:bot_message_page,priority:2;index"`
	EditedAt          *time.Time
	Sensitive         bool
	LastUpdateID      int64
}

func (messageRecord) TableName() string { return "bot_messages" }

type receiptRecord struct {
	ID   int64     `gorm:"primaryKey;autoIncrement:false"`
	Date time.Time `gorm:"index"`
}

func (receiptRecord) TableName() string { return "bot_receipts" }

type Store struct {
	db            *gorm.DB
	cipher        *Cipher
	inTransaction bool
}

var _ domain.Store = (*Store)(nil)

func NewStore(manager *db.Manager, cipher *Cipher) *Store {
	return &Store{db: manager.GetDB(), cipher: cipher}
}
func (s *Store) Transaction(ctx context.Context, f func(domain.Store) error) error {
	return storageError(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return f(&Store{db: tx, cipher: s.cipher, inTransaction: true}) }))
}
func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	for _, e := range []error{domain.ErrInvalid, domain.ErrForbidden, domain.ErrNotFound, domain.ErrConflict, domain.ErrUnavailable, domain.ErrUnknown} {
		if errors.Is(err, e) {
			return e
		}
	}
	type sqlState interface{ SQLState() string }
	var state sqlState
	if errors.As(err, &state) && (state.SQLState() == "23505" || state.SQLState() == "23503") {
		return domain.ErrConflict
	}
	return domain.ErrUnavailable
}
func encode(v any) (string, error)    { b, e := json.Marshal(v); return string(b), storageError(e) }
func decode(data string, v any) error { return storageError(json.Unmarshal([]byte(data), v)) }
func (s *Store) GetSettings(ctx context.Context) (domain.Settings, error) {
	var r settingsRecord
	q := s.db.WithContext(ctx)
	if s.inTransaction {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := q.First(&r, 1).Error; e != nil {
		return domain.Settings{}, storageError(e)
	}
	b, e := s.cipher.decrypt("settings", r.Ciphertext)
	if e != nil {
		return domain.Settings{}, e
	}
	var v struct {
		domain.Settings
		Token         string
		WebhookSecret string
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return domain.Settings{}, domain.ErrUnavailable
	}
	v.Settings.Token = v.Token
	v.Settings.WebhookSecret = v.WebhookSecret
	v.Settings.HasToken = v.Token != ""
	return v.Settings, nil
}
func (s *Store) PutSettings(ctx context.Context, v domain.Settings) error {
	data, e := json.Marshal(struct {
		domain.Settings
		Token         string
		WebhookSecret string
	}{v, v.Token, v.WebhookSecret})
	if e != nil {
		return domain.ErrUnavailable
	}
	encrypted, e := s.cipher.encrypt("settings", data)
	if e != nil {
		return e
	}
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&settingsRecord{ID: 1, Ciphertext: encrypted}).Error)
}
func (s *Store) profile(r profileRecord) (domain.Profile, error) {
	var p domain.Profile
	if e := decode(r.Data, &p); e != nil {
		return p, e
	}
	data, e := s.cipher.decrypt("profile:"+r.ID, r.Request)
	if e != nil {
		return p, e
	}
	return p, storageError(json.Unmarshal(data, &p.Request))
}
func (s *Store) Profiles(ctx context.Context) ([]domain.Profile, error) {
	var rows []profileRecord
	if e := s.db.WithContext(ctx).Order("id").Find(&rows).Error; e != nil {
		return nil, storageError(e)
	}
	out := make([]domain.Profile, 0, len(rows))
	for _, r := range rows {
		p, e := s.profile(r)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, nil
}
func (s *Store) Profile(ctx context.Context, id string) (domain.Profile, error) {
	var r profileRecord
	if e := s.db.WithContext(ctx).First(&r, "id = ?", id).Error; e != nil {
		return domain.Profile{}, storageError(e)
	}
	return s.profile(r)
}
func (s *Store) PutProfile(ctx context.Context, v domain.Profile) error {
	request, e := json.Marshal(v.Request)
	if e != nil {
		return domain.ErrInvalid
	}
	encrypted, e := s.cipher.encrypt("profile:"+v.ID, request)
	if e != nil {
		return e
	}
	v.Request = domain.RequestConfig{}
	data, e := encode(v)
	if e != nil {
		return e
	}
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&profileRecord{ID: v.ID, Data: data, Request: encrypted}).Error)
}
func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	return storageError(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&grantRecord{}, &conversationRecord{}} {
			if e := tx.Where("profile_id = ?", id).Delete(model).Error; e != nil {
				return e
			}
		}
		if e := tx.Where("data->>'profileId' = ?", id).Delete(&inviteRecord{}).Error; e != nil {
			return e
		}
		r := tx.Delete(&profileRecord{}, "id = ?", id)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		return nil
	}))
}
func (s *Store) Chats(ctx context.Context) ([]domain.Chat, error) {
	var rows []chatRecord
	if e := s.db.WithContext(ctx).Order("id").Find(&rows).Error; e != nil {
		return nil, storageError(e)
	}
	out := make([]domain.Chat, 0, len(rows))
	for _, r := range rows {
		var v domain.Chat
		if e := decode(r.Data, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) Chat(ctx context.Context, id int64) (domain.Chat, error) {
	var r chatRecord
	if e := s.db.WithContext(ctx).First(&r, "id = ?", id).Error; e != nil {
		return domain.Chat{}, storageError(e)
	}
	var v domain.Chat
	e := decode(r.Data, &v)
	return v, e
}
func (s *Store) PutChat(ctx context.Context, v domain.Chat) error {
	data, e := encode(v)
	if e != nil {
		return e
	}
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&chatRecord{ID: v.ID, Data: data}).Error)
}
func (s *Store) Users(ctx context.Context, query, cursor string, limit int) (domain.UserPage, error) {
	q := s.db.WithContext(ctx).Order("id DESC")
	if query != "" {
		q = q.Where("search ILIKE ?", "%"+query+"%")
	}
	if cursor != "" {
		id, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil {
			return domain.UserPage{}, domain.ErrInvalid
		}
		q = q.Where("id < ?", id)
	}
	var rows []userRecord
	if e := q.Limit(limit + 1).Find(&rows).Error; e != nil {
		return domain.UserPage{}, storageError(e)
	}
	page := domain.UserPage{Items: make([]domain.User, 0, limit)}
	if len(rows) > limit {
		page.NextCursor = strconv.FormatInt(rows[limit-1].ID, 10)
		rows = rows[:limit]
	}
	for _, r := range rows {
		var v domain.User
		if e := decode(r.Data, &v); e != nil {
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	return page, nil
}
func (s *Store) User(ctx context.Context, id int64) (domain.User, error) {
	var r userRecord
	if e := s.db.WithContext(ctx).First(&r, "id = ?", id).Error; e != nil {
		return domain.User{}, storageError(e)
	}
	var v domain.User
	e := decode(r.Data, &v)
	return v, e
}
func (s *Store) PutUser(ctx context.Context, v domain.User) error {
	data, e := encode(v)
	if e != nil {
		return e
	}
	search := fmt.Sprintf("%d %s %s %s %s", v.ID, v.Username, v.FirstName, v.LastName, v.Note)
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&userRecord{ID: v.ID, Search: search, Data: data, UpdatedAt: v.UpdatedAt}).Error)
}
func (s *Store) Grants(ctx context.Context, userID int64) ([]domain.Grant, error) {
	var rows []grantRecord
	if e := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("profile_id").Find(&rows).Error; e != nil {
		return nil, storageError(e)
	}
	out := make([]domain.Grant, 0, len(rows))
	for _, r := range rows {
		var v domain.Grant
		if e := decode(r.Data, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) PutGrant(ctx context.Context, v domain.Grant) error {
	data, e := encode(v)
	if e != nil {
		return e
	}
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&grantRecord{UserID: v.UserID, ProfileID: v.ProfileID, Data: data}).Error)
}
func (s *Store) Members(ctx context.Context, chatID, userID int64) ([]domain.Member, error) {
	q := s.db.WithContext(ctx)
	if chatID != 0 {
		q = q.Where("chat_id = ?", chatID)
	}
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	var rows []memberRecord
	if e := q.Order("chat_id,user_id").Find(&rows).Error; e != nil {
		return nil, storageError(e)
	}
	out := make([]domain.Member, 0, len(rows))
	for _, r := range rows {
		var v domain.Member
		if e := decode(r.Data, &v); e != nil {
			return nil, e
		}
		v.LastUpdateID = r.LastUpdateID
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) PutMember(ctx context.Context, v domain.Member) error {
	data, e := encode(v)
	if e != nil {
		return e
	}
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "chat_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"data", "last_update_id", "updated_at"}), Where: clause.Where{Exprs: []clause.Expression{gorm.Expr("EXCLUDED.updated_at > bot_members.updated_at OR (EXCLUDED.updated_at = bot_members.updated_at AND EXCLUDED.last_update_id >= bot_members.last_update_id)")}}}).Create(&memberRecord{ChatID: v.ChatID, UserID: v.UserID, Data: data, LastUpdateID: v.LastUpdateID, UpdatedAt: v.UpdatedAt}).Error)
}
func (s *Store) Conversation(ctx context.Context, userID int64) (domain.Conversation, error) {
	var r conversationRecord
	e := s.db.WithContext(ctx).First(&r, "user_id = ?", userID).Error
	return domain.Conversation{UserID: r.UserID, ProfileID: r.ProfileID, ExpiresAt: r.ExpiresAt}, storageError(e)
}
func (s *Store) PutConversation(ctx context.Context, v domain.Conversation) error {
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&conversationRecord{UserID: v.UserID, ProfileID: v.ProfileID, ExpiresAt: v.ExpiresAt}).Error)
}
func (s *Store) DeleteConversation(ctx context.Context, id int64) error {
	return storageError(s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&conversationRecord{}).Error)
}
func (s *Store) Invites(ctx context.Context, chatID, userID int64) ([]domain.Invite, error) {
	q := s.db.WithContext(ctx)
	if chatID != 0 {
		q = q.Where("chat_id = ?", chatID)
	}
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	var rows []inviteRecord
	if e := q.Order("id DESC").Find(&rows).Error; e != nil {
		return nil, storageError(e)
	}
	out := make([]domain.Invite, 0, len(rows))
	for _, r := range rows {
		var v domain.Invite
		if e := decode(r.Data, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) InviteByLink(ctx context.Context, link string) (domain.Invite, error) {
	var r inviteRecord
	if e := s.db.WithContext(ctx).First(&r, "link = ?", link).Error; e != nil {
		return domain.Invite{}, storageError(e)
	}
	var v domain.Invite
	e := decode(r.Data, &v)
	return v, e
}
func (s *Store) PutInvite(ctx context.Context, v domain.Invite) error {
	data, e := encode(v)
	if e != nil {
		return e
	}
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&inviteRecord{ID: v.ID, ChatID: v.ChatID, UserID: v.UserID, Link: v.Link, Data: data}).Error)
}
func fromMessage(v domain.Message) messageRecord {
	return messageRecord{ID: v.ID, ChatID: v.ChatID, TelegramMessageID: v.TelegramMessageID, SenderID: v.SenderID, SenderName: v.SenderName, Kind: v.Kind, Text: v.Text, FileID: v.FileID, FileName: v.FileName, MIMEType: v.MIMEType, FileSize: v.FileSize, Direction: v.Direction, Status: v.Status, ReplyToMessageID: v.ReplyToMessageID, Date: v.Date, EditedAt: v.EditedAt, Sensitive: v.Sensitive, LastUpdateID: v.LastUpdateID}
}
func (r messageRecord) message() domain.Message {
	return domain.Message{ID: r.ID, ChatID: r.ChatID, TelegramMessageID: r.TelegramMessageID, SenderID: r.SenderID, SenderName: r.SenderName, Kind: r.Kind, Text: r.Text, FileID: r.FileID, FileName: r.FileName, MIMEType: r.MIMEType, FileSize: r.FileSize, Direction: r.Direction, Status: r.Status, ReplyToMessageID: r.ReplyToMessageID, Date: r.Date, EditedAt: r.EditedAt, Sensitive: r.Sensitive, LastUpdateID: r.LastUpdateID}
}
func (s *Store) Messages(ctx context.Context, chatID int64, cursor string, limit int) (domain.MessagePage, error) {
	q := s.db.WithContext(ctx).Where("chat_id = ?", chatID).Order("date DESC,id DESC")
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil {
			return domain.MessagePage{}, domain.ErrInvalid
		}
		parts := strings.SplitN(string(raw), "|", 2)
		if len(parts) != 2 {
			return domain.MessagePage{}, domain.ErrInvalid
		}
		date, e := time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return domain.MessagePage{}, domain.ErrInvalid
		}
		q = q.Where("date < ? OR (date = ? AND id < ?)", date, date, parts[1])
	}
	var rows []messageRecord
	if e := q.Limit(limit + 1).Find(&rows).Error; e != nil {
		return domain.MessagePage{}, storageError(e)
	}
	page := domain.MessagePage{Items: make([]domain.Message, 0, limit)}
	if len(rows) > limit {
		last := rows[limit-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.Date.UTC().Format(time.RFC3339Nano) + "|" + last.ID))
		rows = rows[:limit]
	}
	for _, r := range rows {
		page.Items = append(page.Items, r.message())
	}
	return page, nil
}
func (s *Store) Message(ctx context.Context, id string) (domain.Message, error) {
	var r messageRecord
	e := s.db.WithContext(ctx).First(&r, "id = ?", id).Error
	return r.message(), storageError(e)
}
func (s *Store) MessageByTelegram(ctx context.Context, chatID, id int64) (domain.Message, error) {
	var r messageRecord
	e := s.db.WithContext(ctx).First(&r, "chat_id = ? AND telegram_message_id = ?", chatID, id).Error
	return r.message(), storageError(e)
}
func (s *Store) PutMessage(ctx context.Context, v domain.Message) error {
	r := fromMessage(v)
	if r.Sensitive {
		r.Text = ""
		r.FileID = ""
		r.FileName = ""
		r.MIMEType = ""
		r.FileSize = 0
	}
	if r.TelegramMessageID == 0 {
		return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, UpdateAll: true}).Create(&r).Error)
	}
	// A later nonsensitive edit must never restore a credential-bearing message.
	newer := "COALESCE(EXCLUDED.edited_at, EXCLUDED.date) > COALESCE(bot_messages.edited_at, bot_messages.date) OR (COALESCE(EXCLUDED.edited_at, EXCLUDED.date) = COALESCE(bot_messages.edited_at, bot_messages.date) AND EXCLUDED.last_update_id >= bot_messages.last_update_id)"
	updates := map[string]any{"sensitive": gorm.Expr("bot_messages.sensitive OR EXCLUDED.sensitive"), "last_update_id": gorm.Expr("CASE WHEN " + newer + " THEN EXCLUDED.last_update_id ELSE bot_messages.last_update_id END"), "direction": gorm.Expr("CASE WHEN bot_messages.direction = 'out' OR EXCLUDED.direction = 'out' THEN 'out' ELSE EXCLUDED.direction END"), "status": gorm.Expr("CASE WHEN bot_messages.direction = 'out' AND EXCLUDED.direction <> 'out' THEN bot_messages.status ELSE EXCLUDED.status END")}
	for _, col := range []string{"sender_id", "sender_name", "kind", "reply_to_message_id", "edited_at"} {
		updates[col] = gorm.Expr("CASE WHEN " + newer + " THEN EXCLUDED." + col + " ELSE bot_messages." + col + " END")
	}
	for _, col := range []string{"text", "file_id", "file_name", "mime_type"} {
		updates[col] = gorm.Expr("CASE WHEN bot_messages.sensitive OR EXCLUDED.sensitive THEN '' WHEN " + newer + " THEN EXCLUDED." + col + " ELSE bot_messages." + col + " END")
	}
	updates["file_size"] = gorm.Expr("CASE WHEN bot_messages.sensitive OR EXCLUDED.sensitive THEN 0 WHEN " + newer + " THEN EXCLUDED.file_size ELSE bot_messages.file_size END")
	return storageError(s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "chat_id"}, {Name: "telegram_message_id"}}, TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("telegram_message_id <> 0")}}, DoUpdates: clause.Assignments(updates)}).Create(&r).Error)
}
func (s *Store) HasReceipt(ctx context.Context, id int64) (bool, error) {
	var n int64
	e := s.db.WithContext(ctx).Model(&receiptRecord{}).Where("id = ?", id).Count(&n).Error
	return n > 0, storageError(e)
}
func (s *Store) PutReceipt(ctx context.Context, id int64, date time.Time) error {
	return storageError(s.db.WithContext(ctx).Create(&receiptRecord{ID: id, Date: date}).Error)
}
func (s *Store) Cleanup(ctx context.Context, messageBefore, receiptBefore time.Time) error {
	return storageError(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("date < ?", messageBefore).Delete(&messageRecord{}).Error; e != nil {
			return e
		}
		if e := tx.Where("date < ?", receiptBefore).Delete(&receiptRecord{}).Error; e != nil {
			return e
		}
		return tx.Where("expires_at <= ?", time.Now()).Delete(&conversationRecord{}).Error
	}))
}
