package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

type Service struct {
	store     domain.Store
	tg        domain.Telegram
	verifier  domain.Verifier
	publisher domain.Publisher
	origin    string
	// Serialize webhook changes with administrator writes to avoid lost notes,
	// stale grants, or approving an invitation after a completed revocation.
	updates sync.Mutex
}

func NewService(store domain.Store, tg domain.Telegram, verifier domain.Verifier, publisher domain.Publisher, origin string) *Service {
	return &Service{store: store, tg: tg, verifier: verifier, publisher: publisher, origin: strings.TrimRight(origin, "/")}
}

func (s *Service) Settings(ctx context.Context) (domain.Settings, error) {
	return s.store.GetSettings(ctx)
}

func (s *Service) SaveSettings(ctx context.Context, input domain.SaveSettings) (domain.Settings, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	if input.RetentionDays < 1 {
		return domain.Settings{}, domain.ErrInvalid
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	if input.Token != nil {
		token := strings.TrimSpace(*input.Token)
		if token == "" {
			return domain.Settings{}, domain.ErrInvalid
		}
		me, err := s.tg.Me(ctx, token)
		if err != nil {
			return domain.Settings{}, err
		}
		if settings.BotID != 0 && settings.BotID != me.ID {
			return domain.Settings{}, domain.ErrConflict
		}
		settings.Token, settings.BotID, settings.Username, settings.HasToken = token, me.ID, me.Username, true
	}
	settings.Welcome, settings.RetentionDays = input.Welcome, input.RetentionDays
	if err := s.store.PutSettings(ctx, settings); err != nil {
		return domain.Settings{}, err
	}
	return settings, nil
}

func (s *Service) configured(ctx context.Context) (domain.Settings, error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return settings, err
	}
	if settings.Token == "" {
		return settings, domain.ErrUnavailable
	}
	return settings, nil
}

func (s *Service) CheckBot(ctx context.Context) (domain.TelegramUser, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	settings, err := s.configured(ctx)
	if err != nil {
		return domain.TelegramUser{}, err
	}
	me, err := s.tg.Me(ctx, settings.Token)
	if err != nil {
		return me, err
	}
	if settings.BotID != 0 && settings.BotID != me.ID {
		return domain.TelegramUser{}, domain.ErrConflict
	}
	settings.BotID, settings.Username, settings.HasToken = me.ID, me.Username, true
	return me, s.store.PutSettings(ctx, settings)
}

func (s *Service) Webhook(ctx context.Context, register bool) error {
	settings, err := s.configured(ctx)
	if err != nil {
		return err
	}
	url := ""
	if register {
		url = s.origin + "/api/v1/telegram/webhook"
	}
	return s.tg.Webhook(ctx, settings.Token, url, settings.WebhookSecret, register)
}

func (s *Service) WebhookInfo(ctx context.Context) (domain.WebhookInfo, error) {
	settings, err := s.configured(ctx)
	if err != nil {
		return domain.WebhookInfo{}, err
	}
	return s.tg.WebhookInfo(ctx, settings.Token)
}

func (s *Service) Profiles(ctx context.Context) ([]domain.Profile, error) {
	return s.store.Profiles(ctx)
}

func (s *Service) SaveProfile(ctx context.Context, p domain.Profile) (domain.Profile, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, domain.ErrInvalid
	}
	if err := s.verifier.Validate(p.Request); err != nil {
		return p, err
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	} else if _, err := s.store.Profile(ctx, p.ID); err != nil {
		return p, err
	}
	ids := make([]int64, 0, len(p.ChatIDs))
	seen := make(map[int64]bool, len(p.ChatIDs))
	for _, id := range p.ChatIDs {
		if seen[id] {
			continue
		}
		if _, err := s.store.Chat(ctx, id); err != nil {
			return p, err
		}
		ids, seen[id] = append(ids, id), true
	}
	p.ChatIDs = ids
	return p, s.store.PutProfile(ctx, p)
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	s.updates.Lock()
	defer s.updates.Unlock()

	return s.store.DeleteProfile(ctx, id)
}
func (s *Service) TestProfile(ctx context.Context, p domain.Profile, input domain.VerificationInput) (domain.VerificationTestResult, error) {
	if input.Credential == "" {
		return domain.VerificationTestResult{}, domain.ErrInvalid
	}
	if err := s.verifier.Validate(p.Request); err != nil {
		return domain.VerificationTestResult{}, err
	}
	return s.verifier.Test(ctx, p.Request, input)
}

