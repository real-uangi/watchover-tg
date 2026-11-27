package application

import (
	"context"
	"errors"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

type memoryStore struct {
	domain.Store
	settings                         domain.Settings
	profiles                         map[string]domain.Profile
	chats                            map[int64]domain.Chat
	users                            map[int64]domain.User
	grants                           map[int64]map[string]domain.Grant
	conversations                    map[int64]domain.Conversation
	invites                          map[string]domain.Invite
	messages                         map[string]domain.Message
	members                          map[[2]int64]domain.Member
	receipts                         map[int64]time.Time
	receiptErr                       error
	cleanupMessages, cleanupReceipts time.Time
}

func newMemoryStore() *memoryStore {
	return &memoryStore{settings: domain.Settings{Token: "test-token", WebhookSecret: "test-secret", BotID: 99, RetentionDays: 30}, profiles: map[string]domain.Profile{}, chats: map[int64]domain.Chat{}, users: map[int64]domain.User{}, grants: map[int64]map[string]domain.Grant{}, conversations: map[int64]domain.Conversation{}, invites: map[string]domain.Invite{}, messages: map[string]domain.Message{}, members: map[[2]int64]domain.Member{}, receipts: map[int64]time.Time{}}
}
func (m *memoryStore) Transaction(ctx context.Context, fn func(domain.Store) error) error {
	copy := *m
	copy.profiles, copy.chats, copy.users = maps.Clone(m.profiles), maps.Clone(m.chats), maps.Clone(m.users)
	copy.conversations, copy.invites, copy.messages = maps.Clone(m.conversations), maps.Clone(m.invites), maps.Clone(m.messages)
	copy.members, copy.receipts, copy.grants = maps.Clone(m.members), maps.Clone(m.receipts), maps.Clone(m.grants)
	for id, grants := range m.grants {
		copy.grants[id] = maps.Clone(grants)
	}
	if err := fn(&copy); err != nil {
		return err
	}
	*m = copy
	return nil
}
func (m *memoryStore) GetSettings(context.Context) (domain.Settings, error) { return m.settings, nil }
func (m *memoryStore) PutSettings(_ context.Context, s domain.Settings) error {
	m.settings = s
	return nil
}
func (m *memoryStore) Profiles(context.Context) ([]domain.Profile, error) {
	list := []domain.Profile{}
	for _, p := range m.profiles {
		list = append(list, p)
	}
	return list, nil
}
func (m *memoryStore) Profile(_ context.Context, id string) (domain.Profile, error) {
	p, ok := m.profiles[id]
	if !ok {
		return p, domain.ErrNotFound
	}
	return p, nil
}
func (m *memoryStore) PutProfile(_ context.Context, p domain.Profile) error {
	m.profiles[p.ID] = p
	return nil
}
func (m *memoryStore) Chat(_ context.Context, id int64) (domain.Chat, error) {
	c, ok := m.chats[id]
	if !ok {
		return c, domain.ErrNotFound
	}
	return c, nil
}
func (m *memoryStore) PutChat(_ context.Context, c domain.Chat) error { m.chats[c.ID] = c; return nil }
func (m *memoryStore) User(_ context.Context, id int64) (domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return u, domain.ErrNotFound
	}
	return u, nil
}
func (m *memoryStore) PutUser(_ context.Context, u domain.User) error { m.users[u.ID] = u; return nil }
func (m *memoryStore) Grants(_ context.Context, id int64) ([]domain.Grant, error) {
	list := []domain.Grant{}
	for _, g := range m.grants[id] {
		list = append(list, g)
	}
	return list, nil
}
func (m *memoryStore) PutGrant(_ context.Context, g domain.Grant) error {
	if m.grants[g.UserID] == nil {
		m.grants[g.UserID] = map[string]domain.Grant{}
	}
	m.grants[g.UserID][g.ProfileID] = g
	return nil
}
func (m *memoryStore) Conversation(_ context.Context, id int64) (domain.Conversation, error) {
	c, ok := m.conversations[id]
	if !ok {
		return c, domain.ErrNotFound
	}
	return c, nil
}
func (m *memoryStore) PutConversation(_ context.Context, c domain.Conversation) error {
	m.conversations[c.UserID] = c
	return nil
}
func (m *memoryStore) DeleteConversation(_ context.Context, id int64) error {
	delete(m.conversations, id)
	return nil
}
func (m *memoryStore) Invites(_ context.Context, chat, user int64) ([]domain.Invite, error) {
	list := []domain.Invite{}
	for _, i := range m.invites {
		if (chat == 0 || i.ChatID == chat) && (user == 0 || i.UserID == user) {
			list = append(list, i)
		}
	}
	return list, nil
}
func (m *memoryStore) InviteByLink(_ context.Context, link string) (domain.Invite, error) {
	for _, i := range m.invites {
		if i.Link == link {
			return i, nil
		}
	}
	return domain.Invite{}, domain.ErrNotFound
}
func (m *memoryStore) PutInvite(_ context.Context, i domain.Invite) error {
	m.invites[i.ID] = i
	return nil
}
func (m *memoryStore) Message(_ context.Context, id string) (domain.Message, error) {
	v, ok := m.messages[id]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}
