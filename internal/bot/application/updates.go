package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

const sensitiveText = "[核验凭据已隐藏]"

func (s *Service) ReceiveUpdate(ctx context.Context, secret string, update domain.Update) error {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(settings.WebhookSecret)) != 1 {
		return domain.ErrForbidden
	}
	return s.publisher.PublishUpdate(update)
}

func (s *Service) HandleUpdate(ctx context.Context, update domain.Update) error {
	// 接收端可能重复入队；单实例串行查重、处理并提交完成收据。
	s.updates.Lock()
	defer s.updates.Unlock()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	done, err := s.store.HasReceipt(ctx, update.ID)
	if err != nil || done {
		return err
	}
	var replies []domain.Reply
	err = s.store.Transaction(ctx, func(store domain.Store) error {
		var err error
		switch {
		case update.Message != nil:
			err = s.messageUpdate(ctx, store, settings, update.ID, *update.Message, false, &replies)
		case update.EditedMessage != nil:
			err = s.messageUpdate(ctx, store, settings, update.ID, *update.EditedMessage, true, &replies)
		case update.ChannelPost != nil:
			err = s.messageUpdate(ctx, store, settings, update.ID, *update.ChannelPost, false, &replies)
		case update.EditedChannelPost != nil:
			err = s.messageUpdate(ctx, store, settings, update.ID, *update.EditedChannelPost, true, &replies)
		case update.Callback != nil:
			err = s.callbackUpdate(ctx, store, settings, *update.Callback, &replies)
		case update.MyMember != nil:
			err = s.botMemberUpdate(ctx, store, *update.MyMember)
		case update.Member != nil:
			err = s.memberUpdate(ctx, store, update.ID, *update.Member)
		case update.Join != nil:
			err = s.joinUpdate(ctx, store, settings, update.ID, *update.Join, &replies)
		}
		if err != nil {
			return err
		}
		return store.PutReceipt(ctx, update.ID, time.Now())
	})
	if err != nil {
		return err
	}
	// 业务与收据已提交，提示失败不改变资格，也不借Webhook重投补偿内存事件。
	for _, reply := range replies {
		if err := s.publisher.Publish(reply); err != nil {
			slog.Warn("bot reply could not be queued", "chat_id", reply.ChatID)
		}
	}
	if update.Callback != nil {
		if err := s.tg.AnswerCallback(ctx, settings.Token, update.Callback.ID); err != nil {
			slog.Warn("bot callback acknowledgement failed")
		}
	}
	return nil
}

