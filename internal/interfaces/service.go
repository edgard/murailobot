package interfaces

// Service provides access to core services
type Service interface {
	AI() AI
	Bot() Bot
	DB() DB
	Scheduler() Scheduler
}