func (m *memoryStore) MessageByTelegram(_ context.Context, chat, id int64) (domain.Message, error) {
	for _, v := range m.messages {
		if v.ChatID == chat && v.TelegramMessageID == id {
			return v, nil
		}
	}
	return domain.Message{}, domain.ErrNotFound
}
func (m *memoryStore) PutMessage(_ context.Context, v domain.Message) error {
	for id, old := range m.messages {
		if v.TelegramMessageID != 0 && old.ChatID == v.ChatID && old.TelegramMessageID == v.TelegramMessageID {
			v.ID = id
			break
		}
	}
	m.messages[v.ID] = v
	return nil
}
func (m *memoryStore) Members(_ context.Context, chat, user int64) ([]domain.Member, error) {
	list := []domain.Member{}
	for _, v := range m.members {
		if (chat == 0 || v.ChatID == chat) && (user == 0 || v.UserID == user) {
			list = append(list, v)
		}
	}
	return list, nil
}
func (m *memoryStore) PutMember(_ context.Context, v domain.Member) error {
	m.members[[2]int64{v.ChatID, v.UserID}] = v
	return nil
}
func (m *memoryStore) HasReceipt(_ context.Context, id int64) (bool, error) {
	_, ok := m.receipts[id]
	return ok, nil
}
func (m *memoryStore) PutReceipt(_ context.Context, id int64, at time.Time) error {
	if m.receiptErr != nil {
		return m.receiptErr
	}
	m.receipts[id] = at
	return nil
}
func (m *memoryStore) Cleanup(_ context.Context, a, b time.Time) error {
	m.cleanupMessages, m.cleanupReceipts = a, b
	return nil
}

type fakeVerifier struct {
	calls       int
	testCalls   int
	result      domain.VerificationResult
	testResult  domain.VerificationTestResult
	validateErr error
	err         error
}

func (v *fakeVerifier) Validate(domain.RequestConfig) error { return v.validateErr }
func (v *fakeVerifier) Verify(context.Context, domain.RequestConfig, domain.VerificationInput) (domain.VerificationResult, error) {
	v.calls++
	return v.result, v.err
}
func (v *fakeVerifier) Test(context.Context, domain.RequestConfig, domain.VerificationInput) (domain.VerificationTestResult, error) {
	v.testCalls++
	return v.testResult, v.err
}

func TestProfileReturnsDiagnosticsWithoutStore(t *testing.T) {
	v := &fakeVerifier{testResult: domain.VerificationTestResult{
		VerificationResult: domain.VerificationResult{Passed: true, ExternalID: "test-credential", DisplayName: "test-credential"},
		Message:            "验证通过",
		Request:            domain.VerificationTestRequest{URL: "https://verify.example/?key=test-credential"},
	}}
	s := NewService(nil, nil, v, nil, "https://example.com")
	result, err := s.TestProfile(context.Background(), domain.Profile{}, domain.VerificationInput{Credential: "test-credential"})
	if err != nil || !result.Passed || result.ExternalID != "test-credential" || result.DisplayName != "test-credential" || result.Request.URL != v.testResult.Request.URL || v.calls != 0 || v.testCalls != 1 {
		t.Fatalf("test result=%+v error=%v verify calls=%d test calls=%d", result, err, v.calls, v.testCalls)
	}
}

