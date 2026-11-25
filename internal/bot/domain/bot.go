package domain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var (
	ErrForbidden   = errors.New("forbidden")
	ErrInvalid     = errors.New("invalid input")
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("conflict")
	ErrUnavailable = errors.New("service unavailable")
	ErrUnknown     = errors.New("operation result unknown")
)

type Settings struct {
	BotID         int64  `json:"botId"`
	Username      string `json:"username"`
	HasToken      bool   `json:"hasToken"`
	Token         string `json:"-"`
	WebhookSecret string `json:"-"`
	Welcome       string `json:"welcome"`
	RetentionDays int    `json:"retentionDays"`
}
type SaveSettings struct {
	Token         *string `json:"token,omitempty"`
	Welcome       string  `json:"welcome"`
	RetentionDays int     `json:"retentionDays"`
}
type Rule struct {
	Path     string          `json:"path"`
	Expected json.RawMessage `json:"expected"`
}
type RequestConfig struct {
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Headers         map[string]string `json:"headers"`
	Query           map[string]string `json:"query"`
	Body            json.RawMessage   `json:"body,omitempty"`
	Rules           []Rule            `json:"rules"`
	IdentityPath    string            `json:"identityPath,omitempty"`
	DisplayNamePath string            `json:"displayNamePath,omitempty"`
}
type Profile struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Enabled bool          `json:"enabled"`
	Prompt  string        `json:"prompt"`
	Request RequestConfig `json:"request"`
	ChatIDs []int64       `json:"chatIds"`
}
type VerificationInput struct {
	Credential string
	UserID     int64
	Username   string
}
type VerificationResult struct {
	Passed      bool   `json:"passed"`
	ExternalID  string `json:"externalId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}
type Chat struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Username    string `json:"username"`
	Enabled     bool   `json:"enabled"`
	BotStatus   string `json:"botStatus"`
	CanInvite   bool   `json:"canInvite"`
	CanRestrict bool   `json:"canRestrict"`
	CanPost     bool   `json:"canPost"`
}
type User struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	Note        string    `json:"note"`
	PrivateChat bool      `json:"privateChat"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type Grant struct {
	UserID      int64     `json:"userId"`
	ProfileID   string    `json:"profileId"`
	Active      bool      `json:"active"`
	ExternalID  string    `json:"externalId"`
	DisplayName string    `json:"displayName"`
	VerifiedAt  time.Time `json:"verifiedAt"`
}
type Member struct {
	ChatID       int64     `json:"chatId"`
	UserID       int64     `json:"userId"`
	Status       string    `json:"status"`
	UntilDate    int64     `json:"untilDate"`
	UpdatedAt    time.Time `json:"updatedAt"`
	LastUpdateID int64     `json:"-"`
}
type Conversation struct {
	UserID    int64
	ProfileID string
	ExpiresAt time.Time
}
type Invite struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"userId"`
	ChatID    int64     `json:"chatId"`
	ProfileID string    `json:"profileId"`
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expiresAt"`
	Status    string    `json:"status"`
}
type Message struct {
	ID                string     `json:"id"`
	ChatID            int64      `json:"chatId"`
	TelegramMessageID int64      `json:"telegramMessageId"`
	SenderID          int64      `json:"senderId,omitempty"`
	SenderName        string     `json:"senderName"`
	Kind              string     `json:"kind"`
	Text              string     `json:"text"`
	FileID            string     `json:"fileId,omitempty"`
	FileName          string     `json:"fileName,omitempty"`
	MIMEType          string     `json:"mimeType,omitempty"`
	FileSize          int64      `json:"fileSize,omitempty"`
	Direction         string     `json:"direction"`
	Status            string     `json:"status"`
	ReplyToMessageID  int64      `json:"replyToMessageId,omitempty"`
	Date              time.Time  `json:"date"`
	EditedAt          *time.Time `json:"editedAt,omitempty"`
	Sensitive         bool       `json:"sensitive"`
	LastUpdateID      int64      `json:"-"`
}
type MessagePage struct {
	Items      []Message `json:"items"`
	NextCursor string    `json:"nextCursor"`
}
type UserPage struct {
	Items      []User `json:"items"`
	NextCursor string `json:"nextCursor"`
}
type UserDetail struct {
	User    User     `json:"user"`
	Grants  []Grant  `json:"grants"`
	Members []Member `json:"members"`
}
type SendMessage struct {
	ChatID           int64
	Kind             string
	Text             string
	ReplyToMessageID int64
	FileName         string
	MIMEType         string
	Size             int64
	File             io.Reader
}
type MemberAction struct {
	ChatID    int64  `json:"chatId"`
	UserID    int64  `json:"userId"`
	Action    string `json:"action"`
	UntilDate int64  `json:"untilDate"`
}
type File struct {
	Body     io.ReadCloser
	MIMEType string
	Size     int64
	Name     string
}
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data,omitempty"`
	URL  string `json:"url,omitempty"`
}

const ReplyTopic = "watchover.bot.reply"

type Reply struct {
	ChatID  int64
	Text    string
	Buttons [][]Button
}