func (s *Service) Chats(ctx context.Context) ([]domain.Chat, error) { return s.store.Chats(ctx) }
func (s *Service) inspectChat(ctx context.Context, settings domain.Settings, id int64) (domain.Chat, error) {
	chat, err := s.tg.Chat(ctx, settings.Token, id)
	if err != nil {
		return domain.Chat{}, err
	}
	if chat.Type != "group" && chat.Type != "supergroup" && chat.Type != "channel" {
		return domain.Chat{}, domain.ErrInvalid
	}
	member, err := s.tg.Member(ctx, settings.Token, id, settings.BotID)
	if err != nil {
		return domain.Chat{}, err
	}
	return domain.Chat{ID: chat.ID, Type: chat.Type, Title: chat.Title, Username: chat.Username, BotStatus: member.Status, CanInvite: member.CanInvite, CanRestrict: member.CanRestrict, CanPost: member.CanPost}, nil
}

func (s *Service) AddChat(ctx context.Context, id int64) (domain.Chat, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	settings, err := s.configured(ctx)
	if err != nil {
		return domain.Chat{}, err
	}
	chat, err := s.inspectChat(ctx, settings, id)
	if err != nil {
		return chat, err
	}
	if chat.BotStatus != "administrator" || !chat.CanInvite {
		return chat, domain.ErrForbidden
	}
	chat.Enabled = true
	if old, err := s.store.Chat(ctx, id); err == nil {
		chat.Enabled = old.Enabled
	} else if !errors.Is(err, domain.ErrNotFound) {
		return chat, err
	}
	return chat, s.store.PutChat(ctx, chat)
}

func (s *Service) SaveChat(ctx context.Context, id int64, enabled bool) (domain.Chat, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	chat, err := s.store.Chat(ctx, id)
	if err != nil {
		return chat, err
	}
	if enabled && (chat.BotStatus != "administrator" || !chat.CanInvite) {
		return chat, domain.ErrForbidden
	}
	chat.Enabled = enabled
	return chat, s.store.PutChat(ctx, chat)
}

func (s *Service) CheckChat(ctx context.Context, id int64) (domain.Chat, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	old, err := s.store.Chat(ctx, id)
	if err != nil {
		return old, err
	}
	settings, err := s.configured(ctx)
	if err != nil {
		return old, err
	}
	chat, err := s.inspectChat(ctx, settings, id)
	if err != nil {
		return chat, err
	}
	chat.Enabled = old.Enabled
	return chat, s.store.PutChat(ctx, chat)
}

func (s *Service) Users(ctx context.Context, q, cursor string) (domain.UserPage, error) {
	return s.store.Users(ctx, q, cursor, 50)
}
func (s *Service) UserDetail(ctx context.Context, id int64) (domain.UserDetail, error) {
	u, err := s.store.User(ctx, id)
	if err != nil {
		return domain.UserDetail{}, err
	}
	grants, err := s.store.Grants(ctx, id)
	if err != nil {
		return domain.UserDetail{}, err
	}
	members, err := s.store.Members(ctx, 0, id)
	return domain.UserDetail{User: u, Grants: grants, Members: members}, err
}

func (s *Service) SaveNote(ctx context.Context, id int64, note string) (domain.User, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	u, err := s.store.User(ctx, id)
	if err != nil {
		return u, err
	}
	u.Note = note
	return u, s.store.PutUser(ctx, u)
}

