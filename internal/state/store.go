package state

import (
	"errors"
	"path/filepath"
	"sync"
)

// Store is the state directory entry point.
type Store struct {
	root     string
	mu       sync.Mutex
	projects map[string]*sync.RWMutex
}

// NewStore creates a state store rooted at root.
func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("state root is required")
	}
	return &Store{
		root:     filepath.Clean(root),
		projects: map[string]*sync.RWMutex{},
	}, nil
}

// Root returns the cleaned state directory root.
func (s *Store) Root() string {
	return s.root
}

// Project returns the locked state handle for one project.
func (s *Store) Project(name string) *Project {
	s.mu.Lock()
	defer s.mu.Unlock()

	lock := s.projects[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		s.projects[name] = lock
	}
	return &Project{
		root: nameRoot(s),
		name: name,
		lock: lock,
	}
}

// Project is the state access handle for one project.
type Project struct {
	root string
	name string
	lock *sync.RWMutex
}

// Name returns the project name.
func (p *Project) Name() string {
	return p.name
}

// Root returns the state root.
func (p *Project) Root() string {
	return p.root
}

func nameRoot(s *Store) string {
	if s == nil {
		return ""
	}
	return s.root
}