func TestProfileRejectsInvalidInputBeforeRequest(t *testing.T) {
	v := &fakeVerifier{}
	s := NewService(nil, nil, v, nil, "https://example.com")
	if _, err := s.TestProfile(context.Background(), domain.Profile{}, domain.VerificationInput{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("empty credential was accepted", err)
	}
	v.validateErr = domain.ErrInvalid
	if _, err := s.TestProfile(context.Background(), domain.Profile{}, domain.VerificationInput{Credential: "key"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("invalid configuration was accepted", err)
	}
	if v.calls != 0 || v.testCalls != 0 {
		t.Fatal("invalid input reached external verification")
	}
}

type fakePublisher struct {
	replies []domain.Reply
	updates []domain.Update
	err     error
}

func (p *fakePublisher) Publish(r domain.Reply) error { p.replies = append(p.replies, r); return p.err }
func (p *fakePublisher) PublishUpdate(u domain.Update) error {
	p.updates = append(p.updates, u)
	return p.err
}

type fakeTelegram struct {
	domain.Telegram
	joins       []bool
	inviteCalls int
	sendErr     error
	sendResult  domain.TelegramMessage
	me          domain.TelegramUser
	acts        int
	member      domain.TelegramMember
}

func (f *fakeTelegram) Me(context.Context, string) (domain.TelegramUser, error) { return f.me, nil }
func (f *fakeTelegram) Invite(context.Context, string, int64, time.Time) (string, error) {
	f.inviteCalls++
	return "https://t.me/+test", nil
}
func (f *fakeTelegram) Join(_ context.Context, _ string, _ int64, _ int64, approve bool) error {
	f.joins = append(f.joins, approve)
	return nil
}
func (*fakeTelegram) AnswerCallback(context.Context, string, string) error { return nil }
func (f *fakeTelegram) Send(context.Context, string, domain.SendMessage) (domain.TelegramMessage, error) {
	return f.sendResult, f.sendErr
}
func (f *fakeTelegram) Act(context.Context, string, domain.MemberAction) error { f.acts++; return nil }
func (f *fakeTelegram) Member(context.Context, string, int64, int64) (domain.TelegramMember, error) {
	return f.member, nil
}

func fixture() (*Service, *memoryStore, *fakeVerifier, *fakeTelegram, *fakePublisher) {
	store, verifier, tg, publisher := newMemoryStore(), &fakeVerifier{result: domain.VerificationResult{Passed: true}}, &fakeTelegram{}, &fakePublisher{}
	store.profiles["profile"] = domain.Profile{ID: "profile", Name: "身份核验", Enabled: true, ChatIDs: []int64{-100}}
	store.chats[-100] = domain.Chat{ID: -100, Type: "supergroup", Title: "测试群", Enabled: true, BotStatus: "administrator", CanInvite: true, CanRestrict: true}
	return NewService(store, tg, verifier, publisher, "https://example.com"), store, verifier, tg, publisher
}
func callback(id int64, data string) domain.Update {
	return domain.Update{ID: id, Callback: &domain.Callback{ID: "callback", From: domain.TelegramUser{ID: 42, Username: "user"}, Message: &domain.TelegramMessage{Chat: domain.TelegramChat{ID: 42, Type: "private"}}, Data: data}}
}
func credential(id int64, text string) domain.Update {
	return domain.Update{ID: id, Message: &domain.TelegramMessage{MessageID: 10, Date: time.Now().Unix(), From: &domain.TelegramUser{ID: 42, Username: "user"}, Chat: domain.TelegramChat{ID: 42, Type: "private"}, Text: text}}
}
func apply(t *testing.T, s *Service, u domain.Update) {
	t.Helper()
	if err := s.HandleUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func TestReceiveUpdateQueuesWithoutBusinessLock(t *testing.T) {
	s, store, v, _, p := fixture()
	u := credential(1, "/start")
	s.updates.Lock()
	defer s.updates.Unlock()
	done := make(chan error, 1)
	go func() { done <- s.ReceiveUpdate(context.Background(), "test-secret", u) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("receiving update waited for the business lock")
	}
	if len(p.updates) != 1 || p.updates[0].ID != u.ID || p.updates[0].Message.Text != "/start" {
		t.Fatal("update was not queued")
	}
	if len(store.users) != 0 || len(store.messages) != 0 || len(store.receipts) != 0 || len(p.replies) != 0 || v.calls != 0 {
		t.Fatal("receiving update executed business processing")
	}
}

func TestStartPublishesWelcomeAndMenu(t *testing.T) {
	for _, tc := range []struct {
		name, welcome string
		profile       bool
		enabled       bool
	}{
		{name: "available menu", welcome: "配置的欢迎文案", profile: true, enabled: true},
		{name: "no profiles", welcome: "配置的欢迎文案"},
		{name: "disabled profile", welcome: "配置的欢迎文案", profile: true},
		{name: "empty welcome", profile: true, enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, store, _, _, p := fixture()
			store.settings.Welcome = tc.welcome
			if !tc.profile {
				clear(store.profiles)
			} else {
				profile := store.profiles["profile"]
				profile.Enabled = tc.enabled
				store.profiles["profile"] = profile
			}
			u := credential(1, "/start")
			apply(t, s, u)
			apply(t, s, u)
			wantCount := 1
			if tc.welcome != "" {
				wantCount++
			}
			if len(p.replies) != wantCount {
				t.Fatalf("got %d replies, want %d", len(p.replies), wantCount)
			}
			menuText, menuButtons := "当前没有可用的验证方式或群组，请联系管理员。", 0
			if tc.enabled {
				menuText, menuButtons = "请选择验证方式或加入已授权的群组。", 1
			}
			welcomeSeen, menuSeen := false, false
			for _, reply := range p.replies {
				if reply.ChatID != 42 {
					t.Fatalf("wrong reply destination: %d", reply.ChatID)
				}
				switch reply.Text {
				case tc.welcome:
					welcomeSeen = true
					if len(reply.Buttons) != 0 {
						t.Fatal("welcome contains menu buttons")
					}
				case menuText:
					menuSeen = true
					if len(reply.Buttons) != menuButtons {
						t.Fatalf("got %d menu rows, want %d", len(reply.Buttons), menuButtons)
					}
				default:
					t.Fatalf("unexpected reply: %+v", reply)
				}
			}
			if !menuSeen || (tc.welcome != "" && !welcomeSeen) {
				t.Fatal("welcome or menu missing")
			}
		})
	}
}

func TestWelcomePublishFailureDoesNotSkipMenu(t *testing.T) {
	s, store, _, _, p := fixture()
	store.settings.Welcome = "配置的欢迎文案"
	p.err = domain.ErrUnavailable
	apply(t, s, credential(1, "/start"))
	if len(p.replies) != 2 || len(store.receipts) != 1 {
		t.Fatal("reply failure skipped another reply or rolled back completion")
	}
}

func TestVerificationDoesNotRepeatWelcome(t *testing.T) {
	s, store, _, _, p := fixture()
	store.settings.Welcome = "配置的欢迎文案"
	apply(t, s, callback(1, "v:profile"))
	apply(t, s, credential(2, "card"))
	if len(p.replies) != 3 {
		t.Fatalf("expected verification prompt, success and menu, got %d replies", len(p.replies))
	}
	for _, reply := range p.replies {
		if strings.Contains(reply.Text, store.settings.Welcome) {
			t.Fatal("verification repeated welcome")
		}
	}
}

func TestVerificationInvitationAndForwardedLink(t *testing.T) {
	s, store, v, tg, p := fixture()
	v.result.ExternalID = "secret-card"
	apply(t, s, callback(1, "v:profile"))
	if remaining := time.Until(store.conversations[42].ExpiresAt); remaining < 14*time.Minute || remaining > 15*time.Minute {
		t.Fatalf("conversation deadline: %v", remaining)
	}
	u := credential(2, "secret-card")
	apply(t, s, u)
	apply(t, s, u)
	if v.calls != 1 {
		t.Fatalf("duplicate verification calls: %d", v.calls)
	}
	if !store.grants[42]["profile"].Active || store.grants[42]["profile"].ExternalID == "secret-card" {
		t.Fatal("missing grant or credential echo persisted")
	}
	message, err := store.MessageByTelegram(context.Background(), 42, 10)
	if err != nil || !message.Sensitive || message.Text != sensitiveText {
		t.Fatalf("sensitive message: %+v %v", message, err)
	}
	for _, r := range p.replies {
		if strings.Contains(r.Text, "secret-card") {
			t.Fatal("credential in event")
		}
	}
	edited := *u.Message
	edited.EditDate = edited.Date + 10
	edited.Text = "updated-card"
	apply(t, s, domain.Update{ID: 3, EditedMessage: &edited})
	message, _ = store.MessageByTelegram(context.Background(), 42, 10)
	if message.Text != sensitiveText || v.calls != 1 {
		t.Fatal("edit restored or reverified credential")
	}
	apply(t, s, callback(4, "g:profile:-100"))
	if tg.inviteCalls != 1 || len(store.invites) != 1 {
		t.Fatal("invitation missing")
	}
	var invite domain.Invite
	for _, i := range store.invites {
		invite = i
	}
	if remaining := time.Until(invite.ExpiresAt); remaining < 29*time.Minute || remaining > 30*time.Minute {
		t.Fatalf("invite deadline: %v", remaining)
	}
	join := domain.JoinRequest{Chat: domain.TelegramChat{ID: -100}, From: domain.TelegramUser{ID: 43}, InviteLink: &struct {
		Link string `json:"invite_link"`
	}{Link: invite.Link}}
	apply(t, s, domain.Update{ID: 5, Join: &join})
	if tg.joins[0] {
		t.Fatal("forwarded link approved")
	}
	join.From.ID = 42
	apply(t, s, domain.Update{ID: 6, Join: &join})
	if !tg.joins[1] || store.invites[invite.ID].Status != "approved" {
		t.Fatal("verified owner not approved")
	}
}

func TestReceiptFailureRollsBackQualification(t *testing.T) {
	s, store, v, _, p := fixture()
	store.conversations[42] = domain.Conversation{UserID: 42, ProfileID: "profile", ExpiresAt: time.Now().Add(time.Minute)}
	store.receiptErr = errors.New("storage failed")
	u := credential(7, "card")
	if err := s.HandleUpdate(context.Background(), u); err == nil {
		t.Fatal("expected storage failure")
	}
	if len(store.receipts) != 0 || len(store.messages) != 0 || len(store.grants) != 0 || len(p.replies) != 0 {
		t.Fatal("partial transaction committed")
	}
	store.receiptErr = nil
	apply(t, s, u)
	if v.calls != 2 || !store.grants[42]["profile"].Active {
		t.Fatal("retry after rollback did not complete")
	}
}

func TestConcurrentWebhookReplayVerifiesOnce(t *testing.T) {
	s, store, v, _, _ := fixture()
	store.conversations[42] = domain.Conversation{UserID: 42, ProfileID: "profile", ExpiresAt: time.Now().Add(time.Minute)}
	u := credential(8, "card")
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.HandleUpdate(context.Background(), u) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if v.calls != 1 || len(store.receipts) != 1 {
		t.Fatalf("verification=%d receipts=%d", v.calls, len(store.receipts))
	}
}

func TestWrongWebhookSecretHasNoSideEffects(t *testing.T) {
	s, store, _, _, p := fixture()
	if err := s.ReceiveUpdate(context.Background(), "wrong", credential(9, "card")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("got %v", err)
	}
	if len(store.users) != 0 || len(store.messages) != 0 || len(store.receipts) != 0 || len(p.updates) != 0 {
		t.Fatal("unauthenticated update persisted")
	}
}

func TestVerifierFailureIsCompletedWithoutGrant(t *testing.T) {
	s, store, v, _, p := fixture()
	store.conversations[42] = domain.Conversation{UserID: 42, ProfileID: "profile", ExpiresAt: time.Now().Add(time.Minute)}
	v.err = domain.ErrUnavailable
	p.err = domain.ErrUnavailable
	u := credential(10, "card")
	apply(t, s, u)
	apply(t, s, u)
	if v.calls != 1 || len(store.grants) != 0 || len(store.receipts) != 1 {
		t.Fatal("failure was retried or granted")
	}
	if len(p.replies) != 1 || !strings.Contains(p.replies[0].Text, "暂时不可用") {
		t.Fatal("service failure not distinguished")
	}
}

func TestRevokedGrantIsRecheckedAtJoin(t *testing.T) {
	s, store, _, tg, _ := fixture()
	store.grants[42] = map[string]domain.Grant{"profile": {UserID: 42, ProfileID: "profile", Active: false}}
	store.invites["invite"] = domain.Invite{ID: "invite", UserID: 42, ChatID: -100, ProfileID: "profile", Link: "link", ExpiresAt: time.Now().Add(time.Minute), Status: "pending"}
	join := domain.JoinRequest{Chat: domain.TelegramChat{ID: -100}, From: domain.TelegramUser{ID: 42}, InviteLink: &struct {
		Link string `json:"invite_link"`
	}{Link: "link"}}
	apply(t, s, domain.Update{ID: 11, Join: &join})
	if len(tg.joins) != 1 || tg.joins[0] || store.invites["invite"].Status != "revoked" {
		t.Fatal("revocation ignored")
	}
}

func TestLaterEditCanHaveSmallerRandomUpdateID(t *testing.T) {
	s, store, _, _, _ := fixture()
	at := time.Now().Add(-8 * 24 * time.Hour)
	store.messages["known"] = domain.Message{ID: "known", ChatID: -100, TelegramMessageID: 8, Date: at, Text: "before", LastUpdateID: 999}
	message := domain.TelegramMessage{MessageID: 8, Date: at.Unix(), EditDate: time.Now().Unix(), Chat: domain.TelegramChat{ID: -100, Type: "supergroup"}, Text: "after"}
	apply(t, s, domain.Update{ID: 1, EditedMessage: &message})
	if store.messages["known"].Text != "after" {
		t.Fatal("random update ID suppressed a new edit")
	}
	message.EditDate--
	message.Text = "stale"
	apply(t, s, domain.Update{ID: 1000, EditedMessage: &message})
	if store.messages["known"].Text != "after" {
		t.Fatal("older edit overwrote newer content")
	}
}

func TestUnknownEditDoesNotResurrectExpiredMessage(t *testing.T) {
	s, store, _, _, _ := fixture()
	input := credential(1, "card").Message
	input.EditDate = time.Now().Unix()
	apply(t, s, domain.Update{ID: 1, EditedMessage: input})
	if len(store.messages) != 0 {
		t.Fatal("unknown historical edit was inserted")
	}
}

func TestEditingOrdinaryMessageWhileAwaitingCredentialIsRedacted(t *testing.T) {
	s, store, verifier, _, _ := fixture()
	at := time.Now().Add(-time.Minute)
	store.messages["ordinary"] = domain.Message{ID: "ordinary", ChatID: 42, TelegramMessageID: 10, Text: "ordinary text", Date: at, LastUpdateID: 1}
	store.conversations[42] = domain.Conversation{UserID: 42, ProfileID: "profile", ExpiresAt: time.Now().Add(time.Minute)}
	message := credential(2, "secret-card").Message
	message.Date, message.EditDate = at.Unix(), time.Now().Unix()
	apply(t, s, domain.Update{ID: 2, EditedMessage: message})
	stored := store.messages["ordinary"]
	if !stored.Sensitive || stored.Text != sensitiveText || verifier.calls != 0 {
		t.Fatalf("credential edit persisted or verified: %+v calls=%d", stored, verifier.calls)
	}
	if _, ok := store.conversations[42]; !ok {
		t.Fatal("edit consumed verification conversation")
	}
}

func TestSendReturnsCanonicalMessageAndUnknownState(t *testing.T) {
	s, store, _, tg, _ := fixture()
	store.messages["echo-id"] = domain.Message{ID: "echo-id", ChatID: -100, TelegramMessageID: 77, Date: time.Now(), Direction: "in"}
	tg.sendResult = domain.TelegramMessage{MessageID: 77, Date: time.Now().Unix()}
	message, err := s.Send(context.Background(), domain.SendMessage{ChatID: -100, Kind: "text", Text: "hello"})
	if err != nil || message.ID != "echo-id" || message.Direction != "out" {
		t.Fatalf("canonical result: %+v %v", message, err)
	}
	tg.sendErr = domain.ErrUnknown
	message, err = s.Send(context.Background(), domain.SendMessage{ChatID: -100, Kind: "text", Text: "timeout"})
	if !errors.Is(err, domain.ErrUnknown) || message.Status != "unknown" || message.TelegramMessageID != 0 {
		t.Fatalf("unknown result: %+v %v", message, err)
	}
}

func TestChannelDoesNotSupportMute(t *testing.T) {
	s, store, _, tg, _ := fixture()
	chat := store.chats[-100]
	chat.Type = "channel"
	store.chats[-100] = chat
	_, err := s.Act(context.Background(), domain.MemberAction{ChatID: -100, UserID: 42, Action: "mute"})
	if !errors.Is(err, domain.ErrInvalid) || tg.acts != 0 {
		t.Fatalf("got %v, calls %d", err, tg.acts)
	}
}

func TestCleanupKeepsReceiptsIndependentFromMessages(t *testing.T) {
	s, store, _, _, _ := fixture()
	store.settings.RetentionDays = 90
	if err := s.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if diff := now.Sub(store.cleanupMessages) - 90*24*time.Hour; diff < 0 || diff > time.Minute {
		t.Fatalf("message cutoff: %v", diff)
	}
	if diff := now.Sub(store.cleanupReceipts) - 7*24*time.Hour; diff < 0 || diff > time.Minute {
		t.Fatalf("receipt cutoff: %v", diff)
	}
}

func TestDifferentBotCannotReuseExistingIdentity(t *testing.T) {
	s, store, _, tg, _ := fixture()
	tg.me = domain.TelegramUser{ID: 100, IsBot: true}
	token := "replacement"
	_, err := s.SaveSettings(context.Background(), domain.SaveSettings{Token: &token, RetentionDays: 30})
	if !errors.Is(err, domain.ErrConflict) || store.settings.Token != "test-token" {
		t.Fatal("bot identity changed")
	}
}

// The authorization mutex also covers admin writes: a revocation must not be
// overwritten by a verification that was already in flight when it was requested.
type gatedVerifier struct {
	domain.Verifier
	started, release chan struct{}
}

func (*gatedVerifier) Validate(domain.RequestConfig) error { return nil }
func (v *gatedVerifier) Verify(ctx context.Context, _ domain.RequestConfig, _ domain.VerificationInput) (domain.VerificationResult, error) {
	close(v.started)
	select {
	case <-v.release:
		return domain.VerificationResult{Passed: true}, nil
	case <-ctx.Done():
		return domain.VerificationResult{}, ctx.Err()
	}
}
func TestRevocationWaitsForInFlightVerification(t *testing.T) {
	s, store, _, _, _ := fixture()
	v := &gatedVerifier{started: make(chan struct{}), release: make(chan struct{})}
	s.verifier = v
	store.grants[42] = map[string]domain.Grant{"profile": {UserID: 42, ProfileID: "profile", Active: true}}
	apply(t, s, callback(1, "v:profile"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	verified := make(chan error, 1)
	revoked := make(chan error, 1)
	go func() { verified <- s.HandleUpdate(ctx, credential(2, "key")) }()
	select {
	case <-v.started:
	case <-ctx.Done():
		t.Fatal("verification did not start")
	}
	go func() { revoked <- s.Revoke(ctx, 42, "profile") }()
	select {
	case err := <-revoked:
		close(v.release)
		<-verified
		t.Fatalf("revocation passed the active verification: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(v.release)
	if err := <-verified; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	if store.grants[42]["profile"].Active {
		t.Fatal("active verification overwrote revocation")
	}
}
func TestUnsupportedMediaKeepsTypeSummary(t *testing.T) {
	s, store, _, _, _ := fixture()
	u := credential(1, "")
	u.Message.Video = &struct{}{}
	u.Message.Caption = "视频说明"
	apply(t, s, u)
	for _, m := range store.messages {
		if m.Kind != "video" || m.Text != "视频说明" || m.FileID != "" {
			t.Fatal("unsupported media did not preserve summary", m)
		}
	}
}