func (s *Service) Revoke(ctx context.Context, userID int64, profileID string) error {
	s.updates.Lock()
	defer s.updates.Unlock()

	return s.store.Transaction(ctx, func(store domain.Store) error {
		grants, err := store.Grants(ctx, userID)
		if err != nil {
			return err
		}
		for _, grant := range grants {
			if grant.ProfileID == profileID {
				grant.Active = false
				return store.PutGrant(ctx, grant)
			}
		}
		return domain.ErrNotFound
	})
}

func (s *Service) Members(ctx context.Context, chatID int64) ([]domain.Member, error) {
	return s.store.Members(ctx, chatID, 0)
}
func (s *Service) Invites(ctx context.Context, chatID int64) ([]domain.Invite, error) {
	return s.store.Invites(ctx, chatID, 0)
}

func (s *Service) Act(ctx context.Context, action domain.MemberAction) (domain.Member, error) {
	s.updates.Lock()
	defer s.updates.Unlock()

	chat, err := s.store.Chat(ctx, action.ChatID)
	if err != nil {
		return domain.Member{}, err
	}
	switch action.Action {
	case "ban", "unban", "kick", "mute", "unmute":
		if chat.BotStatus != "administrator" || !chat.CanRestrict {
			return domain.Member{}, domain.ErrForbidden
		}
		if (action.Action == "mute" || action.Action == "unmute") && chat.Type != "supergroup" {
			return domain.Member{}, domain.ErrInvalid
		}
		if (action.Action == "kick" || action.Action == "unban") && chat.Type == "group" {
			return domain.Member{}, domain.ErrInvalid
		}
	case "refresh":
	default:
		return domain.Member{}, domain.ErrInvalid
	}
	if action.UntilDate != 0 && (action.Action == "ban" || action.Action == "mute") {
		now := time.Now().Unix()
		if action.UntilDate < now+30 || action.UntilDate > now+366*86400 {
			return domain.Member{}, domain.ErrInvalid
		}
	}
	settings, err := s.configured(ctx)
	if err != nil {
		return domain.Member{}, err
	}
	if action.Action != "refresh" {
		if err := s.tg.Act(ctx, settings.Token, action); err != nil {
			return domain.Member{}, err
		}
	}
	member, err := s.tg.Member(ctx, settings.Token, action.ChatID, action.UserID)
	if err != nil {
		if action.Action != "refresh" {
			return domain.Member{}, domain.ErrUnknown
		}
		return domain.Member{}, err
	}
	result := domain.Member{ChatID: action.ChatID, UserID: action.UserID, Status: member.Status, UntilDate: member.UntilDate, UpdatedAt: time.Now()}
	if err := s.store.PutMember(ctx, result); err != nil {
		if action.Action != "refresh" {
			return result, domain.ErrUnknown
		}
		return result, err
	}
	return result, nil
}

func (s *Service) Messages(ctx context.Context, chatID int64, cursor string) (domain.MessagePage, error) {
	return s.store.Messages(ctx, chatID, cursor, 50)
}