func updateUser(ctx context.Context, store domain.Store, from domain.TelegramUser, private bool) error {
	u, err := store.User(ctx, from.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	u.ID, u.Username, u.FirstName, u.LastName = from.ID, from.Username, from.FirstName, from.LastName
	u.PrivateChat, u.UpdatedAt = u.PrivateChat || private, time.Now()
	return store.PutUser(ctx, u)
}

func updateChat(ctx context.Context, store domain.Store, source domain.TelegramChat) (domain.Chat, error) {
	chat, err := store.Chat(ctx, source.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return chat, err
	}
	chat.ID, chat.Type, chat.Title, chat.Username = source.ID, source.Type, source.Title, source.Username
	return chat, store.PutChat(ctx, chat)
}

func (s *Service) messageUpdate(ctx context.Context, store domain.Store, settings domain.Settings, updateID int64, input domain.TelegramMessage, edited bool, replies *[]domain.Reply) error {
	old, err := store.MessageByTelegram(ctx, input.Chat.ID, input.MessageID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if edited && errors.Is(err, domain.ErrNotFound) {
		return nil
	} // 不复活已过保留期的消息或丢失其敏感标记。
	if err == nil {
		incomingAt := time.Unix(input.Date, 0)
		if input.EditDate != 0 {
			incomingAt = time.Unix(input.EditDate, 0)
		}
		previousAt := old.Date
		if old.EditedAt != nil {
			previousAt = *old.EditedAt
		}
		if old.Sensitive || incomingAt.Before(previousAt) || (incomingAt.Equal(previousAt) && old.LastUpdateID >= updateID) {
			return nil
		}
	}
	private := input.Chat.Type == "private"
	if input.From != nil && input.SenderChat == nil && !input.From.IsBot {
		if err := updateUser(ctx, store, *input.From, private); err != nil {
			return err
		}
	}
	if !private {
		if _, err := updateChat(ctx, store, input.Chat); err != nil {
			return err
		}
	}
	message := domain.Message{ID: old.ID, ChatID: input.Chat.ID, TelegramMessageID: input.MessageID, Text: input.Text, Kind: "text", Direction: "in", Status: "received", Date: time.Unix(input.Date, 0), LastUpdateID: updateID}
	if old.Direction == "out" {
		message.Direction, message.Status = "out", old.Status
	}
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if input.From != nil && input.SenderChat == nil {
		message.SenderID, message.SenderName = input.From.ID, strings.TrimSpace(input.From.FirstName+" "+input.From.LastName)
	}
	if input.SenderChat != nil {
		message.SenderName = input.SenderChat.Title
	}
	if input.ReplyTo != nil {
		message.ReplyToMessageID = input.ReplyTo.MessageID
	}
	if input.EditDate != 0 {
		at := time.Unix(input.EditDate, 0)
		message.EditedAt = &at
	}
	if len(input.Photo) > 0 {
		photo := input.Photo[len(input.Photo)-1]
		message.Kind, message.Text, message.FileID, message.FileSize, message.MIMEType, message.FileName = "photo", input.Caption, photo.FileID, photo.FileSize, "image/jpeg", "photo.jpg"
	} else if input.Document != nil {
		doc := input.Document
		message.Kind, message.Text, message.FileID, message.FileName, message.MIMEType, message.FileSize = "document", input.Caption, doc.FileID, doc.FileName, doc.MIMEType, doc.FileSize
	} else if input.Text == "" {
		switch {
		case input.Video != nil:
			message.Kind = "video"
		case input.Voice != nil:
			message.Kind = "voice"
		case input.Audio != nil:
			message.Kind = "audio"
		case input.Animation != nil:
			message.Kind = "animation"
		case input.Sticker != nil:
			message.Kind = "sticker"
		case input.VideoNote != nil:
			message.Kind = "video_note"
		default:
			message.Kind = "other"
		}
		message.Text = input.Caption
	}
	if private && edited && input.From != nil {
		conversation, err := store.Conversation(ctx, input.From.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if err == nil && conversation.ExpiresAt.After(time.Now()) {
			// 等待凭据时用户也可能编辑旧普通消息；只隐藏，不把编辑当作一次核验。
			message.Sensitive, message.Text, message.FileID, message.FileName, message.MIMEType, message.FileSize = true, sensitiveText, "", "", "", 0
		}
	}
	if private && !edited && input.From != nil && !input.From.IsBot {
		if err := s.privateMessage(ctx, store, settings, input, &message, replies); err != nil {
			return err
		}
	}
	return store.PutMessage(ctx, message)
}

func (s *Service) privateMessage(ctx context.Context, store domain.Store, settings domain.Settings, input domain.TelegramMessage, message *domain.Message, replies *[]domain.Reply) error {
	command := strings.SplitN(input.Text, " ", 2)[0]
	command = strings.SplitN(command, "@", 2)[0]
	switch command {
	case "/start":
		if err := store.DeleteConversation(ctx, input.From.ID); err != nil {
			return err
		}
		if settings.Welcome != "" {
			*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: settings.Welcome})
		}
		return s.menu(ctx, store, settings, input.From.ID, replies)
	case "/cancel":
		if err := store.DeleteConversation(ctx, input.From.ID); err != nil {
			return err
		}
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "已取消验证。发送 /start 重新选择。"})
		return nil
	case "/help":
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "发送 /start 选择验证方式，验证后选择可加入的群组或频道。发送 /cancel 取消当前验证。"})
		return nil
	}
	conversation, err := store.Conversation(ctx, input.From.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// 在验证流程内输入的文本或附件先永久标为敏感；凭据仅存在于内存入站事件与核验调用中。
	message.Sensitive, message.Text, message.FileID, message.FileName, message.MIMEType, message.FileSize = true, sensitiveText, "", "", "", 0
	if !conversation.ExpiresAt.After(time.Now()) {
		if err := store.DeleteConversation(ctx, input.From.ID); err != nil {
			return err
		}
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "验证已超时，请发送 /start 重新选择。"})
		return nil
	}
	profile, err := store.Profile(ctx, conversation.ProfileID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if errors.Is(err, domain.ErrNotFound) || !profile.Enabled {
		if err := store.DeleteConversation(ctx, input.From.ID); err != nil {
			return err
		}
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "验证方式已停用，请发送 /start 重新选择。"})
		return nil
	}
	if strings.TrimSpace(input.Text) == "" || message.Kind != "text" {
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "请以文字发送核验凭据。"})
		return nil
	}
	result, err := s.verifier.Verify(ctx, profile.Request, domain.VerificationInput{Credential: input.Text, UserID: input.From.ID, Username: input.From.Username})
	if err != nil {
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "核验服务暂时不可用，请稍后重新输入。"})
		return nil
	}
	if !result.Passed {
		*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "验证未通过，请检查凭据后重新输入。"})
		return nil
	}
	// 配置提取的上游身份字段也可能回显凭据，不能借结果字段持久化原文。
	result.ExternalID = strings.ReplaceAll(result.ExternalID, input.Text, "[已隐藏]")
	result.DisplayName = strings.ReplaceAll(result.DisplayName, input.Text, "[已隐藏]")
	grant := domain.Grant{UserID: input.From.ID, ProfileID: profile.ID, Active: true, ExternalID: result.ExternalID, DisplayName: result.DisplayName, VerifiedAt: time.Now()}
	if err := store.PutGrant(ctx, grant); err != nil {
		return err
	}
	if err := store.DeleteConversation(ctx, input.From.ID); err != nil {
		return err
	}
	*replies = append(*replies, domain.Reply{ChatID: input.Chat.ID, Text: "验证通过。"})
	return s.menu(ctx, store, settings, input.From.ID, replies)
}

