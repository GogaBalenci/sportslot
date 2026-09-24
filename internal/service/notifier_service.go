package service

import (
	"context"
	"log"
	"time"

	"sportslot/internal/maxclient"
	"sportslot/internal/repository"
)

const reminderWindowBefore = 55 * time.Minute
const reminderWindowAfter = 65 * time.Minute

// NotifierService - фоновая горутина без внешних очередей (Redis/RabbitMQ/cron):
// проверяет предстоящие тренировки по time.Ticker и шлёт напоминания через
// MAX Bot API. Запускается как `go notifierService.Run(ctx)` в main.go.
type NotifierService struct {
	bookingRepo repository.BookingRepository
	maxClient   *maxclient.Client
	interval    time.Duration
}

func NewNotifierService(bookingRepo repository.BookingRepository, maxClient *maxclient.Client, interval time.Duration) *NotifierService {
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	return &NotifierService{
		bookingRepo: bookingRepo,
		maxClient:   maxClient,
		interval:    interval,
	}
}

// Run блокирует выполнение до отмены ctx - предполагается запуск в отдельной горутине.
func (n *NotifierService) Run(ctx context.Context) {
	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()

	log.Printf("notifier: started with interval=%s", n.interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("notifier: stopped")
			return
		case <-ticker.C:
			n.checkAndSendReminders(ctx)
		}
	}
}

func (n *NotifierService) checkAndSendReminders(ctx context.Context) {
	now := time.Now()
	windowStart := now.Add(reminderWindowBefore)
	windowEnd := now.Add(reminderWindowAfter)

	reminders, err := n.bookingRepo.FindUpcomingForReminder(ctx, windowStart, windowEnd)
	if err != nil {
		log.Printf("notifier: fetch upcoming reminders error: %v", err)
		return
	}

	for _, rem := range reminders {
		if err := n.maxClient.SendReminder(ctx, rem.MaxUserID, rem.VenueName, rem.SlotStartAt); err != nil {
			log.Printf("notifier: send reminder error for booking %s: %v", rem.BookingID, err)
			continue // ошибка одной отправки не блокирует остальные напоминания
		}
		if err := n.bookingRepo.MarkReminderSent(ctx, rem.BookingID); err != nil {
			log.Printf("notifier: mark reminder sent error for booking %s: %v", rem.BookingID, err)
		}
	}
}