func (s *Service) Send(ctx context.Context, input domain.SendMessage) (domain.Message, error) {
	switch input.Kind {
	case "text":
		if strings.TrimSpace(input.Text) == "" || len([]rune(input.Text)) > 4096 {
			return domain.Message{}, domain.ErrInvalid
		}
	case "photo", "document":
		if input.File == nil || input.Size <= 0 || len([]rune(input.Text)) > 1024 {
			return domain.Message{}, domain.ErrInvalid
		}
		limit := int64(50 << 20)
		if input.Kind == "photo" {
			limit = 10 << 20
		}
		if input.Size > limit {
			return domain.Message{}, domain.ErrInvalid
		}
	default:
		return domain.Message{}, domain.ErrInvalid
	}
	chat, err := s.store.Chat(ctx, input.ChatID)
	if err == nil {
		if !chat.Enabled || (chat.Type == "channel" && !chat.CanPost) {
			return domain.Message{}, domain.ErrForbidden
		}
	} else if errors.Is(err, domain.ErrNotFound) {
		u, userErr := s.store.User(ctx, input.ChatID)
		if userErr != nil {
			return domain.Message{}, userErr
		}
		if !u.PrivateChat {
			return domain.Message{}, domain.ErrForbidden
		}
	} else {
		return domain.Message{}, err
	}
	settings, err := s.configured(ctx)
	if err != nil {
		return domain.Message{}, err
	}
	sent, sendErr := s.tg.Send(ctx, settings.Token, input)
	message := domain.Message{ID: uuid.NewString(), ChatID: input.ChatID, Kind: input.Kind, Text: input.Text, Direction: "out", Status: "sent", ReplyToMessageID: input.ReplyToMessageID, Date: time.Now(), FileName: input.FileName, MIMEType: input.MIMEType, FileSize: input.Size}
	if sendErr == nil {
		message.TelegramMessageID = sent.MessageID
		if sent.Date != 0 {
			message.Date = time.Unix(sent.Date, 0)
		}
		if sent.Document != nil {
			message.FileID = sent.Document.FileID
			message.FileSize = sent.Document.FileSize
		}
		if len(sent.Photo) > 0 {
			photo := sent.Photo[len(sent.Photo)-1]
			message.FileID, message.FileSize = photo.FileID, photo.FileSize
		}
	} else if errors.Is(sendErr, domain.ErrUnknown) {
		message.Status = "unknown"
	} else {
		message.Status = "failed"
	}
	if err := s.store.PutMessage(ctx, message); err != nil {
		if sendErr == nil || errors.Is(sendErr, domain.ErrUnknown) {
			message.Status = "unknown"
			return message, domain.ErrUnknown
		}
		return message, err
	}
	if sendErr == nil {
		// 频道回显可能已先插入同一Telegram消息，返回UPSERT保留的真实记录ID。
		canonical, err := s.store.MessageByTelegram(ctx, message.ChatID, message.TelegramMessageID)
		if err != nil {
			message.Status = "unknown"
			return message, domain.ErrUnknown
		}
		return canonical, nil
	}
	return message, sendErr
}

func (s *Service) Download(ctx context.Context, id string) (domain.File, error) {
	message, err := s.store.Message(ctx, id)
	if err != nil {
		return domain.File{}, err
	}
	settings, err := s.configured(ctx)
	if err != nil {
		return domain.File{}, err
	}
	if message.FileID == "" || message.Sensitive {
		return domain.File{}, domain.ErrNotFound
	}
	if message.FileSize > 20<<20 {
		return domain.File{}, domain.ErrInvalid
	}
	if message.Date.Before(time.Now().AddDate(0, 0, -settings.RetentionDays)) {
		return domain.File{}, domain.ErrNotFound
	}
	return s.tg.Download(ctx, settings.Token, message)
}

func (s *Service) DeliverReply(ctx context.Context, reply domain.Reply) error {
	settings, err := s.configured(ctx)
	if err != nil {
		return err
	}
	sent, err := s.tg.Reply(ctx, settings.Token, reply)
	if err != nil {
		return err
	}
	message := domain.Message{ID: uuid.NewString(), ChatID: reply.ChatID, TelegramMessageID: sent.MessageID, Kind: "text", Text: reply.Text, Direction: "out", Status: "sent", Date: time.Unix(sent.Date, 0)}
	return s.store.PutMessage(ctx, message)
}

func (s *Service) Cleanup(ctx context.Context) error {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	// Telegram保留更新不超过24小时；收据独立保留7天，不跟随消息保留设置。
	now := time.Now()
	return s.store.Cleanup(ctx, now.AddDate(0, 0, -settings.RetentionDays), now.AddDate(0, 0, -7))
}
