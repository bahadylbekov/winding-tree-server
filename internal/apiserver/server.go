package apiserver

import (
	"crypto/tls"
	"net/http"
	"time"
	"winding-tree-server/internal/model"
	"winding-tree-server/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

const (
	sessionName        = "go"
	ctxKeyUser  ctxKey = iota
	ctxKeyRequestID
)

var (
	errIncorrectEmailOrPassword = "incorrect email or password"
	errInternalServerError      = "internal server error"
	errNotAuthenticated         = "not authenticated"
	errBadRequest               = "bad request"
)

type server struct {
	router       *gin.Engine
	logger       *logrus.Logger
	store        store.Store
	sessionStore sessions.Store
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	TLSConfig    *tls.Config
}

type ctxKey int8

// NewServer ...
func NewServer(store store.Store, sessionStore sessions.Store) *server {

	tlsConfig := &tls.Config{
		// Causes servers to use Go's default cipher suite preferences,
		// which are tuned to avoid attacks. Does nothing on clients.
		PreferServerCipherSuites: true,
		// Only use curves which have assembly implementations
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519, // Go 1.8 only
		},

		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	s := &server{
		router:       gin.Default(),
		logger:       logrus.New(),
		store:        store,
		sessionStore: sessionStore,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    tlsConfig,
	}

	s.configureRouter()

	return s
}

// ServeHTTP ...
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// configureRouter ..
func (s *server) configureRouter() {
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://moonshard.io", "http://equityone.org"}

	s.router.Use(s.SetRequestID())
	s.router.Use(s.logRequest())
	s.router.Use(cors.New(config))
	s.router.POST("/users", s.handleUsersCreate)
	s.router.POST("/sessions", s.handleSessionsCreate)

	private := s.router.Group("/private")
	private.Use(s.AuthenticationUser())
	{
		private.GET("/whoami", s.getMyUserInfo)
	}

}

// authenticateUser ...
func (s *server) AuthenticationUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := s.sessionStore.Get(c.Request, sessionName)
		if err != nil {
			respondWithError(c, http.StatusInternalServerError, errInternalServerError)
			return
		}

		id, ok := session.Values["user_id"]
		if !ok {
			respondWithError(c, http.StatusUnauthorized, errNotAuthenticated)
			return
		}

		u, err := s.store.User().Find(id.(int))
		if err != nil {
			respondWithError(c, http.StatusUnauthorized, errNotAuthenticated)
			return
		}
		c.Set("ctxKeyUser", u)
		c.Next()
	}
}

// handleUsersCreate ...
func (s *server) handleUsersCreate(c *gin.Context) {
	var u *model.User
	c.BindJSON(&u)

	if err := s.store.User().Create(u); err != nil {
		respondWithError(c, http.StatusBadRequest, errBadRequest)
		return
	}

	u.Sanitize()
	c.JSON(http.StatusOK, gin.H{
		"email":              u.Email,
		"encrypted_password": u.EncryptedPassword,
	})
}

// handleSessionsCreate ...
func (s *server) handleSessionsCreate(c *gin.Context) {
	var req *model.User
	c.BindJSON(&req)

	u, err := s.store.User().FindByEmail(req.Email)
	if err != nil || !u.ComparePasswords(req.Password) {
		respondWithError(c, http.StatusUnauthorized, errIncorrectEmailOrPassword)
		return
	}

	session, err := s.sessionStore.Get(c.Request, "go")
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, errInternalServerError)
		return
	}

	session.Values["user_id"] = u.ID
	if err := s.sessionStore.Save(c.Request, c.Writer, session); err != nil {
		respondWithError(c, http.StatusInternalServerError, errInternalServerError)
		return
	}
}

func (s *server) logRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		logger := s.logger.WithFields(logrus.Fields{
			"remote_addr": c.Request.RemoteAddr,
			"request_id":  c.Value("ctxKeyRequestID"),
		})
		logger.Infof("started %s %s", c.Request.Method, c.Request.RequestURI)
		start := time.Now()
		c.Next()

		var level logrus.Level
		switch {
		case c.Writer.Status() >= 500:
			level = logrus.ErrorLevel
		case c.Writer.Status() >= 400:
			level = logrus.WarnLevel
		default:
			level = logrus.InfoLevel
		}
		logger.Logf(
			level,
			"completed with %d %s in %v",
			c.Writer.Status(),
			http.StatusText(c.Writer.Status()),
			time.Now().Sub(start),
		)
	}
}

func (s *server) SetRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.New().String()
		c.Header("X-Request-ID", id)
		c.Set("ctxKeyRequestID", id)
		c.Next()
	}
}

// handleUsersCreate ...
func (s *server) getMyUserInfo(c *gin.Context) {
	c.JSON(http.StatusOK, c.Value("ctxKeyUser").(*model.User))
}

// respondWithError ...
func respondWithError(c *gin.Context, code int, message interface{}) {
	c.AbortWithStatusJSON(code, gin.H{"error": message})
}
