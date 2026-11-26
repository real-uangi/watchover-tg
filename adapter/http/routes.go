package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/real-uangi/allingo/common/result"
	authapp "github.com/real-uangi/watchover-tg/internal/auth/application"
	authdomain "github.com/real-uangi/watchover-tg/internal/auth/domain"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
	"go.uber.org/fx"
)

const cookieName = "watchover_session"

var Module = fx.Module("http", fx.Invoke(Register))

func Register(engine *gin.Engine, cfg *config.Config, auth *authapp.Service, bot *application.Service, assets fs.FS) error {
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return errors.New("management frontend build is missing")
	}
	api := engine.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
	})
	api.POST("/telegram/webhook", func(c *gin.Context) {
		var update domain.Update
		if !readJSONLimit(c, &update, 1<<20) {
			return
		}
		err := bot.ReceiveUpdate(c.Request.Context(), c.GetHeader("X-Telegram-Bot-Api-Secret-Token"), update)
		reply(c, nil, err)
	})
	admin := api.Group("/admin")
	admin.Use(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			origin, err := config.CanonicalOrigin(c.GetHeader("Origin"))
			if err != nil || origin != cfg.Origin {
				respond(c, 403, "请求来源无效", nil)
				c.Abort()
			}
		}
	})
	admin.POST("/auth/login", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !readJSON(c, &input) {
			return
		}
		session, err := auth.Login(c.Request.Context(), input.Password)
		if err != nil {
			reply(c, nil, err)
			return
		}
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: session.Token, Path: "/api/v1/admin", MaxAge: 12 * 3600, Expires: session.ExpiresAt, HttpOnly: true, Secure: cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
		reply(c, gin.H{"authenticated": true, "expiresAt": session.ExpiresAt}, nil)
	})
	admin.Use(func(c *gin.Context) {
		token, _ := c.Cookie(cookieName)
		if _, err := auth.Session(c.Request.Context(), token); err != nil {
			reply(c, nil, err)
			c.Abort()
		}
	})
	admin.GET("/auth/session", func(c *gin.Context) {
		token, _ := c.Cookie(cookieName)
		session, err := auth.Session(c.Request.Context(), token)
		reply(c, gin.H{"authenticated": true, "expiresAt": session.ExpiresAt}, err)
	})
	admin.POST("/auth/logout", func(c *gin.Context) {
		token, _ := c.Cookie(cookieName)
		err := auth.Logout(c.Request.Context(), token)
		if err == nil {
			http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Path: "/api/v1/admin", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
		}
		reply(c, nil, err)
	})
	admin.GET("/settings", func(c *gin.Context) { v, e := bot.Settings(c.Request.Context()); reply(c, v, e) })
	admin.POST("/settings/save", func(c *gin.Context) {
		var input struct {
			Token         *string `json:"token"`
			Welcome       string  `json:"welcome"`
			RetentionDays *int    `json:"retentionDays"`
		}
		if !readJSON(c, &input) {
			return
		}
		if input.RetentionDays == nil {
			reply(c, nil, domain.ErrInvalid)
			return
		}
		v, e := bot.SaveSettings(c.Request.Context(), domain.SaveSettings{Token: input.Token, Welcome: input.Welcome, RetentionDays: *input.RetentionDays})
		reply(c, v, e)
	})
	admin.POST("/settings/check", func(c *gin.Context) { v, e := bot.CheckBot(c.Request.Context()); reply(c, v, e) })
	admin.GET("/webhook/info", func(c *gin.Context) { v, e := bot.WebhookInfo(c.Request.Context()); reply(c, v, e) })
	admin.POST("/webhook/register", func(c *gin.Context) { reply(c, nil, bot.Webhook(c.Request.Context(), true)) })
	admin.POST("/webhook/remove", func(c *gin.Context) { reply(c, nil, bot.Webhook(c.Request.Context(), false)) })
	admin.GET("/profiles/list", func(c *gin.Context) {
		v, e := bot.Profiles(c.Request.Context())
		if v == nil {
			v = []domain.Profile{}
		}
		reply(c, gin.H{"items": v}, e)
	})
	admin.POST("/profiles/save", func(c *gin.Context) {
		var v domain.Profile
		if !readJSON(c, &v) {
			return
		}
		v, e := bot.SaveProfile(c.Request.Context(), v)
		reply(c, v, e)
	})
	admin.POST("/profiles/delete", func(c *gin.Context) {
		var v struct {
			ID string `json:"id"`
		}
		if !readJSON(c, &v) {
			return
		}
		reply(c, nil, bot.DeleteProfile(c.Request.Context(), v.ID))
	})
	admin.POST("/profiles/test", func(c *gin.Context) {
		var v struct {
			Profile    domain.Profile `json:"profile"`
			Credential string         `json:"credential"`
			UserID     int64          `json:"userId"`
			Username   string         `json:"username"`
		}
		if !readJSON(c, &v) {
			return
		}
		r, e := bot.TestProfile(c.Request.Context(), v.Profile, domain.VerificationInput{Credential: v.Credential, UserID: v.UserID, Username: v.Username})
		reply(c, r, e)
	})
	admin.GET("/chats/list", func(c *gin.Context) {
		v, e := bot.Chats(c.Request.Context())
		if v == nil {
			v = []domain.Chat{}
		}
		reply(c, gin.H{"items": v}, e)
	})
	admin.POST("/chats/add", func(c *gin.Context) {
		var v struct {
			ID int64 `json:"id"`
		}
		if !readJSON(c, &v) {
			return
		}
		r, e := bot.AddChat(c.Request.Context(), v.ID)
		reply(c, r, e)
	})
	admin.POST("/chats/save", func(c *gin.Context) {
		var v struct {
			ID      int64 `json:"id"`
			Enabled *bool `json:"enabled"`
		}
		if !readJSON(c, &v) {
			return
		}
		if v.Enabled == nil {
			reply(c, nil, domain.ErrInvalid)
			return
		}
		r, e := bot.SaveChat(c.Request.Context(), v.ID, *v.Enabled)
		reply(c, r, e)
	})
	admin.POST("/chats/check", func(c *gin.Context) {
		var v struct {
			ID int64 `json:"id"`
		}
		if !readJSON(c, &v) {
			return
		}
		r, e := bot.CheckChat(c.Request.Context(), v.ID)
		reply(c, r, e)
	})
	admin.GET("/chats/members", func(c *gin.Context) {
		id, ok := queryID(c, "chatId")
		if !ok {
			return
		}
		v, e := bot.Members(c.Request.Context(), id)
		if v == nil {
			v = []domain.Member{}
		}
		reply(c, gin.H{"items": v}, e)
	})
	admin.GET("/chats/invites", func(c *gin.Context) {
		id, ok := queryID(c, "chatId")
		if !ok {
			return
		}
		v, e := bot.Invites(c.Request.Context(), id)
		if v == nil {
			v = []domain.Invite{}
		}
		reply(c, gin.H{"items": v}, e)
	})
	admin.POST("/members/action", func(c *gin.Context) {
		var v domain.MemberAction
		if !readJSON(c, &v) {
			return
		}
		r, e := bot.Act(c.Request.Context(), v)
		reply(c, r, e)
	})
	admin.GET("/users/list", func(c *gin.Context) {
		v, e := bot.Users(c.Request.Context(), c.Query("q"), c.Query("cursor"))
		if v.Items == nil {
			v.Items = []domain.User{}
		}
		reply(c, v, e)
	})
	admin.GET("/users/detail", func(c *gin.Context) {
		id, ok := queryID(c, "id")
		if !ok {
			return
		}
		v, e := bot.UserDetail(c.Request.Context(), id)
		if v.Grants == nil {
			v.Grants = []domain.Grant{}
		}
		if v.Members == nil {
			v.Members = []domain.Member{}
		}
		reply(c, v, e)
	})
	admin.POST("/users/note", func(c *gin.Context) {
		var v struct {
			ID   int64  `json:"id"`
			Note string `json:"note"`
		}
		if !readJSON(c, &v) {
			return
		}
		r, e := bot.SaveNote(c.Request.Context(), v.ID, v.Note)
		reply(c, r, e)
	})
	admin.POST("/users/revoke", func(c *gin.Context) {
		var v struct {
			UserID    int64  `json:"userId"`
			ProfileID string `json:"profileId"`
		}
		if !readJSON(c, &v) {
			return
		}
		reply(c, nil, bot.Revoke(c.Request.Context(), v.UserID, v.ProfileID))
	})
	admin.GET("/messages/list", func(c *gin.Context) {
		id, ok := queryID(c, "chatId")
		if !ok {
			return
		}
		v, e := bot.Messages(c.Request.Context(), id, c.Query("cursor"))
		if v.Items == nil {
			v.Items = []domain.Message{}
		}
		reply(c, v, e)
	})
	admin.POST("/messages/send", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 51<<20)
		err := c.Request.ParseMultipartForm(8 << 20)
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				respond(c, 413, "上传内容超过限制", nil)
			} else {
				respond(c, 400, "上传表单无效", nil)
			}
			return
		}
		id, err := strconv.ParseInt(c.PostForm("chatId"), 10, 64)
		if err != nil || id == 0 {
			reply(c, nil, domain.ErrInvalid)
			return
		}
		var replyID int64
		if raw := c.PostForm("replyToMessageId"); raw != "" {
			replyID, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || replyID < 0 {
				reply(c, nil, domain.ErrInvalid)
				return
			}
		}
		input := domain.SendMessage{ChatID: id, Kind: c.PostForm("kind"), Text: c.PostForm("text"), ReplyToMessageID: replyID}
		if input.Kind == "photo" || input.Kind == "document" {
			file, header, err := c.Request.FormFile("file")
			if err != nil {
				respond(c, 400, "请选择上传文件", nil)
				return
			}
			defer file.Close()
			input.File = file
			input.FileName = header.Filename
			input.MIMEType = header.Header.Get("Content-Type")
			input.Size = header.Size
		}
		v, e := bot.Send(c.Request.Context(), input)
		reply(c, v, e)
	})
	admin.GET("/messages/file", func(c *gin.Context) {
		file, err := bot.Download(c.Request.Context(), c.Query("id"))
		if err != nil {
			reply(c, nil, err)
			return
		}
		defer file.Body.Close()
		c.Header("Content-Security-Policy", "sandbox")
		disposition := "attachment"
		contentType := "application/octet-stream"
		if file.MIMEType == "image/jpeg" || file.MIMEType == "image/png" || file.MIMEType == "image/webp" {
			disposition = "inline"
			contentType = file.MIMEType
		}
		c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))
		c.DataFromReader(200, file.Size, contentType, file.Body, nil)
	})
	files := http.FileServer(http.FS(assets))
	engine.NoRoute(func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			respond(c, 404, "接口不存在", nil)
			return
		}
		name := strings.TrimPrefix(path.Clean(requestPath), "/")
		if info, err := fs.Stat(assets, name); err == nil && !info.IsDir() {
			files.ServeHTTP(c.Writer, c.Request)
			return
		}
		if path.Ext(name) != "" {
			c.Status(404)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(200, "text/html; charset=utf-8", index)
	})
	return nil
}
func queryID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Query(name), 10, 64)
	if err != nil || id == 0 {
		reply(c, nil, domain.ErrInvalid)
		return 0, false
	}
	return id, true
}
func readJSON(c *gin.Context, dest any) bool { return readJSONLimit(c, dest, 64<<10) }
func readJSONLimit(c *gin.Context, dest any, limit int64) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		respond(c, 415, "需要 JSON 请求", nil)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	decoder := json.NewDecoder(c.Request.Body)
	err = decoder.Decode(dest)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = domain.ErrInvalid
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			respond(c, 413, "请求体超过限制", nil)
		} else {
			respond(c, 400, "请求 JSON 无效", nil)
		}
		return false
	}
	return true
}
func reply(c *gin.Context, data any, err error) {
	if err == nil {
		respond(c, 200, "OK", data)
		return
	}
	switch {
	case errors.Is(err, authdomain.ErrUnauthorized):
		respond(c, 401, "管理员认证失效", nil)
	case errors.Is(err, domain.ErrForbidden):
		respond(c, 403, "请求未授权", nil)
	case errors.Is(err, domain.ErrInvalid):
		respond(c, 400, "请求字段或配置无效", nil)
	case errors.Is(err, domain.ErrNotFound):
		respond(c, 404, "资源不存在", nil)
	case errors.Is(err, domain.ErrConflict):
		respond(c, 409, "配置或操作存在冲突", nil)
	case errors.Is(err, domain.ErrUnknown):
		respond(c, 502, "操作结果未知，请核对后再操作", data)
	case errors.Is(err, domain.ErrUnavailable):
		respond(c, 503, "外部服务暂不可用或权限不足", nil)
	default:
		respond(c, 500, "内部处理失败", nil)
	}
}
func respond(c *gin.Context, status int, message string, data any) {
	c.Header("Cache-Control", "no-store")
	c.Render(status, result.Custom(status, message, data))
}