func (s *Service) menu(ctx context.Context, store domain.Store, settings domain.Settings, userID int64, replies *[]domain.Reply) error {
	profiles, err := store.Profiles(ctx)
	if err != nil {
		return err
	}
	grants, err := store.Grants(ctx, userID)
	if err != nil {
		return err
	}
	active := make(map[string]bool, len(grants))
	for _, grant := range grants {
		if grant.Active {
			active[grant.ProfileID] = true
		}
	}
	memberCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	buttons := make([][]domain.Button, 0)
	checked := make(map[int64]bool)
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		if !active[profile.ID] {
			buttons = append(buttons, []domain.Button{{Text: "验证：" + profile.Name, Data: "v:" + profile.ID}})
			continue
		}
		for _, id := range profile.ChatIDs {
			if checked[id] {
				continue
			}
			checked[id] = true
			chat, err := store.Chat(ctx, id)
			if err != nil {
				return err
			}
			if !chat.Enabled || chat.BotStatus != "administrator" || !chat.CanInvite {
				continue
			}
			if memberCtx.Err() == nil {
				member, err := s.tg.Member(memberCtx, settings.Token, id, userID)
				if err != nil {
					slog.Warn("bot menu membership lookup failed", "chat_id", id)
				} else {
					switch member.Status {
					case "member", "administrator", "creator":
						continue
					case "restricted":
						if member.IsMember {
							continue
						}
					}
				}
			}
			buttons = append(buttons, []domain.Button{{Text: "加入：" + chat.Title, Data: "g:" + profile.ID + ":" + strconv.FormatInt(id, 10)}})
		}
	}
	text := "请选择验证方式或加入已授权的群组。"
	if len(buttons) == 0 {
		text = "当前没有待完成的验证或可加入的群组。"
	}
	*replies = append(*replies, domain.Reply{ChatID: userID, Text: text, Buttons: buttons})
	return nil
}

