package store

import "winding-tree-server/internal/model"

// UserRepository interface
type UserRepository interface {
	Create(*model.User) error
	Find(int) (*model.User, error)
	FindByEmail(string) (*model.User, error)
}
