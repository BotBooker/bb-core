package serverapi

import (
	"os"
	"testing"
)

func TestNewDIContainer(t *testing.T) {
	c := newDIContainer()
	if c == nil {
		t.Fatal("newDIContainer() returned nil")
	}
	if c.db != nil {
		t.Error("db should be nil initially")
	}
	if c.cache != nil {
		t.Error("cache should be nil initially")
	}
	if c.eventBus != nil {
		t.Error("eventBus should be nil initially")
	}
}

func TestDIContainer_DB(t *testing.T) {
	// Save and restore env
	originalDSN := os.Getenv("DSN")
	defer os.Setenv("DSN", originalDSN)
	os.Setenv("DSN", "postgres://test:5432/test")

	c := newDIContainer()
	db := c.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}

	// Second call should return same instance
	db2 := c.DB()
	if db != db2 {
		t.Error("DB() should return same instance on multiple calls")
	}
}

func TestDIContainer_Cache(t *testing.T) {
	// Save and restore env
	originalRedis := os.Getenv("REDIS_HOST")
	defer os.Setenv("REDIS_HOST", originalRedis)
	os.Setenv("REDIS_HOST", "localhost:6379")

	c := newDIContainer()
	cache := c.Cache()
	if cache == nil {
		t.Fatal("Cache() returned nil")
	}

	// Second call should return same instance
	cache2 := c.Cache()
	if cache != cache2 {
		t.Error("Cache() should return same instance on multiple calls")
	}
}

func TestDIContainer_EventBus(t *testing.T) {
	c := newDIContainer()
	bus := c.EventBus()
	if bus == nil {
		t.Fatal("EventBus() returned nil")
	}

	// Second call should return same instance
	bus2 := c.EventBus()
	if bus != bus2 {
		t.Error("EventBus() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserRepo(t *testing.T) {
	c := newDIContainer()
	repo := c.UserRepo()
	if repo == nil {
		t.Fatal("UserRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.UserRepo()
	if repo != repo2 {
		t.Error("UserRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_SessionRepo(t *testing.T) {
	c := newDIContainer()
	repo := c.SessionRepo()
	if repo == nil {
		t.Fatal("SessionRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.SessionRepo()
	if repo != repo2 {
		t.Error("SessionRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_NotificationRepo(t *testing.T) {
	c := newDIContainer()
	repo := c.NotificationRepo()
	if repo == nil {
		t.Fatal("NotificationRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.NotificationRepo()
	if repo != repo2 {
		t.Error("NotificationRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_AuthService(t *testing.T) {
	c := newDIContainer()
	svc := c.AuthService()
	if svc == nil {
		t.Fatal("AuthService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.AuthService()
	if svc != svc2 {
		t.Error("AuthService() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserService(t *testing.T) {
	c := newDIContainer()
	svc := c.UserService()
	if svc == nil {
		t.Fatal("UserService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.UserService()
	if svc != svc2 {
		t.Error("UserService() should return same instance on multiple calls")
	}
}

func TestDIContainer_NotificationService(t *testing.T) {
	c := newDIContainer()
	svc := c.NotificationService()
	if svc == nil {
		t.Fatal("NotificationService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.NotificationService()
	if svc != svc2 {
		t.Error("NotificationService() should return same instance on multiple calls")
	}
}

func TestDIContainer_Handler(t *testing.T) {
	c := newDIContainer()
	h := c.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	// Second call should return same instance
	h2 := c.Handler()
	if h != h2 {
		t.Error("Handler() should return same instance on multiple calls")
	}
}

func TestDIContainer_Integration(t *testing.T) {
	// Save and restore env
	originalDSN := os.Getenv("DSN")
	originalRedis := os.Getenv("REDIS_HOST")
	defer func() {
		os.Setenv("DSN", originalDSN)
		os.Setenv("REDIS_HOST", originalRedis)
	}()
	os.Setenv("DSN", "postgres://test:5432/test")
	os.Setenv("REDIS_HOST", "localhost:6379")

	c := newDIContainer()

	// Get all dependencies
	db := c.DB()
	cache := c.Cache()
	bus := c.EventBus()
	userRepo := c.UserRepo()
	sessionRepo := c.SessionRepo()
	notifRepo := c.NotificationRepo()
	authSvc := c.AuthService()
	userSvc := c.UserService()
	notifSvc := c.NotificationService()
	handler := c.Handler()

	if db == nil {
		t.Error("DB is nil")
	}
	if cache == nil {
		t.Error("Cache is nil")
	}
	if bus == nil {
		t.Error("EventBus is nil")
	}
	if userRepo == nil {
		t.Error("UserRepo is nil")
	}
	if sessionRepo == nil {
		t.Error("SessionRepo is nil")
	}
	if notifRepo == nil {
		t.Error("NotificationRepo is nil")
	}
	if authSvc == nil {
		t.Error("AuthService is nil")
	}
	if userSvc == nil {
		t.Error("UserService is nil")
	}
	if notifSvc == nil {
		t.Error("NotificationService is nil")
	}
	if handler == nil {
		t.Error("Handler is nil")
	}
}