func eligible(ctx context.Context, store domain.Store, userID int64, profileID string, chatID int64) (bool, error) {
	profile, err := store.Profile(ctx, profileID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !profile.Enabled {
		return false, nil
	}
	bound := false
	for _, id := range profile.ChatIDs {
		if id == chatID {
			bound = true
			break
		}
	}
	if !bound {
		return false, nil
	}
	chat, err := store.Chat(ctx, chatID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !chat.Enabled || chat.BotStatus != "administrator" || !chat.CanInvite {
		return false, nil
	}
	grants, err := store.Grants(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, grant := range grants {
		if grant.ProfileID == profileID && grant.Active {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) callbackUpdate(ctx context.Context, store domain.Store, settings domain.Settings, callback domain.Callback, replies *[]domain.Reply) error {
	if callback.Message == nil || callback.Message.Chat.Type != "private" || callback.Message.Chat.ID != callback.From.ID {
		return nil
	}
	if err := updateUser(ctx, store, callback.From, true); err != nil {
		return err
	}
	parts := strings.Split(callback.Data, ":")
	if len(parts) == 2 && parts[0] == "v" {
		profile, err := store.Profile(ctx, parts[1])
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if errors.Is(err, domain.ErrNotFound) || !profile.Enabled {
			*replies = append(*replies, domain.Reply{ChatID: callback.From.ID, Text: "验证方式已停用，请发送 /start 重新选择。"})
			return nil
		}
		if err := store.PutConversation(ctx, domain.Conversation{UserID: callback.From.ID, ProfileID: profile.ID, ExpiresAt: time.Now().Add(15 * time.Minute)}); err != nil {
			return err
		}
		prompt := profile.Prompt
		if prompt == "" {
			prompt = "请发送核验凭据。"
		}
		*replies = append(*replies, domain.Reply{ChatID: callback.From.ID, Text: prompt + "\n\n请在15分钟内输入，发送 /cancel 取消。"})
		return nil
	}
	if len(parts) != 3 || parts[0] != "g" {
		return nil
	}
	chatID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil
	}
	ok, err := eligible(ctx, store, callback.From.ID, parts[1], chatID)
	if err != nil {
		return err
	}
	if !ok {
		*replies = append(*replies, domain.Reply{ChatID: callback.From.ID, Text: "当前无权加入该群组，请发送 /start 查看可用群组。"})
		return nil
	}
	invites, err := store.Invites(ctx, chatID, callback.From.ID)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, invite := range invites {
		if invite.ProfileID == parts[1] && invite.Status == "pending" && invite.ExpiresAt.After(now) {
			*replies = append(*replies, domain.Reply{ChatID: callback.From.ID, Text: "请使用下方链接申请加入，仅限当前账号使用。", Buttons: [][]domain.Button{{{Text: "申请加入", URL: invite.Link}}}})
			return nil
		}
	}
	expires := now.Add(30 * time.Minute)
	link, err := s.tg.Invite(ctx, settings.Token, chatID, expires)
	if err != nil {
		text := "暂时无法生成邀请，请稍后重新点击。"
		if errors.Is(err, domain.ErrUnknown) {
			text = "生成邀请的结果未知，请稍后重新点击。"
		}
		*replies = append(*replies, domain.Reply{ChatID: callback.From.ID, Text: text})
		return nil
	}
	invite := domain.Invite{ID: uuid.NewString(), UserID: callback.From.ID, ChatID: chatID, ProfileID: parts[1], Link: link, ExpiresAt: expires, Status: "pending"}
	if err := store.PutInvite(ctx, invite); err != nil {
		return err
	}
	*replies = append(*replies, domain.Reply{ChatID: callback.From.ID, Text: "邀请30分钟内有效，仅限当前账号使用。", Buttons: [][]domain.Button{{{Text: "申请加入", URL: link}}}})
	return nil
}

func (s *Service) botMemberUpdate(ctx context.Context, store domain.Store, update domain.MemberUpdate) error {
	chat, err := updateChat(ctx, store, update.Chat)
	if err != nil {
		return err
	}
	chat.BotStatus, chat.CanInvite, chat.CanRestrict, chat.CanPost = update.New.Status, update.New.CanInvite, update.New.CanRestrict, update.New.CanPost
	return store.PutChat(ctx, chat)
}

func (s *Service) memberUpdate(ctx context.Context, store domain.Store, updateID int64, update domain.MemberUpdate) error {
	if _, err := updateChat(ctx, store, update.Chat); err != nil {
		return err
	}
	if err := updateUser(ctx, store, update.New.User, false); err != nil {
		return err
	}
	old, err := store.Members(ctx, update.Chat.ID, update.New.User.ID)
	if err != nil {
		return err
	}
	if len(old) > 0 {
		eventAt := time.Unix(update.Date, 0)
		if old[0].UpdatedAt.After(eventAt) || (old[0].UpdatedAt.Equal(eventAt) && old[0].LastUpdateID >= updateID) {
			return nil
		}
	}
	return store.PutMember(ctx, domain.Member{ChatID: update.Chat.ID, UserID: update.New.User.ID, Status: update.New.Status, UntilDate: update.New.UntilDate, UpdatedAt: time.Unix(update.Date, 0), LastUpdateID: updateID})
}

func (s *Service) joinUpdate(ctx context.Context, store domain.Store, settings domain.Settings, updateID int64, request domain.JoinRequest, replies *[]domain.Reply) error {
	if request.InviteLink == nil {
		return nil
	}
	invite, err := store.InviteByLink(ctx, request.InviteLink.Link)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	} // 不处理其他管理员的邀请入口。
	if err != nil {
		return err
	}
	owner := invite.UserID == request.From.ID && invite.ChatID == request.Chat.ID
	approve := owner && invite.Status == "pending" && invite.ExpiresAt.After(time.Now())
	if approve {
		approve, err = eligible(ctx, store, request.From.ID, invite.ProfileID, request.Chat.ID)
		if err != nil {
			return err
		}
	}
	if err := s.tg.Join(ctx, settings.Token, request.Chat.ID, request.From.ID, approve); err != nil {
		if owner {
			invite.Status = "failed"
			text := "入群申请暂未完成，请发送 /start 重新申请。"
			if errors.Is(err, domain.ErrUnknown) {
				invite.Status, text = "unknown", "入群申请的结果未知，请检查是否已加入，或发送 /start 重试。"
			}
			if err := store.PutInvite(ctx, invite); err != nil {
				return err
			}
			*replies = append(*replies, domain.Reply{ChatID: request.From.ID, Text: text})
		}
		return nil
	}
	if !owner {
		return nil
	} // 被转发链接的其他用户被拒绝，原用户仍可使用该邀请。
	invite.Status = "revoked"
	if approve {
		invite.Status = "approved"
		if err := store.PutMember(ctx, domain.Member{ChatID: request.Chat.ID, UserID: request.From.ID, Status: "member", UpdatedAt: time.Now(), LastUpdateID: updateID}); err != nil {
			return err
		}
		*replies = append(*replies, domain.Reply{ChatID: request.From.ID, Text: "入群申请已通过。"})
	} else {
		*replies = append(*replies, domain.Reply{ChatID: request.From.ID, Text: "邀请已失效或资格已撤销，请发送 /start 查看。"})
	}
	return store.PutInvite(ctx, invite)
}
