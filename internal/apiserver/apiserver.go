package apiserver

import (
	"net/http"
	"winding-tree-server/internal/store/sqlstore"

	"github.com/gin-contrib/sessions/cookie"
	"github.com/jmoiron/sqlx"
)

// Start ...
func Start(config *Config) error {
	db, err := newDB(config.DatabaseURL)
	if err != nil {
		return err
	}

	defer db.Close()
	store := sqlstore.New(db)
	sessionStore := cookie.NewStore([]byte(config.SessionKey))
	s := NewServer(store, sessionStore)

	return http.ListenAndServe(config.BindAddress, s)
}

// newDB ...
func newDB(databaseURL string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