type Store interface {
	Transaction(context.Context, func(Store) error) error
	GetSettings(context.Context) (Settings, error)
	PutSettings(context.Context, Settings) error
	Profiles(context.Context) ([]Profile, error)
	Profile(context.Context, string) (Profile, error)
	PutProfile(context.Context, Profile) error
	DeleteProfile(context.Context, string) error
	Chats(context.Context) ([]Chat, error)
	Chat(context.Context, int64) (Chat, error)
	PutChat(context.Context, Chat) error
	Users(context.Context, string, string, int) (UserPage, error)
	User(context.Context, int64) (User, error)
	PutUser(context.Context, User) error
	Grants(context.Context, int64) ([]Grant, error)
	PutGrant(context.Context, Grant) error
	Members(context.Context, int64, int64) ([]Member, error)
	PutMember(context.Context, Member) error
	Conversation(context.Context, int64) (Conversation, error)
	PutConversation(context.Context, Conversation) error
	DeleteConversation(context.Context, int64) error
	Invites(context.Context, int64, int64) ([]Invite, error)
	InviteByLink(context.Context, string) (Invite, error)
	PutInvite(context.Context, Invite) error
	Messages(context.Context, int64, string, int) (MessagePage, error)
	Message(context.Context, string) (Message, error)
	MessageByTelegram(context.Context, int64, int64) (Message, error)
	PutMessage(context.Context, Message) error
	HasReceipt(context.Context, int64) (bool, error)
	PutReceipt(context.Context, int64, time.Time) error
	Cleanup(context.Context, time.Time, time.Time) error
}

type Telegram interface {
	Me(context.Context, string) (TelegramUser, error)
	Chat(context.Context, string, int64) (TelegramChat, error)
	Member(context.Context, string, int64, int64) (TelegramMember, error)
	Webhook(context.Context, string, string, string, bool) error
	WebhookInfo(context.Context, string) (WebhookInfo, error)
	Invite(context.Context, string, int64, time.Time) (string, error)
	Join(context.Context, string, int64, int64, bool) error
	Act(context.Context, string, MemberAction) error
	Send(context.Context, string, SendMessage) (TelegramMessage, error)
	Reply(context.Context, string, Reply) (TelegramMessage, error)
	AnswerCallback(context.Context, string, string) error
	Download(context.Context, string, Message) (File, error)
}
type Verifier interface {
	Validate(RequestConfig) error
	Verify(context.Context, RequestConfig, VerificationInput) (VerificationResult, error)
}
type Publisher interface{ Publish(Reply) error }

type WebhookInfo struct {
	URL           string `json:"url"`
	Pending       int    `json:"pending_update_count"`
	LastError     string `json:"last_error_message"`
	LastErrorDate int64  `json:"last_error_date"`
}
type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}
type TelegramChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}
type TelegramMember struct {
	User        TelegramUser `json:"user"`
	Status      string       `json:"status"`
	UntilDate   int64        `json:"until_date"`
	CanInvite   bool         `json:"can_invite_users"`
	CanRestrict bool         `json:"can_restrict_members"`
	CanPost     bool         `json:"can_post_messages"`
}
type Photo struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
}
type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MIMEType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}
type TelegramMessage struct {
	Video       *struct{}        `json:"video"`
	Voice       *struct{}        `json:"voice"`
	Audio       *struct{}        `json:"audio"`
	Animation   *struct{}        `json:"animation"`
	Sticker     *struct{}        `json:"sticker"`
	VideoNote   *struct{}        `json:"video_note"`
	MessageID   int64            `json:"message_id"`
	Date        int64            `json:"date"`
	EditDate    int64            `json:"edit_date"`
	From        *TelegramUser    `json:"from"`
	SenderChat  *TelegramChat    `json:"sender_chat"`
	Chat        TelegramChat     `json:"chat"`
	Text        string           `json:"text"`
	Caption     string           `json:"caption"`
	Photo       []Photo          `json:"photo"`
	Document    *Document        `json:"document"`
	ReplyTo     *TelegramMessage `json:"reply_to_message"`
	MigrateTo   int64            `json:"migrate_to_chat_id"`
	MigrateFrom int64            `json:"migrate_from_chat_id"`
}
type Callback struct {
	ID      string           `json:"id"`
	From    TelegramUser     `json:"from"`
	Message *TelegramMessage `json:"message"`
	Data    string           `json:"data"`
}
type MemberUpdate struct {
	Chat TelegramChat   `json:"chat"`
	Date int64          `json:"date"`
	New  TelegramMember `json:"new_chat_member"`
}
type JoinRequest struct {
	Chat       TelegramChat `json:"chat"`
	From       TelegramUser `json:"from"`
	InviteLink *struct {
		Link string `json:"invite_link"`
	} `json:"invite_link"`
}
type Update struct {
	ID                int64            `json:"update_id"`
	Message           *TelegramMessage `json:"message"`
	EditedMessage     *TelegramMessage `json:"edited_message"`
	ChannelPost       *TelegramMessage `json:"channel_post"`
	EditedChannelPost *TelegramMessage `json:"edited_channel_post"`
	Callback          *Callback        `json:"callback_query"`
	MyMember          *MemberUpdate    `json:"my_chat_member"`
	Member            *MemberUpdate    `json:"chat_member"`
	Join              *JoinRequest     `json:"chat_join_request"`
}
